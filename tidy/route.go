package tidy

import (
	"container/heap"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/view"
)

// Routing costs, in pixels of wire length. A bend costs as much as a short
// detour; crossing another wire costs more than going around a node; running
// on top of another wire is worse still.
const (
	bendCost    = 30
	crossCost   = 60
	overlapCost = 200
	obstacleGap = 10 // clearance kept around shapes
	stubLength  = 15 // straight run out of a port before the first bend
)

// Reroute gives a new route to every wire that crosses a shape or runs on top of
// another wire, keeping each new route only if the diagram scores better.
// Orthogonal wires get right-angle routes through fixed ports; other wires get
// waypoints around the obstacles and keep their style.
//
// A wire may only find a clean route once another one has moved, so passes
// repeat until one changes nothing.
func (d *Diagram) Reroute(log *Log) {
	for pass := 0; pass < 4; pass++ {
		moved := false
		for _, w := range d.Wires {
			if !d.broken(w) {
				continue
			}
			before := d.Measure().Score
			snap := d.Save()
			var ok bool
			if view.IsOrthogonal(w.Style) {
				ok = d.routeOrthogonal(w)
			} else {
				ok = d.routeStraight(w)
			}
			if !ok || d.Measure().Score >= before-1e-9 {
				d.Restore(snap)
				continue
			}
			w.Routed = true
			moved = true
			log.add("reroute", []string{w.V.Cell.ID}, "rerouted wire %s", w.V.Cell.ID)
		}
		if !moved {
			return
		}
	}
}

// broken reports whether a wire crosses a shape or overlaps another wire.
func (d *Diagram) broken(w *Wire) bool {
	pl := w.Path()
	if throughOwnEnd(pl, w) {
		return true
	}
	for _, n := range d.solidNodes() {
		if (w.Src != nil && w.Src.Within(n)) || (w.Dst != nil && w.Dst.Within(n)) {
			continue
		}
		if crosses(pl, n.Box) {
			return true
		}
	}
	for _, o := range d.Wires {
		if o != w && !shareEnd(w, o) && overlaps(pl, o.Path()) {
			return true
		}
	}
	return false
}

// obstacles returns the shapes a wire must go around: every solid node except
// the wire's own ends and the shapes containing them.
func (d *Diagram) obstacles(w *Wire) []geom.Rect {
	var out []geom.Rect
	for _, n := range d.solidNodes() {
		if (w.Src != nil && w.Src.Within(n)) || (w.Dst != nil && w.Dst.Within(n)) {
			continue
		}
		out = append(out, n.Box)
	}
	return out
}

func (d *Diagram) otherPaths(w *Wire) []geom.Polyline {
	var out []geom.Polyline
	for _, o := range d.Wires {
		if o != w {
			out = append(out, o.Path())
		}
	}
	return out
}

// side is a port on a box side: the relative anchor and the outward direction.
type side struct {
	ax, ay float64
	dir    geom.Point
}

var sides = []side{
	{0.5, 0, geom.Point{Y: -1}},
	{1, 0.5, geom.Point{X: 1}},
	{0.5, 1, geom.Point{Y: 1}},
	{0, 0.5, geom.Point{X: -1}},
}

type endChoice struct {
	port geom.Point // on the outline, or the free point
	stub geom.Point // first point outside the clearance
	side *side
}

func ends(n *Node, free *geom.Point) []endChoice {
	if n == nil {
		if free == nil {
			return nil
		}
		return []endChoice{{port: *free, stub: *free}}
	}
	var out []endChoice
	for i := range sides {
		s := &sides[i]
		p := geom.Point{X: n.Box.X + s.ax*n.Box.W, Y: n.Box.Y + s.ay*n.Box.H}
		out = append(out, endChoice{port: p, stub: p.Add(geom.Point{X: s.dir.X * stubLength, Y: s.dir.Y * stubLength}), side: s})
	}
	return out
}

// routeOrthogonal searches a grid of lines through the obstacle edges and ports
// for the cheapest right-angle route over every pair of ports.
func (d *Diagram) routeOrthogonal(w *Wire) bool {
	srcs, dsts := ends(w.Src, w.SrcPoint), ends(w.Dst, w.DstPoint)
	if len(srcs) == 0 || len(dsts) == 0 {
		return false
	}
	obs := d.obstacles(w)
	// The wire's own end shapes are obstacles too, except for the stub leaving
	// through the chosen port, so a route never doubles back through them.
	var own []geom.Rect
	if w.Src != nil {
		own = append(own, w.Src.Box)
	}
	if w.Dst != nil && w.Dst != w.Src {
		own = append(own, w.Dst.Box)
	}
	others := d.otherPaths(w)
	g := newGrid(obs, own, srcs, dsts)
	best := math.Inf(1)
	var bestPath geom.Polyline
	var bestS, bestD endChoice
	for _, s := range srcs {
		for _, t := range dsts {
			pl, cost := g.search(s.stub, t.stub, others)
			if pl == nil {
				continue
			}
			cost += stubCost(s, others) + stubCost(t, others)
			if cost < best {
				best, bestS, bestD = cost, s, t
				bestPath = pl
			}
		}
	}
	if bestPath == nil {
		return false
	}
	full := append(geom.Polyline{bestS.port}, bestPath...)
	full = append(full, bestD.port)
	full = simplify(full)
	full = unjog(full, g)
	w.Points = append([]geom.Point(nil), full[1:len(full)-1]...)
	setAnchor(w, "exit", bestS.side)
	setAnchor(w, "entry", bestD.side)
	return true
}

// stubCost charges the run from a port to its stub like any other piece of
// route, plus a port another wire already ends at, since two wires through one
// port read as one.
func stubCost(e endChoice, others []geom.Polyline) float64 {
	c := e.port.Dist(e.stub)
	seg := geom.Segment{A: e.port, B: e.stub}
	for _, o := range others {
		if len(o) > 0 && (o[0].Dist(e.port) < 1 || o[len(o)-1].Dist(e.port) < 1) {
			c += overlapCost
		}
		for _, os := range o.Segments() {
			if seg.Overlaps(os, 1) {
				c += overlapCost
			}
			if seg.Intersects(os) {
				c += crossCost
			}
		}
	}
	return c
}

// jogLength is the shortest step a route may take sideways; shorter ones are
// straightened when that stays clear of obstacles.
const jogLength = 12

// unjog removes short sideways steps by sliding the run before or after them
// into line, keeping the runs out of the ports leaving the way they did.
func unjog(pl geom.Polyline, g *grid) geom.Polyline {
	for changed := true; changed; {
		changed = false
		for i := 1; i+2 < len(pl); i++ {
			if pl[i].Dist(pl[i+1]) >= jogLength {
				continue
			}
			for _, cand := range []geom.Polyline{slideBefore(pl, i), slideAfter(pl, i)} {
				if cand != nil && routeClear(cand, pl, g) {
					pl = simplify(cand)
					changed = true
					break
				}
			}
			if changed {
				break
			}
		}
	}
	return pl
}

// slideBefore removes the jog from point i to i+1 by moving the run that ends
// at i into line with i+1. The first point is a port and never moves.
func slideBefore(pl geom.Polyline, i int) geom.Polyline {
	if i-1 < 1 {
		return nil
	}
	out := append(geom.Polyline(nil), pl...)
	if math.Abs(pl[i].Y-pl[i+1].Y) < 0.01 {
		out[i-1].X, out[i].X = pl[i+1].X, pl[i+1].X
	} else {
		out[i-1].Y, out[i].Y = pl[i+1].Y, pl[i+1].Y
	}
	return out
}

// slideAfter removes the jog from point i to i+1 by moving the run that starts
// at i+1 into line with i. The last point is a port and never moves.
func slideAfter(pl geom.Polyline, i int) geom.Polyline {
	if i+2 > len(pl)-2 {
		return nil
	}
	out := append(geom.Polyline(nil), pl...)
	if math.Abs(pl[i].Y-pl[i+1].Y) < 0.01 {
		out[i+1].X, out[i+2].X = pl[i].X, pl[i].X
	} else {
		out[i+1].Y, out[i+2].Y = pl[i].Y, pl[i].Y
	}
	return out
}

// routeClear checks a straightened route: the runs out of the ports keep their
// direction and some length, and every run between them stays clear of
// obstacles. The port runs sit inside their own shape's clearance and are not
// checked against it.
func routeClear(cand, orig geom.Polyline, g *grid) bool {
	n := len(cand)
	if dir(cand[0], cand[1]) != dir(orig[0], orig[1]) || dir(cand[n-2], cand[n-1]) != dir(orig[n-2], orig[n-1]) {
		return false
	}
	if cand[0].Dist(cand[1]) < 5 || cand[n-2].Dist(cand[n-1]) < 5 {
		return false
	}
	for k := 1; k+2 < n; k++ {
		if !g.open(cand[k], cand[k+1]) {
			return false
		}
	}
	return true
}

func dir(a, b geom.Point) geom.Point {
	return geom.Point{X: sign(b.X - a.X), Y: sign(b.Y - a.Y)}
}

func sign(v float64) float64 {
	switch {
	case v > 0.01:
		return 1
	case v < -0.01:
		return -1
	}
	return 0
}

func setAnchor(w *Wire, prefix string, s *side) {
	if s == nil {
		return
	}
	w.Style.Set(prefix+"X", geom.Format(s.ax))
	w.Style.Set(prefix+"Y", geom.Format(s.ay))
	w.Style.Set(prefix+"Dx", "0")
	w.Style.Set(prefix+"Dy", "0")
}

// simplify drops repeated points and points in the middle of a straight run.
func simplify(pl geom.Polyline) geom.Polyline {
	var out geom.Polyline
	for _, p := range pl {
		if len(out) > 0 && out[len(out)-1].Dist(p) < 0.01 {
			continue
		}
		if len(out) >= 2 {
			a, b := out[len(out)-2], out[len(out)-1]
			if (math.Abs(a.X-b.X) < 0.01 && math.Abs(b.X-p.X) < 0.01) || (math.Abs(a.Y-b.Y) < 0.01 && math.Abs(b.Y-p.Y) < 0.01) {
				out[len(out)-1] = p
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// grid is the routing graph: the crossings of horizontal and vertical lines
// through every obstacle edge (offset by the clearance) and every stub.
type grid struct {
	xs, ys []float64
	obs    []geom.Rect // inflated by the clearance
}

// ownGap is the clearance kept around a wire's own end shapes, smaller than
// obstacleGap so that two close shapes can still be wired to each other.
const ownGap = 5

func newGrid(obs, own []geom.Rect, srcs, dsts []endChoice) *grid {
	g := &grid{}
	xset, yset := map[float64]bool{}, map[float64]bool{}
	add := func(o geom.Rect, gap float64) {
		big := o.Inset(-gap)
		g.obs = append(g.obs, o.Inset(-gap+1))
		xset[big.X], xset[big.Right()] = true, true
		yset[big.Y], yset[big.Bottom()] = true, true
	}
	for _, o := range obs {
		add(o, obstacleGap)
	}
	for _, o := range own {
		add(o, ownGap)
	}
	for _, e := range append(append([]endChoice(nil), srcs...), dsts...) {
		xset[e.stub.X], yset[e.stub.Y] = true, true
	}
	for x := range xset {
		g.xs = append(g.xs, x)
	}
	for y := range yset {
		g.ys = append(g.ys, y)
	}
	sort.Float64s(g.xs)
	sort.Float64s(g.ys)
	// Lines midway between neighbors give routes a way through narrow gaps.
	g.xs = withMidpoints(g.xs)
	g.ys = withMidpoints(g.ys)
	return g
}

func withMidpoints(v []float64) []float64 {
	out := make([]float64, 0, 2*len(v))
	for i, x := range v {
		if i > 0 {
			out = append(out, (v[i-1]+x)/2)
		}
		out = append(out, x)
	}
	return out
}

func (g *grid) free(p geom.Point) bool {
	for _, o := range g.obs {
		if p.X > o.X && p.X < o.Right() && p.Y > o.Y && p.Y < o.Bottom() {
			return false
		}
	}
	return true
}

func (g *grid) open(a, b geom.Point) bool {
	s := geom.Segment{A: a, B: b}
	for _, o := range g.obs {
		if s.CrossesRect(o, 0) {
			return false
		}
	}
	return true
}

type state struct {
	i, j int
	dir  int // 0 none, 1 horizontal, 2 vertical
}

type item struct {
	s    state
	cost float64
	idx  int
}

type pq []*item

func (q pq) Len() int { return len(q) }
func (q pq) Less(i, j int) bool {
	if q[i].cost != q[j].cost {
		return q[i].cost < q[j].cost
	}
	if q[i].s.i != q[j].s.i {
		return q[i].s.i < q[j].s.i
	}
	if q[i].s.j != q[j].s.j {
		return q[i].s.j < q[j].s.j
	}
	return q[i].s.dir < q[j].s.dir
}
func (q pq) Swap(i, j int) { q[i], q[j] = q[j], q[i]; q[i].idx = i; q[j].idx = j }
func (q *pq) Push(x any)   { it := x.(*item); it.idx = len(*q); *q = append(*q, it) }
func (q *pq) Pop() any {
	old := *q
	it := old[len(old)-1]
	*q = old[:len(old)-1]
	return it
}

// search finds the cheapest right-angle route between two grid points.
func (g *grid) search(from, to geom.Point, others []geom.Polyline) (geom.Polyline, float64) {
	fi, fj := index(g.xs, from.X), index(g.ys, from.Y)
	ti, tj := index(g.xs, to.X), index(g.ys, to.Y)
	if fi < 0 || fj < 0 || ti < 0 || tj < 0 {
		return nil, 0
	}
	pt := func(i, j int) geom.Point { return geom.Point{X: g.xs[i], Y: g.ys[j]} }
	dist := map[state]float64{}
	prev := map[state]state{}
	start := state{fi, fj, 0}
	dist[start] = 0
	q := &pq{{s: start}}
	for q.Len() > 0 {
		it := heap.Pop(q).(*item)
		if it.cost > dist[it.s] {
			continue
		}
		s := it.s
		if s.i == ti && s.j == tj {
			var rev geom.Polyline
			for cur := s; ; cur = prev[cur] {
				rev = append(rev, pt(cur.i, cur.j))
				if cur == start {
					break
				}
			}
			out := make(geom.Polyline, len(rev))
			for k := range rev {
				out[k] = rev[len(rev)-1-k]
			}
			return out, it.cost
		}
		for _, mv := range [4][3]int{{1, 0, 1}, {-1, 0, 1}, {0, 1, 2}, {0, -1, 2}} {
			ni, nj := s.i+mv[0], s.j+mv[1]
			if ni < 0 || nj < 0 || ni >= len(g.xs) || nj >= len(g.ys) {
				continue
			}
			a, b := pt(s.i, s.j), pt(ni, nj)
			if !g.free(b) || !g.open(a, b) {
				continue
			}
			c := it.cost + a.Dist(b)
			if s.dir != 0 && s.dir != mv[2] {
				c += bendCost
			}
			seg := geom.Segment{A: a, B: b}
			for _, o := range others {
				for _, os := range o.Segments() {
					if seg.Intersects(os) {
						c += crossCost
					}
					if seg.Overlaps(os, 1) {
						c += overlapCost
					}
				}
			}
			ns := state{ni, nj, mv[2]}
			if old, seen := dist[ns]; !seen || c < old {
				dist[ns] = c
				prev[ns] = s
				heap.Push(q, &item{s: ns, cost: c})
			}
		}
	}
	return nil, 0
}

func index(v []float64, x float64) int {
	k := sort.SearchFloat64s(v, x)
	if k < len(v) && math.Abs(v[k]-x) < 1e-9 {
		return k
	}
	return -1
}

// routeStraight finds waypoints for a straight or curved wire: the shortest
// path from end to end through the corners of the obstacles, kept clear of
// them, with crossings of other wires charged.
func (d *Diagram) routeStraight(w *Wire) bool {
	var from, to geom.Point
	switch {
	case w.Src != nil:
		from = w.Src.Box.Center()
	case w.SrcPoint != nil:
		from = *w.SrcPoint
	default:
		return false
	}
	switch {
	case w.Dst != nil:
		to = w.Dst.Box.Center()
	case w.DstPoint != nil:
		to = *w.DstPoint
	default:
		return false
	}
	obs := d.obstacles(w)
	pts := []geom.Point{from, to}
	var clear []geom.Rect
	for _, o := range obs {
		big := o.Inset(-obstacleGap)
		clear = append(clear, o.Inset(-obstacleGap+1))
		pts = append(pts, geom.Point{X: big.X, Y: big.Y}, geom.Point{X: big.Right(), Y: big.Y},
			geom.Point{X: big.X, Y: big.Bottom()}, geom.Point{X: big.Right(), Y: big.Bottom()})
	}
	visible := func(a, b geom.Point) bool {
		s := geom.Segment{A: a, B: b}
		for _, o := range clear {
			if s.CrossesRect(o, 0) {
				return false
			}
		}
		return true
	}
	others := d.otherPaths(w)
	// Dijkstra over the visibility graph.
	n := len(pts)
	dist := make([]float64, n)
	prev := make([]int, n)
	done := make([]bool, n)
	for i := range dist {
		dist[i], prev[i] = math.Inf(1), -1
	}
	dist[0] = 0
	for {
		u := -1
		for i := 0; i < n; i++ {
			if !done[i] && !math.IsInf(dist[i], 1) && (u < 0 || dist[i] < dist[u]) {
				u = i
			}
		}
		if u < 0 || u == 1 {
			break
		}
		done[u] = true
		for v := 0; v < n; v++ {
			if done[v] || v == u || !visible(pts[u], pts[v]) {
				continue
			}
			c := dist[u] + pts[u].Dist(pts[v])
			seg := geom.Segment{A: pts[u], B: pts[v]}
			for _, o := range others {
				for _, os := range o.Segments() {
					if seg.Intersects(os) {
						c += crossCost
					}
				}
			}
			if c < dist[v] {
				dist[v], prev[v] = c, u
			}
		}
	}
	if math.IsInf(dist[1], 1) {
		return false
	}
	var rev []geom.Point
	for v := prev[1]; v > 0; v = prev[v] {
		rev = append(rev, pts[v])
	}
	w.Points = w.Points[:0]
	for k := len(rev) - 1; k >= 0; k-- {
		w.Points = append(w.Points, rev[k])
	}
	return true
}
