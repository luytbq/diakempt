// Package communication lays out UML communication diagrams: objects and
// actors joined by links that carry numbered messages. The diagram reads left
// to right from the actor that sends the first message: each object one
// column right of the object it is first reached from, the objects of a
// column in the order their first message is sent, every gap wide and tall
// enough for the message labels on the links crossing it.
package communication

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
)

// Spacing, in pixels.
const (
	minGapX  = 80 // between two columns
	minGapY  = 40 // between two objects in a column
	labelPad = 20 // around a message label
	loopUp   = 30 // how far a self link rises above its object
	loopHalf = 20 // half the width of a self link
	hubSize  = 3  // neighbors a hub may have before it gets columns on both sides
)

// Unsupported explains why a diagram cannot be laid out as a communication
// diagram.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

// less orders message numbers as the messages are sent: 1 < 1.1 < 1.2 < 2.
func less(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

type link struct {
	w *tidy.Wire
	// src and dst are the top-level shapes the ends sit in.
	src, dst *tidy.Node
	number   []int
	text     geom.Point // size of its label
}

// Layout places the objects and actors of a communication diagram and
// routes the links between them.
func Layout(d *tidy.Diagram) error {
	for _, n := range d.Nodes {
		if n.Container {
			return &Unsupported{fmt.Sprintf("container %s in a communication diagram", n.V.ID)}
		}
	}
	return layout(d, true)
}

// LayoutDeployment places the nodes of a deployment diagram the same way,
// starting at the best connected node. A node holding artifacts moves with
// them, and nodes keep their size.
func LayoutDeployment(d *tidy.Diagram) error {
	return layout(d, false)
}

// layout lays a diagram out in columns. Top-level shapes are placed; what
// they hold moves with them. objects selects a communication diagram: the
// layout starts at the first message, and objects of a column share a width.
func layout(d *tidy.Diagram, objects bool) error {
	var nodes, others []*tidy.Node
	unit := map[*tidy.Node]*tidy.Node{}
	for _, n := range d.Nodes {
		top := n
		for top.Parent != nil {
			top = top.Parent
		}
		unit[n] = top
		switch {
		case n.Text || n.V.Shape == "note" || n.Overlay:
			others = append(others, n)
		case n.Parent == nil:
			nodes = append(nodes, n)
		}
	}
	if len(nodes) < 2 {
		return &Unsupported{"fewer than two objects"}
	}
	var links []*link
	adj := map[*tidy.Node][]*link{}
	for _, w := range d.Wires {
		if w.Src == nil || w.Dst == nil {
			return &Unsupported{fmt.Sprintf("link %s has a free end", w.V.Cell.ID)}
		}
		if w.Src.Text || w.Dst.Text {
			return &Unsupported{fmt.Sprintf("link %s ends on a text", w.V.Cell.ID)}
		}
		l := &link{w: w, number: detect.MessageNumber(w.V.Text), src: unit[w.Src], dst: unit[w.Dst]}
		lw, lh := d.TextSize(w.V.Text, w.Style)
		l.text = geom.Point{X: lw, Y: lh}
		links = append(links, l)
		adj[l.src] = append(adj[l.src], l)
		if l.dst != l.src {
			adj[l.dst] = append(adj[l.dst], l)
		}
	}
	byNumber := func(ls []*link) {
		sort.SliceStable(ls, func(i, j int) bool {
			a, b := ls[i].number, ls[j].number
			switch {
			case a == nil:
				return false
			case b == nil:
				return true
			}
			return less(a, b)
		})
	}
	for _, ls := range adj {
		byNumber(ls)
	}
	ordered := append([]*link(nil), links...)
	byNumber(ordered)
	// A communication diagram starts at the actor of the first message, else
	// at either end of it; any other diagram at the best connected shape.
	var root *tidy.Node
	if objects && len(ordered) > 0 && ordered[0].number != nil {
		l := ordered[0]
		root = l.src
		if l.dst.V.Shape == "umlActor" && l.src.V.Shape != "umlActor" {
			root = l.dst
		}
	} else {
		for _, n := range nodes {
			if root == nil || len(adj[n]) > len(adj[root]) {
				root = n
			}
		}
	}
	// Columns: breadth first from the root, following links in message
	// order, so each column lists its objects in the order they are reached.
	// A hub with many neighbors, as a deployment's server is, gets columns on
	// both sides: its first neighbors go right, the rest left.
	col := map[*tidy.Node]int{root: 0}
	columns := map[int][]*tidy.Node{0: {root}}
	lo, hi := 0, 0
	queue := []*tidy.Node{root}
	split := !objects && len(adj[root]) > hubSize
	reached := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, l := range adj[n] {
			o := l.dst
			if o == n {
				o = l.src
			}
			if _, seen := col[o]; seen {
				continue
			}
			step := 1
			switch {
			case col[n] < 0:
				step = -1
			case n == root && split:
				if reached >= (len(adj[root])+1)/2 {
					step = -1
				}
				reached++
			}
			c := col[n] + step
			col[o] = c
			lo, hi = min(lo, c), max(hi, c)
			columns[c] = append(columns[c], o)
			queue = append(queue, o)
		}
	}
	for _, n := range nodes {
		if _, ok := col[n]; !ok {
			col[n] = 0
			columns[0] = append(columns[0], n)
		}
	}
	outer := func(a, b int) int {
		if abs(a) > abs(b) {
			return a
		}
		return b
	}
	// Room for labels: a column gap fits the widest label of the links
	// crossing it, and objects in a column sit far enough apart that the
	// labels of links fanning out to them do not touch. gapX[c] is the gap
	// right of column c.
	gapX := map[int]float64{}
	tallest := map[int]float64{}
	loopRoom := map[*tidy.Node]float64{}
	for _, l := range links {
		a, b := col[l.src], col[l.dst]
		if l.src == l.dst {
			loopRoom[l.src] = math.Max(loopRoom[l.src], loopUp+l.text.Y)
			continue
		}
		if a > b {
			a, b = b, a
		}
		if b == a+1 {
			gapX[a] = math.Max(gapX[a], l.text.X+2*labelPad)
		}
		o := outer(a, b)
		tallest[o] = math.Max(tallest[o], l.text.Y)
	}
	// Heights first, from the root outward, so each column can center on
	// the column it is reached from; then left to right.
	width := map[int]float64{}
	center := map[*tidy.Node]geom.Point{}
	order := []int{0}
	for k := 1; k <= max(-lo, hi); k++ {
		order = append(order, k, -k)
	}
	for _, c := range order {
		ns := columns[c]
		if len(ns) == 0 {
			continue
		}
		// One width per column, so a link reaching an object at the middle
		// of its side stays clear of the others in the column. Actors keep
		// their figure's size.
		w := 0.0
		for _, n := range ns {
			w = math.Max(w, n.Box.W)
		}
		width[c] = w
		for _, n := range ns {
			if objects && n.V.Shape != "umlActor" {
				n.Box = geom.Rect{X: n.Box.Center().X - w/2, Y: n.Box.Y, W: w, H: n.Box.H}
			}
		}
		y := 0.0
		for _, n := range ns {
			y += loopRoom[n]
			center[n] = geom.Point{Y: y + n.Box.H/2}
			step := math.Max(n.Box.H+minGapY, 2*(tallest[c]+labelPad))
			y += step
		}
		// The column centers on the objects it is reached from.
		if c == 0 {
			continue
		}
		from := c - 1
		if c < 0 {
			from = c + 1
		}
		sum, cnt := 0.0, 0
		for _, n := range ns {
			for _, l := range adj[n] {
				o := l.dst
				if o == n {
					o = l.src
				}
				if col[o] == from {
					sum += center[o].Y
					cnt++
				}
			}
		}
		if cnt > 0 {
			first, last := center[ns[0]].Y, center[ns[len(ns)-1]].Y
			shift := sum/float64(cnt) - (first+last)/2
			if top := center[ns[0]].Y - ns[0].Box.H/2 - loopRoom[ns[0]]; top+shift < 0 {
				shift = -top
			}
			for _, n := range ns {
				center[n] = geom.Point{Y: center[n].Y + shift}
			}
		}
	}
	x := 0.0
	for c := lo; c <= hi; c++ {
		for _, n := range columns[c] {
			center[n] = geom.Point{X: x + width[c]/2, Y: center[n].Y}
		}
		x += width[c] + math.Max(minGapX, gapX[c])
	}
	moved := map[*tidy.Node]geom.Point{}
	origin := geom.Point{X: math.Inf(1), Y: math.Inf(1)}
	for _, n := range d.Nodes {
		origin.X, origin.Y = math.Min(origin.X, n.Box.X), math.Min(origin.Y, n.Box.Y)
	}
	for _, n := range nodes {
		c := center[n]
		moved[n] = geom.Point{X: c.X - n.Box.Center().X, Y: c.Y - n.Box.Center().Y}
		n.Box = geom.Rect{X: c.X - n.Box.W/2, Y: c.Y - n.Box.H/2, W: n.Box.W, H: n.Box.H}
	}
	// What a top-level shape holds moves with it.
	for _, n := range d.Nodes {
		if u := unit[n]; u != n && !n.Text && !n.Overlay {
			n.Box = n.Box.Move(moved[u])
		}
	}
	for _, n := range others {
		var best *tidy.Node
		bd := math.Inf(1)
		for m := range moved {
			if dd := gap(n.Orig, m.Orig); dd < bd {
				best, bd = m, dd
			}
		}
		if best != nil {
			n.Box = n.Box.Move(moved[best])
		}
	}
	now := geom.Point{X: math.Inf(1), Y: math.Inf(1)}
	for _, n := range d.Nodes {
		now.X, now.Y = math.Min(now.X, n.Box.X), math.Min(now.Y, n.Box.Y)
	}
	by := geom.Point{X: origin.X - now.X, Y: origin.Y - now.Y}
	for _, n := range d.Nodes {
		n.Box = n.Box.Move(by)
	}
	// Links: straight, from the middle of the side of one object facing the
	// other; a self link loops above its object; one that would still cross
	// another object goes around it.
	var detour []*tidy.Wire
	for _, l := range links {
		w := l.w
		for _, k := range []string{"exitX", "exitY", "exitDx", "exitDy", "exitPerimeter", "entryX", "entryY", "entryDx", "entryDy", "entryPerimeter"} {
			w.Style.Del(k)
		}
		w.Routed = true
		w.Points = nil
		if l.src == l.dst && w.Src == w.Dst {
			b := w.Src.Box
			c := b.Center().X
			w.Points = []geom.Point{{X: c - loopHalf, Y: b.Y - loopUp}, {X: c + loopHalf, Y: b.Y - loopUp}}
			port(w, "exit", (c-loopHalf-b.X)/b.W, 0)
			port(w, "entry", (c+loopHalf-b.X)/b.W, 0)
			continue
		}
		a, b := col[l.src], col[l.dst]
		if a != b {
			srcX, dstX := 1.0, 0.0
			if a > b {
				srcX, dstX = 0, 1
			}
			port(w, "exit", srcX, 0.5)
			port(w, "entry", dstX, 0.5)
		}
		seg := geom.Segment{A: endPoint(w.Src.Box, a < b), B: endPoint(w.Dst.Box, b < a)}
		if a == b {
			seg = geom.Segment{A: w.Src.Box.Center(), B: w.Dst.Box.Center()}
		}
		for _, n := range nodes {
			if n != l.src && n != l.dst && seg.CrossesRect(n.Box, -4) {
				detour = append(detour, w)
				break
			}
		}
	}
	for _, w := range detour {
		d.RouteWire(w)
	}
	return nil
}

func port(w *tidy.Wire, prefix string, x, y float64) {
	w.Style.Set(prefix+"X", geom.Format(x))
	w.Style.Set(prefix+"Y", geom.Format(y))
	w.Style.Set(prefix+"Dx", "0")
	w.Style.Set(prefix+"Dy", "0")
}

// endPoint is the middle of a box's right side, or of its left side.
func endPoint(b geom.Rect, right bool) geom.Point {
	if right {
		return geom.Point{X: b.Right(), Y: b.Center().Y}
	}
	return geom.Point{X: b.X, Y: b.Center().Y}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// gap returns the distance between two boxes, zero when they touch or overlap.
func gap(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}
