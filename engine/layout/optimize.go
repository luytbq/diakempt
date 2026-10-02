package layout

import (
	"math"
	"time"

	"github.com/luytbq/diakempt/engine/num"
)

// optimize improves a finished layout using geometry only.
//
// Placement and routing work on the abstract grid: a column has one x for every
// row of its lane, and a wire style is chosen per grid cell before any
// coordinate exists. That leaves slack the grid cannot see, such as a wire that
// detours around a gutter track when a straight path was free. Optimize works
// on the coordinates alone. Each round visits every part of the diagram in a
// fixed order and applies a move only when it lowers the cost and keeps the
// geometry valid. It stops after cfg.Optimize rounds or when a round changes
// nothing.
//
// Every move lowers the cost by at least minGain and the cost cannot go below
// zero, so the loop cannot oscillate and ends after finitely many moves. The
// round limit only bounds the running time.
//
// optimize runs in the virtual TD space, before Check and the axis swap, so one code
// path serves every direction and the self-check sees the final geometry.
func optimize(r Result, cfg Config, trace Trace) Result {
	if cfg.Optimize <= 0 {
		trace.Log("optimize: off")
		return r
	}
	t0 := time.Now()
	o := newOptimizer(r, cfg)
	o.trace = trace
	before := totalBends(o.r)
	rounds, moves := 0, 0
	for rounds < cfg.Optimize {
		rounds++
		n := o.round(rounds)
		moves += n
		if n == 0 {
			break
		}
	}
	trace.Log("optimize: %d rounds, %d moves, bends %d -> %d %s", rounds, moves, before, totalBends(o.r), Since(t0))
	return o.r
}

func totalBends(r Result) int {
	n := 0
	for _, e := range r.Edges {
		n += len(e.Pts) - 2
	}
	return n
}

// Cost weights. A bend is worth 100 px of wire, a crossing 300, a wire through
// someone else's label 200: removing two bends must never buy a new crossing.
const (
	costBend     = 100
	costCross    = 300
	costLabelHit = 200
	// minGain is the smallest cost drop that counts as an improvement, so that
	// rounding noise cannot keep the loop alive.
	minGain = 0.5
	// minEnd is the shortest first or last segment a move may leave, so that
	// the arrow head keeps room.
	minEnd = 8
)

type optimizer struct {
	r     Result
	cfg   Config
	boxes map[string]box
	pool  box
	trace Trace
}

func newOptimizer(r Result, cfg Config) *optimizer {
	o := &optimizer{r: r, cfg: cfg, boxes: map[string]box{}}
	edges := make([]PlacedEdge, len(r.Edges))
	for i, e := range r.Edges {
		e.Pts = append([][2]float64(nil), e.Pts...)
		if e.Label != nil {
			b := *e.Label
			e.Label = &b
		}
		edges[i] = e
	}
	o.r.Edges = edges
	for _, it := range r.Items {
		o.boxes[it.ID] = it.box()
	}
	o.pool = box{0, float64(r.PoolHeader + r.LaneHeader), r.PoolW, r.PoolH}
	return o
}

// round runs one pass over every kind of part, in this order: wires, items,
// lanes. A move on one part can open room for another, which is why the loop
// runs more than one round. It returns the number of moves applied.
func (o *optimizer) round(n int) int {
	wires := 0
	for i := range o.r.Edges {
		for o.straighten(i, n) {
			wires++
		}
	}
	items := o.shiftItems(n)
	lanes := o.shrinkLanes(n)
	o.trace.Log("optimize: round %d: %d wires straightened, %d items moved, %d lanes narrowed", n, wires, items, lanes)
	return wires + items + lanes
}

// straighten tries to remove two bends from edge i by sliding one inner segment
// until it lines up with the parallel segment two steps away, which makes the
// segment in between vanish. It applies the best such move and reports whether
// it applied one.
func (o *optimizer) straighten(i, round int) bool {
	e := &o.r.Edges[i]
	pts := e.Pts
	m := len(pts) - 1 // number of segments
	if m < 3 {
		return false
	}
	base := o.edgeCost(i, pts)
	var best [][2]float64
	bestCost := base - minGain
	bestS, bestT := 0, 0
	for s := 1; s <= m-2; s++ {
		for _, t := range []int{s - 2, s + 2} {
			if t < 0 || t > m-1 {
				continue
			}
			cand := slide(pts, s, segCoord(pts, t))
			if cand == nil || !o.valid(i, cand) {
				continue
			}
			if c := o.edgeCost(i, cand); c < bestCost {
				best, bestCost, bestS, bestT = cand, c, s, t
			}
		}
	}
	if best == nil {
		return false
	}
	o.trace.Log("optimize: round %d: %s: segment %d slid onto segment %d, bends %d -> %d, cost %.1f -> %.1f",
		round, e.ID, bestS, bestT, len(pts)-2, len(best)-2, base, bestCost)
	o.setPath(i, best)
	return true
}

// segCoord is the coordinate that stays fixed along segment s: x for a vertical
// segment, y for a horizontal one.
func segCoord(pts [][2]float64, s int) float64 {
	if vertical(pts[s], pts[s+1]) {
		return pts[s][0]
	}
	return pts[s][1]
}

func vertical(a, b [2]float64) bool { return math.Abs(a[0]-b[0]) < 0.01 }

// slide moves segment s of the path to coordinate c on its fixed axis and
// returns the cleaned path. It returns nil when the move would turn the first
// or last segment around or make it shorter than minEnd, since those segments
// start and end at the ports.
func slide(pts [][2]float64, s int, c float64) [][2]float64 {
	out := append([][2]float64(nil), pts...)
	axis := 1
	if vertical(pts[s], pts[s+1]) {
		axis = 0
	}
	out[s][axis], out[s+1][axis] = c, c
	n := len(out)
	if !sameDir(pts[0], pts[1], out[0], out[1]) || !sameDir(pts[n-2], pts[n-1], out[n-2], out[n-1]) {
		return nil
	}
	return cleanPath(out)
}

// sameDir reports whether segment a2-b2 points the same way as a1-b1 and is at
// least minEnd long, or keeps the original length if that was shorter.
func sameDir(a1, b1, a2, b2 [2]float64) bool {
	d1 := [2]float64{b1[0] - a1[0], b1[1] - a1[1]}
	d2 := [2]float64{b2[0] - a2[0], b2[1] - a2[1]}
	if float64(d1[0]*d2[0]) < 0 || float64(d1[1]*d2[1]) < 0 {
		return false
	}
	l1, l2 := math.Abs(d1[0])+math.Abs(d1[1]), math.Abs(d2[0])+math.Abs(d2[1])
	return l2 >= math.Min(l1, minEnd)
}

// cleanPath drops repeated points and points in the middle of a straight run,
// and keeps both end points.
func cleanPath(pts [][2]float64) [][2]float64 {
	var dedup [][2]float64
	for _, p := range pts {
		if len(dedup) > 0 {
			q := dedup[len(dedup)-1]
			if math.Abs(p[0]-q[0]) < 0.01 && math.Abs(p[1]-q[1]) < 0.01 {
				continue
			}
		}
		dedup = append(dedup, p)
	}
	if len(dedup) < 2 {
		return nil
	}
	out := [][2]float64{dedup[0]}
	for k := 1; k < len(dedup)-1; k++ {
		a, b, c := out[len(out)-1], dedup[k], dedup[k+1]
		if (vertical(a, b) && vertical(b, c)) || (math.Abs(a[1]-b[1]) < 0.01 && math.Abs(b[1]-c[1]) < 0.01) {
			continue
		}
		out = append(out, b)
	}
	return append(out, dedup[len(dedup)-1])
}

// edgeCost is the part of the total cost that depends on the path of edge i:
// its bends and length, its crossings with other wires, and how many other
// labels it runs through.
func (o *optimizer) edgeCost(i int, pts [][2]float64) float64 {
	c := float64(costBend * (len(pts) - 2))
	c += pathLen(pts)
	for j, f := range o.r.Edges {
		if j == i {
			continue
		}
		for k := 0; k+1 < len(pts); k++ {
			for q := 0; q+1 < len(f.Pts); q++ {
				if crosses(pts[k], pts[k+1], f.Pts[q], f.Pts[q+1]) {
					c += costCross
				}
			}
			if f.Label != nil && segHits(pts[k], pts[k+1], *f.Label) {
				c += costLabelHit
			}
		}
	}
	return c
}

// crosses reports whether two segments cross at a point strictly inside both.
func crosses(a1, b1, a2, b2 [2]float64) bool {
	v1, v2 := vertical(a1, b1), vertical(a2, b2)
	if v1 == v2 {
		return false
	}
	if !v1 {
		a1, b1, a2, b2 = a2, b2, a1, b1
	}
	x, y := a1[0], a2[1]
	lo1, hi1 := math.Min(a1[1], b1[1]), math.Max(a1[1], b1[1])
	lo2, hi2 := math.Min(a2[0], b2[0]), math.Max(a2[0], b2[0])
	return y > lo1+0.01 && y < hi1-0.01 && x > lo2+0.01 && x < hi2-0.01
}

// valid reports whether edge i may take path pts. Only segments that are new
// are checked against clearance rules, so a path the router built with a
// tighter clearance does not block every later move.
func (o *optimizer) valid(i int, pts [][2]float64) bool {
	return o.validFrom(i, pts, o.r.Edges[i].Pts)
}

// validFrom is valid with the edge's previous path given explicitly, for when
// the edge already holds its candidate path while other edges are checked.
func (o *optimizer) validFrom(i int, pts, prev [][2]float64) bool {
	e := o.r.Edges[i]
	old := map[[2][2]float64]bool{}
	for k := 0; k+1 < len(prev); k++ {
		old[[2][2]float64{prev[k], prev[k+1]}] = true
	}
	gap := float64(o.cfg.TrackGap)
	last := len(pts) - 2
	for k := 0; k+1 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		if !inside(a, o.pool) || !inside(b, o.pool) {
			return false
		}
		unchanged := old[[2][2]float64{a, b}]
		for _, it := range o.r.Items {
			bx := o.boxes[it.ID]
			end := (it.ID == e.Src && k == 0) || (it.ID == e.Dst && k == last)
			// An unchanged segment still has to stay out of items, since the
			// move may have brought an item to it; only the clearance rules
			// are relaxed for it.
			if end || unchanged {
				if segHits(a, b, shrink(bx, 1)) {
					return false
				}
				continue
			}
			if segHits(a, b, grow(bx, gap)) {
				return false
			}
		}
		if unchanged {
			continue
		}
		for j, f := range o.r.Edges {
			if j == i {
				continue
			}
			shared := f.Src == e.Src || f.Dst == e.Dst
			for q := 0; q+1 < len(f.Pts); q++ {
				if tooClose(a, b, f.Pts[q], f.Pts[q+1], gap, shared) {
					return false
				}
			}
		}
	}
	if e.Label != nil {
		if _, _, ok := anchor(pts, *e.Label, e.LabelOff); !ok {
			return false
		}
	}
	return true
}

func inside(p [2]float64, b box) bool {
	return p[0] >= b[0]-0.01 && p[0] <= b[2]+0.01 && p[1] >= b[1]-0.01 && p[1] <= b[3]+0.01
}

func grow(b box, d float64) box { return box{b[0] - d, b[1] - d, b[2] + d, b[3] + d} }

// tooClose reports whether two parallel segments run side by side closer than
// gap over a shared stretch. Wires that share an end may also run exactly on
// top of each other, since they merge into one line.
func tooClose(a1, b1, a2, b2 [2]float64, gap float64, shared bool) bool {
	v1, v2 := vertical(a1, b1), vertical(a2, b2)
	if v1 != v2 {
		return false
	}
	ax := 1
	if v1 {
		ax = 0
	}
	along := 1 - ax
	lo := math.Max(math.Min(a1[along], b1[along]), math.Min(a2[along], b2[along]))
	hi := math.Min(math.Max(a1[along], b1[along]), math.Max(a2[along], b2[along]))
	if hi-lo <= 1 {
		return false
	}
	d := math.Abs(a1[ax] - a2[ax])
	if shared && d < 0.01 {
		return false
	}
	return d < gap
}

// anchor finds where a label box sits along a path: the point on the path
// nearest the label centre, and the path distance to it. ok is false when the
// label would end up farther from its wire than before, which means the wire
// moved away from its label.
func anchor(pts [][2]float64, lb box, oldOff [2]float64) (p [2]float64, dist float64, ok bool) {
	cx, cy := (lb[0]+lb[2])/2, (lb[1]+lb[3])/2
	bestD := math.Inf(1)
	acc := 0.0
	for k := 0; k+1 < len(pts); k++ {
		a, b := pts[k], pts[k+1]
		q := [2]float64{
			math.Min(math.Max(cx, math.Min(a[0], b[0])), math.Max(a[0], b[0])),
			math.Min(math.Max(cy, math.Min(a[1], b[1])), math.Max(a[1], b[1])),
		}
		if d := math.Abs(q[0]-cx) + math.Abs(q[1]-cy); d < bestD {
			bestD, p = d, q
			dist = acc + math.Abs(q[0]-a[0]) + math.Abs(q[1]-a[1])
		}
		acc += math.Abs(b[0]-a[0]) + math.Abs(b[1]-a[1])
	}
	return p, dist, bestD <= math.Abs(oldOff[0])+math.Abs(oldOff[1])+0.5
}

// setPath gives edge i a new path and re-anchors its label, so the label box
// stays where it was and draw.io still draws it there.
func (o *optimizer) setPath(i int, pts [][2]float64) {
	e := &o.r.Edges[i]
	e.Pts = pts
	if e.Label == nil {
		return
	}
	p, dist, _ := anchor(pts, *e.Label, e.LabelOff)
	lb := *e.Label
	if total := pathLen(pts); total != 0 {
		e.LabelT = num.Round(float64(2*dist)/total-1, 4)
	}
	e.LabelOff = [2]float64{num.Round((lb[0]+lb[2])/2-p[0], 2), num.Round((lb[1]+lb[3])/2-p[1], 2)}
}
