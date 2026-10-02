package layout

import (
	"fmt"
	"math"

	"github.com/luytbq/diakempt/engine/num"
)

// costAttachGap is the cost per pixel by which the gap between a db or note and
// its node differs from --attach-gap, in either direction: a note left far away
// is pulled back, and no other pull can push it closer than the distance the
// user chose.
const costAttachGap = 1

// costGravity is a weak pull, per pixel, from an item towards the centre of its
// lane. An item between two straight wires can slide without changing their
// total length, so without this pull it would stay at the lane edge and keep
// the lane wide. It is small enough never to outweigh a pixel of wire.
const costGravity = 0.05

// mover is a set of items that move together: a node with the db and notes
// attached to it, or a single db or note on its own.
type mover struct {
	idx  []int // indexes into Result.Items; the first is the node or the lone attachment
	ids  map[string]bool
	lone bool // a db or note moving without its node
}

// movers lists every node with its attachments, then every attachment alone,
// in table order.
func (o *optimizer) movers() []mover {
	var out []mover
	byAttach := map[string][]int{}
	for i, it := range o.r.Items {
		if it.Attach != "" {
			byAttach[it.Attach] = append(byAttach[it.Attach], i)
		}
	}
	for i, it := range o.r.Items {
		if it.Attach != "" {
			continue
		}
		m := mover{idx: append([]int{i}, byAttach[it.ID]...), ids: map[string]bool{it.ID: true}}
		for _, a := range byAttach[it.ID] {
			m.ids[o.r.Items[a].ID] = true
		}
		out = append(out, m)
	}
	for i, it := range o.r.Items {
		if it.Attach != "" {
			out = append(out, mover{idx: []int{i}, ids: map[string]bool{it.ID: true}, lone: true})
		}
	}
	return out
}

// shiftItems tries to move every mover sideways and returns how many moves it
// applied.
func (o *optimizer) shiftItems(round int) int {
	n := 0
	for _, m := range o.movers() {
		if o.shift(m, round) {
			n++
		}
	}
	return n
}

// shift moves m sideways to the candidate position with the lowest cost, when
// that beats the current position. Candidates are as far left and as far right
// as the obstacles in m's own row band allow, and every position that lines a
// connected wire up straight.
func (o *optimizer) shift(m mover, round int) bool {
	left, right := o.room(m)
	cands := []float64{}
	if left < -0.5 {
		cands = append(cands, left)
	}
	if right > 0.5 {
		cands = append(cands, right)
	}
	cands = append(cands, o.alignments(m, left, right)...)
	edges := o.touching(m)
	base := o.moverCost(m, edges)
	bestDX, bestCost, found := 0.0, base-minGain, false
	var bestPaths map[int][][2]float64
	for _, dx := range cands {
		dx = num.Round(dx, 2)
		if math.Abs(dx) < 0.01 {
			continue
		}
		paths, ok := o.tryShift(m, edges, dx)
		if !ok {
			if dx, paths, ok = o.bisectShift(m, edges, dx); !ok {
				continue
			}
		}
		if c := o.moverCostWith(m, edges, dx, paths); c < bestCost {
			bestDX, bestCost, bestPaths, found = dx, c, paths, true
		}
	}
	if !found {
		return false
	}
	o.trace.Log("optimize: round %d: moved %s %.2f px sideways, cost %.1f -> %.1f",
		round, o.r.Items[m.idx[0]].ID, bestDX, base, bestCost)
	o.moveItems(m, bestDX)
	for i, p := range bestPaths {
		o.setPath(i, p)
	}
	return true
}

// room returns how far m can move left (a negative number) and right before it
// comes within a gap of something in its own row band: another item, a label,
// a wire that does not belong to m, or the lane border.
func (o *optimizer) room(m mover) (left, right float64) {
	left, right = math.Inf(-1), math.Inf(1)
	var obstacles []box
	for i, it := range o.r.Items {
		if !m.ids[it.ID] {
			obstacles = append(obstacles, grow(o.boxes[o.r.Items[i].ID], float64(o.cfg.MinGutter)))
		}
	}
	for _, e := range o.r.Edges {
		if e.Label != nil {
			obstacles = append(obstacles, grow(*e.Label, 4))
		}
		// A wire touching m moves only its end points on m, and the bend next
		// to a vertical end segment. Its other segments stay and block m like
		// any other wire.
		moving := movingPoints(e, m.ids)
		for k := 0; k+1 < len(e.Pts); k++ {
			if moving[k] || moving[k+1] {
				continue
			}
			a, b := e.Pts[k], e.Pts[k+1]
			seg := box{math.Min(a[0], b[0]), math.Min(a[1], b[1]), math.Max(a[0], b[0]), math.Max(a[1], b[1])}
			obstacles = append(obstacles, grow(seg, float64(o.cfg.TrackGap)))
		}
	}
	margin := float64(o.cfg.GutterMargin)
	for _, i := range m.idx {
		it := o.r.Items[i]
		b := o.boxes[it.ID]
		lx := o.r.LaneX[it.Lane]
		left = math.Max(left, lx+margin-b[0])
		right = math.Min(right, lx+o.r.LaneW[it.Lane]-margin-b[2])
		for _, ob := range obstacles {
			if ob[1] >= b[3] || ob[3] <= b[1] {
				continue
			}
			switch {
			case ob[2] <= b[0]+0.01:
				left = math.Max(left, ob[2]-b[0])
			case ob[0] >= b[2]-0.01:
				right = math.Min(right, ob[0]-b[2])
			}
		}
	}
	return math.Min(left, 0), math.Max(right, 0)
}

// alignments returns the shifts, within [left, right], that line a connected
// wire up so that it no longer needs a bend: the node's port moves to the x of
// the wire's far end.
func (o *optimizer) alignments(m mover, left, right float64) []float64 {
	if m.lone {
		return nil
	}
	id := o.r.Items[m.idx[0]].ID
	var out []float64
	for _, e := range o.r.Edges {
		p := e.Pts
		if len(p) < 3 {
			continue
		}
		var dx float64
		switch {
		case e.Src == id && vertical(p[0], p[1]):
			dx = p[len(p)-1][0] - p[0][0]
		case e.Dst == id && vertical(p[len(p)-2], p[len(p)-1]):
			dx = p[0][0] - p[len(p)-1][0]
		default:
			continue
		}
		if dx >= left && dx <= right {
			out = append(out, dx)
		}
	}
	return out
}

// touching returns the indexes of the edges with an end on m.
func (o *optimizer) touching(m mover) []int {
	var out []int
	for i, e := range o.r.Edges {
		if m.ids[e.Src] || m.ids[e.Dst] {
			out = append(out, i)
		}
	}
	return out
}

// movingPoints marks the points of e's path that move when the items in ids
// move: the same points shiftedPath shifts.
func movingPoints(e PlacedEdge, ids map[string]bool) []bool {
	p := e.Pts
	n := len(p)
	out := make([]bool, n)
	if ids[e.Src] && ids[e.Dst] {
		for k := range out {
			out[k] = true
		}
		return out
	}
	if ids[e.Src] {
		out[0] = true
		if n > 1 && vertical(p[0], p[1]) {
			out[1] = true
		}
	}
	if ids[e.Dst] {
		out[n-1] = true
		if n > 1 && vertical(p[n-2], p[n-1]) {
			out[n-2] = true
		}
	}
	return out
}

// shiftedPath is the path of edge e after the items in ids move by dx. An end
// on a moved item moves with it, and when that end segment is vertical the
// bend after it moves too, so the segment stays vertical. It returns nil when
// the path cannot follow, which is when a straight vertical wire would have to
// turn oblique.
func shiftedPath(e PlacedEdge, ids map[string]bool, dx float64) [][2]float64 {
	p := append([][2]float64(nil), e.Pts...)
	n := len(p)
	src, dst := ids[e.Src], ids[e.Dst]
	if src && dst {
		for k := range p {
			p[k][0] += dx
		}
		return p
	}
	if n == 2 && vertical(p[0], p[1]) {
		return nil
	}
	if src {
		if vertical(p[0], p[1]) {
			p[1][0] += dx
		}
		p[0][0] += dx
	}
	if dst {
		if vertical(p[n-2], p[n-1]) {
			p[n-2][0] += dx
		}
		p[n-1][0] += dx
	}
	if !sameDir(e.Pts[0], e.Pts[1], p[0], p[1]) || !sameDir(e.Pts[n-2], e.Pts[n-1], p[n-2], p[n-1]) {
		return nil
	}
	return cleanPath(p)
}

// tryShift moves m by dx, checks that the new positions and the new paths of
// the touching edges are valid, and puts everything back. It returns the new
// paths. Every touching edge takes its new path before any is checked, so two
// edges of m are checked against each other where they will be, not where
// they were.
func (o *optimizer) tryShift(m mover, edges []int, dx float64) (map[int][][2]float64, bool) {
	paths := map[int][][2]float64{}
	prev := map[int][][2]float64{}
	for _, i := range edges {
		p := shiftedPath(o.r.Edges[i], m.ids, dx)
		if p == nil {
			return nil, false
		}
		paths[i], prev[i] = p, o.r.Edges[i].Pts
	}
	o.moveItems(m, dx)
	for _, i := range edges {
		o.r.Edges[i].Pts = paths[i]
	}
	defer func() {
		o.moveItems(m, -dx)
		for _, i := range edges {
			o.r.Edges[i].Pts = prev[i]
		}
	}()
	for _, i := range edges {
		if !o.validFrom(i, paths[i], prev[i]) {
			return nil, false
		}
	}
	return paths, o.itemsFree(m, edges, paths)
}

// bisectShift returns the longest valid shift between 0 and dx, found by
// halving, or false when even a short shift is invalid. Most rules that stop a
// long shift, such as a wire it would run into, leave the shorter shifts free.
func (o *optimizer) bisectShift(m mover, edges []int, dx float64) (float64, map[int][][2]float64, bool) {
	lo, hi := 0.0, dx
	var best map[int][][2]float64
	for step := 0; step < 12; step++ {
		mid := num.Round((lo+hi)/2, 2)
		if math.Abs(mid-lo) < 0.5 {
			break
		}
		if paths, ok := o.tryShift(m, edges, mid); ok {
			lo, best = mid, paths
		} else {
			hi = mid
		}
	}
	return lo, best, best != nil
}

// itemsFree reports whether m's items, at their current position, keep clear
// of every other item, every label, and every wire that does not touch m.
func (o *optimizer) itemsFree(m mover, edges []int, paths map[int][][2]float64) bool {
	gap := float64(o.cfg.TrackGap)
	for _, i := range m.idx {
		b := o.boxes[o.r.Items[i].ID]
		lx := o.r.LaneX[o.r.Items[i].Lane]
		if b[0] < lx || b[2] > lx+o.r.LaneW[o.r.Items[i].Lane] {
			return false
		}
		for _, it := range o.r.Items {
			if !m.ids[it.ID] && area(grow(b, gap), o.boxes[it.ID]) > 0 {
				return false
			}
		}
		for j, e := range o.r.Edges {
			if e.Label != nil && area(b, *e.Label) > 0 {
				return false
			}
			if _, mine := paths[j]; mine {
				continue
			}
			for k := 0; k+1 < len(e.Pts); k++ {
				if segHits(e.Pts[k], e.Pts[k+1], grow(b, gap)) {
					return false
				}
			}
		}
	}
	return true
}

// moveItems shifts the items of m by dx.
func (o *optimizer) moveItems(m mover, dx float64) {
	for _, i := range m.idx {
		it := &o.r.Items[i]
		it.X = num.Round(it.X+dx, 2)
		o.boxes[it.ID] = it.box()
	}
}

// moverCost is the cost that depends on where m sits: the touching edges and,
// for a db or note, the gap to its node.
func (o *optimizer) moverCost(m mover, edges []int) float64 {
	c := 0.0
	for _, i := range edges {
		c += o.edgeCost(i, o.r.Edges[i].Pts)
	}
	return c + o.attachGap(m) + o.gravity(m)
}

// gravity is the pull of m's first item towards the centre of its lane.
func (o *optimizer) gravity(m mover) float64 {
	it := o.r.Items[m.idx[0]]
	b := o.boxes[it.ID]
	mid := o.r.LaneX[it.Lane] + o.r.LaneW[it.Lane]/2
	return float64(costGravity * math.Abs((b[0]+b[2])/2-mid))
}

// moverCostWith is moverCost with m moved by dx and the edges on new paths.
func (o *optimizer) moverCostWith(m mover, edges []int, dx float64, paths map[int][][2]float64) float64 {
	prev := map[int][][2]float64{}
	o.moveItems(m, dx)
	for _, i := range edges {
		prev[i], o.r.Edges[i].Pts = o.r.Edges[i].Pts, paths[i]
	}
	defer func() {
		o.moveItems(m, -dx)
		for _, i := range edges {
			o.r.Edges[i].Pts = prev[i]
		}
	}()
	c := 0.0
	for _, i := range edges {
		c += o.edgeCost(i, paths[i])
	}
	return c + o.attachGap(m) + o.gravity(m)
}

// attachGap is how far the gap between a lone db or note and its node is from
// --attach-gap.
func (o *optimizer) attachGap(m mover) float64 {
	if !m.lone {
		return 0
	}
	it := o.r.Items[m.idx[0]]
	a, n := o.boxes[it.ID], o.boxes[it.Attach]
	gap := math.Max(a[0]-n[2], n[0]-a[2])
	return float64(costAttachGap * math.Abs(gap-float64(o.cfg.AttachGap)))
}

// shrinkLanes narrows every lane to its content plus a margin on each side,
// keeping the lane at least as wide as its minimum, and moves every later lane
// left by the width saved. It returns how many lanes it narrowed.
func (o *optimizer) shrinkLanes(round int) int {
	n := 0
	margin := float64(o.cfg.MinGutter)
	for li := range o.r.LaneX {
		lx, lw := o.r.LaneX[li], o.r.LaneW[li]
		lo, hi, ok := o.laneContent(li)
		if !ok {
			continue
		}
		w := math.Max(o.laneMinW(li), hi-lo+float64(2*margin))
		if lw-w < 0.5 {
			continue
		}
		// Centre the content in the narrower lane, then close the gap on the
		// right by moving everything after this lane.
		inner := num.Round(lx+(w-(hi-lo))/2-lo, 2)
		saved := num.Round(lw-w, 2)
		o.shiftRegion(lx, lx+lw, inner)
		o.shiftRegion(lx+lw, math.Inf(1), -saved)
		o.r.LaneW[li] = num.Round(lw-saved, 2)
		for lj := li + 1; lj < len(o.r.LaneX); lj++ {
			o.r.LaneX[lj] = num.Round(o.r.LaneX[lj]-saved, 2)
		}
		// The pool width is the running sum of the lane widths, added up the
		// same way geometry does, so that merge, which adds them up again from
		// the file, arrives at the same number.
		o.r.PoolW = 0
		for _, w := range o.r.LaneW {
			o.r.PoolW += w
		}
		o.pool[2] = o.r.PoolW
		o.trace.Log("optimize: round %d: lane %s narrowed by %.2f px", round, o.laneName(li), saved)
		n++
	}
	return n
}

func (o *optimizer) laneName(li int) string {
	if li < len(o.r.Lanes) {
		return o.r.Lanes[li].ID
	}
	return fmt.Sprint(li)
}

func (o *optimizer) laneMinW(li int) float64 {
	if li < len(o.r.LaneMinW) {
		return o.r.LaneMinW[li]
	}
	return 0
}

// laneContent is the horizontal extent of everything inside lane li: its items,
// the vertical wire segments and bends inside it, and the labels whose centre
// lies inside it.
func (o *optimizer) laneContent(li int) (lo, hi float64, ok bool) {
	lx, rx := o.r.LaneX[li], o.r.LaneX[li]+o.r.LaneW[li]
	lo, hi = math.Inf(1), math.Inf(-1)
	add := func(a, b float64) {
		lo, hi, ok = math.Min(lo, a), math.Max(hi, b), true
	}
	in := func(x float64) bool { return x > lx+0.01 && x < rx-0.01 }
	for _, it := range o.r.Items {
		if it.Lane == li {
			b := o.boxes[it.ID]
			add(b[0], b[2])
		}
	}
	for _, e := range o.r.Edges {
		for k := 1; k+1 < len(e.Pts); k++ {
			if x := e.Pts[k][0]; in(x) {
				add(x, x)
			}
		}
		if e.Label != nil {
			if c := (e.Label[0] + e.Label[2]) / 2; in(c) {
				add(e.Label[0], e.Label[2])
			}
		}
	}
	return lo, hi, ok
}

// shiftRegion moves by d everything whose x lies in [x0, x1): items, wire
// points, and the labels anchored there. Labels move with their anchor point
// and are then re-anchored on the moved path.
func (o *optimizer) shiftRegion(x0, x1, d float64) {
	if math.Abs(d) < 0.005 {
		return
	}
	inR := func(x float64) bool { return x >= x0-0.005 && x < x1-0.005 }
	for i := range o.r.Items {
		it := &o.r.Items[i]
		if inR(it.X + it.W/2) {
			it.X = num.Round(it.X+d, 2)
			o.boxes[it.ID] = it.box()
		}
	}
	for i := range o.r.Edges {
		e := &o.r.Edges[i]
		var labelDX float64
		if e.Label != nil {
			p, _, _ := anchor(e.Pts, *e.Label, e.LabelOff)
			if inR(p[0]) {
				labelDX = d
			}
		}
		for k := range e.Pts {
			if inR(e.Pts[k][0]) {
				e.Pts[k][0] = num.Round(e.Pts[k][0]+d, 2)
			}
		}
		if e.Label != nil {
			lb := *e.Label
			lb[0], lb[2] = num.Round(lb[0]+labelDX, 2), num.Round(lb[2]+labelDX, 2)
			e.Label = &lb
			o.setPath(i, e.Pts)
		}
	}
}
