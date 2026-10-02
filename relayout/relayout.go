// Package relayout lays a recognized flowchart or swimlane diagram out from
// scratch with the layout engine, then maps the result back onto the original
// cells, which keep their styles and text.
//
// The engine reads a Flow Table, so a diagram is first written as one: each
// shape becomes an element row, each wire an edge row, each lane a lane row. A
// Flow Table carries what a drawing does not say outright, and the current
// drawing supplies it as hints: the direction most wires go, lane order by
// position, the main branch as the target most in line with its source, and
// back edges from a depth-first walk in reading order.
package relayout

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/engine/layout"
	"github.com/luytbq/diakempt/engine/model"
	"github.com/luytbq/diakempt/engine/schema"
	"github.com/luytbq/diakempt/engine/validate"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/view"
)

// Plan is a diagram written as a Flow Table, with the way back from row ids to
// the view.
type Plan struct {
	Rows      []model.Row
	Dir       string
	nodes     map[string]*view.Node
	edges     map[string]*edge
	lanes     map[string]*view.Node
	laneOrder []*view.Node
	pool      *view.Node
	cfg       layout.Config
}

type edge struct {
	w        *view.Wire
	src, dst *view.Node // in flow direction
	reversed bool       // the flow runs from the wire's target to its source
	back     bool
	id       string
}

// Unsupported explains why a diagram cannot be laid out by the engine.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

func unsupported(format string, args ...any) error {
	return &Unsupported{Reason: fmt.Sprintf(format, args...)}
}

// Build writes a diagram as a Flow Table. withLanes selects a swimlane
// diagram; without it, a flowchart.
func Build(sd *segment.Diagram, withLanes bool) (*Plan, error) {
	p := &Plan{nodes: map[string]*view.Node{}, edges: map[string]*edge{}, lanes: map[string]*view.Node{}, cfg: layout.DefaultConfig()}
	var leaves []*view.Node
	var containers []*view.Node
	for _, n := range sd.Nodes {
		if n.Container {
			containers = append(containers, n)
		} else {
			leaves = append(leaves, n)
		}
	}
	if err := p.findLanes(containers, leaves, withLanes); err != nil {
		return nil, err
	}
	var edges []*edge
	wired := map[*view.Node]bool{}
	for _, w := range sd.Wires {
		switch {
		case w.Src == nil || w.Dst == nil:
			return nil, unsupported("wire %s has a free end", w.Cell.ID)
		case w.Src.Container || w.Dst.Container:
			return nil, unsupported("wire %s ends on a container", w.Cell.ID)
		case w.Src == w.Dst:
			return nil, unsupported("wire %s loops back to its own shape", w.Cell.ID)
		}
		e := &edge{w: w, src: w.Src, dst: w.Dst}
		end := w.Style.Value("endArrow", "classic") != "none"
		start := w.Style.Value("startArrow", "none") != "none"
		if start && !end {
			e.src, e.dst, e.reversed = w.Dst, w.Src, true
		}
		edges = append(edges, e)
		wired[w.Src], wired[w.Dst] = true, true
	}
	p.Dir = direction(edges, p.laneOrder)
	return p, p.write(leaves, edges, wired)
}

// findLanes picks the lanes: swimlane containers holding shapes, and the pool
// holding them if there is one. Any other container cannot be represented.
func (p *Plan) findLanes(containers, leaves []*view.Node, withLanes bool) error {
	if !withLanes {
		if len(containers) > 0 {
			return unsupported("a flowchart inside containers")
		}
		return nil
	}
	for _, c := range containers {
		holdsShapes := false
		for _, ch := range c.Children {
			if !ch.Container {
				holdsShapes = true
			}
		}
		if c.Shape == "swimlane" && holdsShapes {
			p.laneOrder = append(p.laneOrder, c)
		}
	}
	if len(p.laneOrder) == 0 {
		return unsupported("no lanes holding shapes")
	}
	pool := p.laneOrder[0].Parent
	// Empty lanes of the same pool are lanes too.
	if pool != nil {
		for _, c := range pool.Children {
			if c.Container && c.Shape == "swimlane" && len(c.Children) == 0 {
				p.laneOrder = append(p.laneOrder, c)
			}
		}
	}
	for _, l := range p.laneOrder {
		if l.Parent != pool {
			return unsupported("lanes in different pools")
		}
		for _, ch := range l.Children {
			if ch.Container {
				return unsupported("a container inside lane %s", l.ID)
			}
		}
	}
	for _, c := range containers {
		isLane := false
		for _, l := range p.laneOrder {
			isLane = isLane || l == c
		}
		if !isLane && c != pool {
			return unsupported("container %s is neither a lane nor their pool", c.ID)
		}
	}
	if pool != nil && pool.Parent != nil {
		return unsupported("the pool sits inside another container")
	}
	p.pool = pool
	for _, n := range leaves {
		if n.Parent == nil {
			return unsupported("shape %s is outside every lane", n.ID)
		}
	}
	// Lanes side by side read left to right; stacked lanes top to bottom.
	vertical := lanesVertical(p.laneOrder)
	sort.SliceStable(p.laneOrder, func(i, j int) bool {
		a, b := p.laneOrder[i].Box, p.laneOrder[j].Box
		if vertical {
			return a.X < b.X
		}
		return a.Y < b.Y
	})
	if p.pool != nil {
		p.cfg.PoolHeader = startSize(p.pool)
	} else {
		p.cfg.PoolHeader = 0
	}
	p.cfg.LaneHeader = startSize(p.laneOrder[0])
	return nil
}

// lanesVertical reports whether lanes stand side by side as columns.
func lanesVertical(lanes []*view.Node) bool {
	if len(lanes) < 2 {
		return lanes[0].Style.Value("horizontal", "1") != "0"
	}
	var minX, maxX, minY, maxY = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	for _, l := range lanes {
		c := l.Box.Center()
		minX, maxX = math.Min(minX, c.X), math.Max(maxX, c.X)
		minY, maxY = math.Min(minY, c.Y), math.Max(maxY, c.Y)
	}
	return maxX-minX >= maxY-minY
}

func startSize(n *view.Node) int {
	v := geom.ParseNumber(n.Style.Value("startSize", "23"))
	return int(math.Round(v))
}

// direction is TD for lanes standing as columns or wires mostly going down, LR
// for lanes stacked as rows or wires mostly going right.
func direction(edges []*edge, lanes []*view.Node) string {
	if len(lanes) > 0 {
		if lanesVertical(lanes) {
			return layout.DirTD
		}
		return layout.DirLR
	}
	down, right := 0, 0
	for _, e := range edges {
		a, b := e.src.Box.Center(), e.dst.Box.Center()
		if b.Y-a.Y > 5 {
			down++
		}
		if b.X-a.X > 5 {
			right++
		}
	}
	if right > down {
		return layout.DirLR
	}
	return layout.DirTD
}

// flow and cross return a point's coordinates along and across the direction.
func (p *Plan) flow(pt geom.Point) float64 {
	if p.Dir == layout.DirLR {
		return pt.X
	}
	return pt.Y
}

func (p *Plan) cross(pt geom.Point) float64 {
	if p.Dir == layout.DirLR {
		return pt.Y
	}
	return pt.X
}

// write orders the rows the way the Flow Table requires: lanes, then from each
// start, every element followed by what attaches to it, its outgoing edges and
// then their targets; a merge node waits for its last source.
func (p *Plan) write(leaves []*view.Node, edges []*edge, wired map[*view.Node]bool) error {
	ids := map[*view.Node]string{}
	laneIDs := map[*view.Node]string{}
	for i, l := range p.laneOrder {
		id := fmt.Sprintf("L%d", i+1)
		laneIDs[l] = id
		p.lanes[id] = l
		p.add(model.Row{ID: id, Type: "lane", Lines: lines(l.Text)})
	}
	byFlow := append([]*view.Node(nil), leaves...)
	sort.SliceStable(byFlow, func(i, j int) bool {
		a, b := byFlow[i].Box.Center(), byFlow[j].Box.Center()
		if math.Abs(p.flow(a)-p.flow(b)) > 5 {
			return p.flow(a) < p.flow(b)
		}
		return p.cross(a) < p.cross(b)
	})
	for i, n := range byFlow {
		ids[n] = fmt.Sprintf("N%d", i+1)
		p.nodes[ids[n]] = n
	}
	outs, ins := map[*view.Node][]*edge{}, map[*view.Node][]*edge{}
	for _, e := range edges {
		outs[e.src] = append(outs[e.src], e)
		ins[e.dst] = append(ins[e.dst], e)
	}
	// Outgoing edges: side branches by position across the flow, the main
	// branch (the target most in line with the source) last.
	for _, n := range byFlow {
		es := outs[n]
		c := p.cross(n.Box.Center())
		main := -1
		for i, e := range es {
			if main < 0 || math.Abs(p.cross(e.dst.Box.Center())-c) < math.Abs(p.cross(es[main].dst.Box.Center())-c) {
				main = i
			}
		}
		if main >= 0 {
			m := es[main]
			rest := append(append([]*edge(nil), es[:main]...), es[main+1:]...)
			sort.SliceStable(rest, func(i, j int) bool {
				return p.cross(rest[i].dst.Box.Center()) < p.cross(rest[j].dst.Box.Center())
			})
			outs[n] = append(rest, m)
		}
	}
	// Back edges: a depth-first walk from the starts in reading order.
	var starts []*view.Node
	for _, n := range byFlow {
		if wired[n] && len(ins[n]) == 0 {
			starts = append(starts, n)
		}
	}
	state := map[*view.Node]int{} // 1 on the stack, 2 done
	var dfs func(n *view.Node)
	dfs = func(n *view.Node) {
		state[n] = 1
		for _, e := range outs[n] {
			switch state[e.dst] {
			case 1:
				e.back = true
			case 0:
				dfs(e.dst)
			}
		}
		state[n] = 2
	}
	for _, n := range starts {
		if state[n] == 0 {
			dfs(n)
		}
	}
	for _, n := range byFlow {
		if wired[n] && state[n] == 0 {
			dfs(n)
		}
	}
	// Shapes without wires attach to the nearest wired shape as notes or data.
	// They are listed in document order, not by position: the engine decides
	// where each one goes from its order, so an order taken from positions
	// would change every time the result is laid out again.
	attached := map[*view.Node][]*view.Node{}
	attachTo := map[*view.Node]*view.Node{}
	for _, n := range leaves {
		if wired[n] {
			continue
		}
		var best *view.Node
		bd := math.Inf(1)
		for _, m := range byFlow {
			if m == n || !wired[m] || (p.pool != nil && m.Parent != n.Parent) {
				continue
			}
			if d := gap(n.Box, m.Box); d < bd {
				best, bd = m, d
			}
		}
		if best == nil {
			return unsupported("shape %s has no wire and nothing to stand beside", n.ID)
		}
		attached[best] = append(attached[best], n)
		attachTo[n] = best
	}
	eid := 0
	written := map[*view.Node]bool{}
	ready := func(n *view.Node) bool {
		for _, e := range ins[n] {
			if !e.back && !written[e.src] {
				return false
			}
		}
		return true
	}
	var emit func(n *view.Node)
	emit = func(n *view.Node) {
		written[n] = true
		p.add(model.Row{ID: ids[n], Type: elementType(n, ins[n], outs[n]), Parent: laneIDs[n.Parent], Lines: lines(n.Text)})
		for _, a := range attached[n] {
			typ := "text"
			if isDB(a) {
				typ = "db"
			}
			written[a] = true
			p.add(model.Row{ID: ids[a], Type: typ, Parent: laneIDs[a.Parent], Lines: lines(a.Text),
				Meta: map[string]string{"attach": ids[n]}, MetaKeys: []string{"attach"}})
		}
		for _, e := range outs[n] {
			eid++
			e.id = fmt.Sprintf("E%d", eid)
			p.edges[e.id] = e
			meta := map[string]string{"from": ids[e.src], "to": ids[e.dst]}
			keys := []string{"from", "to"}
			if e.back {
				meta["back"] = "true"
				keys = append(keys, "back")
			}
			p.add(model.Row{ID: e.id, Type: "edge", Lines: lines(e.w.Text), Meta: meta, MetaKeys: keys})
		}
		for _, e := range outs[n] {
			if !written[e.dst] && ready(e.dst) {
				emit(e.dst)
			}
		}
	}
	for _, n := range starts {
		if !written[n] {
			emit(n)
		}
	}
	rest := false
	for _, n := range byFlow {
		if written[n] || attachTo[n] != nil {
			continue
		}
		if !rest {
			p.add(model.Row{ID: "REST", Type: "text", Lines: []string{schema.RestMarker}})
			rest = true
		}
		emit(n)
	}
	for _, is := range validate.Validate(p.Rows) {
		if is.Level == model.LevelError {
			return unsupported("the diagram does not make a valid flow table: %s", is.Msg)
		}
	}
	return nil
}

func (p *Plan) add(r model.Row) {
	r.Idx = len(p.Rows)
	p.Rows = append(p.Rows, r)
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func isDB(n *view.Node) bool {
	switch n.Shape {
	case "cylinder", "cylinder3", "datastore", "mxgraph.flowchart.database", "mxgraph.flowchart.stored_data":
		return true
	}
	return false
}

func isEllipse(n *view.Node) bool {
	switch n.Shape {
	case "ellipse", "doubleEllipse", "mxgraph.flowchart.terminator", "mxgraph.flowchart.start_1", "mxgraph.flowchart.start_2":
		return true
	}
	return false
}

// elementType maps a shape and its wires to a Flow Table element type. The type
// decides the engine's geometry for the shape; the shape's own style is kept.
func elementType(n *view.Node, in, out []*edge) string {
	switch {
	case (n.Shape == "rhombus" || n.Shape == "mxgraph.flowchart.decision") && len(out) >= 2:
		return "condition"
	case isDB(n):
		return "db"
	case n.Shape == "ellipse" && n.Style.Value("dashed", "0") == "1":
		return "external"
	case n.Shape == "doubleEllipse":
		return "end"
	case isEllipse(n) && len(in) == 0:
		return "start"
	case isEllipse(n) && len(out) == 0:
		return "end"
	case isEllipse(n):
		return "external"
	}
	return "task"
}

func gap(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}

// Lay runs the engine on the plan.
func (p *Plan) Lay(tm *text.Measure) (layout.Result, error) {
	out, err := layout.Lay(p.Rows, tm, layout.Options{Config: p.cfg, Direction: p.Dir})
	if err != nil {
		return layout.Result{}, err
	}
	for _, f := range out.Findings {
		if f.Level == "error" {
			return layout.Result{}, unsupported("the engine's self-check failed: %s", f.Msg)
		}
	}
	return out.Result, nil
}

// Apply writes an engine result into the working copy, keeping the diagram's
// top-left corner where it was.
func (p *Plan) Apply(d *tidy.Diagram, r layout.Result, sd *segment.Diagram) {
	var origin geom.Point
	switch {
	case p.pool != nil:
		origin = geom.Point{X: p.pool.Box.X, Y: p.pool.Box.Y}
	case len(p.laneOrder) > 0:
		b := p.laneOrder[0].Box
		for _, l := range p.laneOrder[1:] {
			b = b.Union(l.Box)
		}
		origin = geom.Point{X: b.X, Y: b.Y - float64(r.PoolHeader)}
	default:
		b := sd.Bounds()
		minX, minY := math.Inf(1), math.Inf(1)
		for _, it := range r.Items {
			minX, minY = math.Min(minX, it.X), math.Min(minY, it.Y)
		}
		origin = geom.Point{X: b.X - minX, Y: b.Y - minY}
	}
	at := func(x, y float64) geom.Point { return geom.Point{X: x + origin.X, Y: y + origin.Y} }
	if p.pool != nil {
		if n := d.NodeFor(p.pool); n != nil {
			n.Box = geom.Rect{X: origin.X, Y: origin.Y, W: r.PoolW, H: r.PoolH}
		}
	}
	horizontal := r.Dir == layout.DirLR || r.Dir == layout.DirRL
	for i, pl := range r.Lanes {
		n := d.NodeFor(p.lanes[pl.ID])
		if n == nil {
			continue
		}
		hdr := float64(r.PoolHeader)
		if horizontal {
			o := at(hdr, r.LaneX[i])
			n.Box = geom.Rect{X: o.X, Y: o.Y, W: r.PoolW - hdr, H: r.LaneW[i]}
		} else {
			o := at(r.LaneX[i], hdr)
			n.Box = geom.Rect{X: o.X, Y: o.Y, W: r.LaneW[i], H: r.PoolH - hdr}
		}
	}
	shapes := map[string]layout.Shape{}
	for _, it := range r.Items {
		shapes[it.ID] = it.Shape
		v := p.nodes[it.ID]
		if v == nil {
			continue
		}
		n := d.NodeFor(v)
		if n == nil {
			continue
		}
		o := at(it.X, it.Y)
		n.Box = geom.Rect{X: o.X, Y: o.Y, W: it.W, H: it.H}
		if v.Style.Value("html", "0") == "1" && v.Style.Value("whiteSpace", "") != "wrap" {
			n.SetStyle = map[string]string{"whiteSpace": "wrap"}
		}
	}
	for _, pe := range r.Edges {
		e := p.edges[pe.ID]
		if e == nil {
			continue
		}
		w := d.WireFor(e.w)
		if w == nil {
			continue
		}
		var pts []geom.Point
		if len(pe.Pts) > 2 {
			for _, q := range pe.Pts[1 : len(pe.Pts)-1] {
				pts = append(pts, at(q[0], q[1]))
			}
		}
		exit, entry := pe.ExitFrac, pe.EntryFrac
		exitShape, entryShape := shapes[pe.Src], shapes[pe.Dst]
		pos, off := pe.LabelT, geom.Point{X: pe.LabelOff[0], Y: pe.LabelOff[1]}
		if e.reversed {
			for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
				pts[i], pts[j] = pts[j], pts[i]
			}
			exit, entry = entry, exit
			exitShape, entryShape = entryShape, exitShape
			pos = -pos
		}
		w.Points = pts
		setAnchor(w, "exit", exit, exitShape)
		setAnchor(w, "entry", entry, entryShape)
		w.Routed = true
		if pe.Label != nil {
			w.PlaceLabel(pos, off)
		}
	}
}

func setAnchor(w *tidy.Wire, prefix string, frac [2]float64, s layout.Shape) {
	w.Style.Set(prefix+"X", geom.Format(frac[0]))
	w.Style.Set(prefix+"Y", geom.Format(frac[1]))
	w.Style.Set(prefix+"Dx", "0")
	w.Style.Set(prefix+"Dy", "0")
	// A port off the midpoint of a diamond or ellipse side is computed on the
	// outline and rounded, which can leave it a hair inside the shape; draw.io
	// would project it back along a ray from the center and add a jog, so it is
	// told to use the point as given.
	if offOutline(s, frac) {
		w.Style.Set(prefix+"Perimeter", "0")
	} else {
		w.Style.Del(prefix + "Perimeter")
	}
}

func offOutline(s layout.Shape, frac [2]float64) bool {
	switch s {
	case layout.ShapeDiamond, layout.ShapeEllipse, layout.ShapeDoubleEllipse, layout.ShapeDashedEllipse:
	default:
		return false
	}
	for _, mid := range [][2]float64{{0.5, 0}, {1, 0.5}, {0.5, 1}, {0, 0.5}} {
		if frac == mid {
			return false
		}
	}
	return true
}
