package tidy

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/report"
)

// Log collects what operations did, for the report.
type Log struct {
	Counts  map[string]int
	Details []report.Detail
}

func (l *Log) add(op string, cells []string, format string, args ...any) {
	if l.Counts == nil {
		l.Counts = map[string]int{}
	}
	l.Counts[op]++
	l.Details = append(l.Details, report.Detail{Op: op, Cells: cells, Msg: fmt.Sprintf(format, args...)})
}

// Ops returns the counts in operation order.
func (l *Log) Ops(order []string) []report.OpCount {
	var out []report.OpCount
	for _, op := range order {
		if n := l.Counts[op]; n > 0 {
			out = append(out, report.OpCount{Op: op, Count: n})
		}
	}
	return out
}

// axis selects x (0) or y (1).
type axis int

const (
	axisX axis = iota
	axisY
)

func center(r geom.Rect, a axis) float64 {
	if a == axisX {
		return r.X + r.W/2
	}
	return r.Y + r.H/2
}

func vec(a axis, v float64) geom.Point {
	if a == axisX {
		return geom.Point{X: v}
	}
	return geom.Point{Y: v}
}

// sameLine is how close two centers must be to count as level with each other;
// relative order is only defined beyond it.
const sameLine = 0.5

// Shift opens space along one axis: every node whose center lies at or past
// from moves by delta, together with its contents, and wire points past from
// move with them. Nodes before from stay. Because everything past the line
// moves by the same amount, no two nodes swap order.
//
// A container straddling the line stays where it is and stretches (or
// shrinks) by delta, so its far edge follows its contents.
func (d *Diagram) Shift(a axis, from, delta float64) {
	moved := map[*Node]bool{}
	var units []*Node
	for _, n := range d.Nodes {
		if n.Parent != nil && moved[n.Parent] {
			moved[n] = true
			continue
		}
		start := n.Box.X
		if a == axisY {
			start = n.Box.Y
		}
		past := center(n.Box, a) >= from-sameLine
		if n.Container {
			past = start >= from-sameLine
			if !past && end(n.Box, a) >= from-sameLine {
				if a == axisX {
					n.Box.W = math.Max(1, n.Box.W+delta)
				} else {
					n.Box.H = math.Max(1, n.Box.H+delta)
				}
			}
		}
		if past {
			moved[n] = true
			units = append(units, n)
		}
	}
	for _, n := range units {
		var shift func(m *Node)
		shift = func(m *Node) {
			m.Box = m.Box.Move(vec(a, delta))
			for _, c := range m.Children {
				shift(c)
			}
		}
		shift(n)
	}
	movePt := func(p geom.Point) geom.Point {
		if a == axisX && p.X >= from-sameLine {
			p.X += delta
		}
		if a == axisY && p.Y >= from-sameLine {
			p.Y += delta
		}
		return p
	}
	for _, w := range d.Wires {
		for i := range w.Points {
			w.Points[i] = movePt(w.Points[i])
		}
		if w.SrcPoint != nil {
			p := movePt(*w.SrcPoint)
			w.SrcPoint = &p
		}
		if w.DstPoint != nil {
			p := movePt(*w.DstPoint)
			w.DstPoint = &p
		}
	}
}

// Separate pushes overlapping siblings apart until each pair is at least gap
// apart on one axis. For each overlap it picks the axis needing the smaller
// push, and opens space with Shift so relative order holds. A shape drawn
// entirely inside another, like a badge on a box, is left where it is.
func (d *Diagram) Separate(gap float64, log *Log) {
	limit := 20 * (len(d.Nodes) + 1)
	for iter := 0; iter < limit; iter++ {
		a, b, ax, from, delta, ok := d.worstOverlap(gap)
		if !ok {
			return
		}
		d.Shift(ax, from, delta)
		log.add("separate", []string{a.V.ID, b.V.ID}, "moved %s and what follows it %s px to clear %s",
			b.V.ID, geom.Format(delta), a.V.ID)
	}
}

// worstOverlap finds the first overlapping sibling pair in document order and
// the shift that clears it.
func (d *Diagram) worstOverlap(gap float64) (a, b *Node, ax axis, from, delta float64, ok bool) {
	groups := map[*Node][]*Node{}
	var parents []*Node
	for _, n := range d.Nodes {
		if n.Text || n.Overlay {
			continue
		}
		if _, seen := groups[n.Parent]; !seen {
			parents = append(parents, n.Parent)
		}
		groups[n.Parent] = append(groups[n.Parent], n)
	}
	for _, p := range parents {
		sib := groups[p]
		for i := 0; i < len(sib); i++ {
			for j := i + 1; j < len(sib); j++ {
				x, y := sib[i], sib[j]
				if !x.Box.Inset(1).Overlaps(y.Box.Inset(1)) {
					continue
				}
				if x.Box.ContainsRect(y.Box) || y.Box.ContainsRect(x.Box) {
					continue
				}
				best := math.Inf(1)
				for _, cand := range []axis{axisX, axisY} {
					cx, cy := center(x.Box, cand), center(y.Box, cand)
					if math.Abs(cx-cy) <= sameLine {
						continue
					}
					first, later := x, y
					if cx > cy {
						first, later = y, x
					}
					end, start := first.Box.Right(), later.Box.X
					if cand == axisY {
						end, start = first.Box.Bottom(), later.Box.Y
					}
					need := end + gap - start
					if need > 0 && need < best {
						best = need
						a, b, ax, from, delta, ok = first, later, cand, center(later.Box, cand), need, true
					}
				}
				if ok {
					return
				}
			}
		}
	}
	return
}

// header returns the space a swimlane's title takes on its left or top side.
func header(n *Node) (left, top float64) {
	st := n.V.Style
	if n.V.Shape != "swimlane" {
		return 0, 0
	}
	size := geom.ParseNumber(st.Value("startSize", "23"))
	if st.Value("horizontal", "1") == "0" {
		return size, 0
	}
	return 0, size
}

// containerPad is the space kept between a container's edge and its contents.
const containerPad = 10

// FitContainers grows every container so its children fit inside, innermost
// first. Containers only grow; one that has room to spare keeps its size.
func (d *Diagram) FitContainers(log *Log) bool {
	changed := false
	order := append([]*Node(nil), d.Nodes...)
	sort.SliceStable(order, func(i, j int) bool { return depth(order[i]) > depth(order[j]) })
	for _, n := range order {
		if !n.Container || len(n.Children) == 0 {
			continue
		}
		// Only a container something sticks out of grows; one whose children
		// fit, however tightly, is left as drawn.
		fits := true
		for _, c := range n.Children {
			if !n.Box.Inset(-1).ContainsRect(c.Box) {
				fits = false
			}
		}
		if fits {
			continue
		}
		// Shapes keep some padding from the edge; lanes sit flush against it.
		var content geom.Rect
		for _, c := range n.Children {
			pad := float64(containerPad)
			if c.Container {
				pad = 0
			}
			content = content.Union(c.Box.Inset(-pad))
		}
		left, top := header(n)
		want := geom.Rect{X: content.X - left, Y: content.Y - top}
		want.W = content.Right() - want.X
		want.H = content.Bottom() - want.Y
		nb := n.Box.Union(want)
		if !near(nb, n.Box) {
			log.add("containers", []string{n.V.ID}, "grew %s from %sx%s to %sx%s", n.V.ID,
				geom.Format(n.Box.W), geom.Format(n.Box.H), geom.Format(nb.W), geom.Format(nb.H))
			n.Box = nb
			changed = true
		}
	}
	if d.restack() {
		changed = true
	}
	return changed
}

func depth(n *Node) int {
	k := 0
	for p := n.Parent; p != nil; p = p.Parent {
		k++
	}
	return k
}

// stacked reports whether n is a pool whose lanes draw.io keeps side by side.
func (d *Diagram) stacked(n *Node) bool {
	return n.V.Style.Value("childLayout", "") == "stackLayout"
}

// restack lays the lanes of each stacked pool edge to edge again after some of
// them grew, and fits the pool around them. Along the stack, a lane moves with
// its contents to close or open the gap to its neighbor; across it, lanes only
// stretch to a common extent, so no shape moves on that axis.
func (d *Diagram) restack() bool {
	changed := false
	for _, pool := range d.Nodes {
		if !d.stacked(pool) || len(pool.Children) == 0 {
			continue
		}
		horizontal := pool.V.Style.Value("horizontalStack", "1") != "0"
		lanes := append([]*Node(nil), pool.Children...)
		ax := axisY
		if horizontal {
			ax = axisX
		}
		sort.SliceStable(lanes, func(i, j int) bool { return center(lanes[i].Box, ax) < center(lanes[j].Box, ax) })
		left, top := header(pool)
		// Common extent across the stack.
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, l := range lanes {
			if horizontal {
				lo, hi = math.Min(lo, l.Box.Y), math.Max(hi, l.Box.Bottom())
			} else {
				lo, hi = math.Min(lo, l.Box.X), math.Max(hi, l.Box.Right())
			}
		}
		if horizontal {
			lo = math.Min(lo, pool.Box.Y+top)
		} else {
			lo = math.Min(lo, pool.Box.X+left)
		}
		next := pool.Box.X + left
		if !horizontal {
			next = pool.Box.Y + top
		}
		for _, l := range lanes {
			if horizontal {
				if dx := next - l.Box.X; math.Abs(dx) > 0.005 {
					d.Move(l, geom.Point{X: dx})
					changed = true
				}
				if nb := (geom.Rect{X: l.Box.X, Y: lo, W: l.Box.W, H: hi - lo}); !near(nb, l.Box) {
					l.Box = nb
					changed = true
				}
				next = l.Box.Right()
			} else {
				if dy := next - l.Box.Y; math.Abs(dy) > 0.005 {
					d.Move(l, geom.Point{Y: dy})
					changed = true
				}
				if nb := (geom.Rect{X: lo, Y: l.Box.Y, W: hi - lo, H: l.Box.H}); !near(nb, l.Box) {
					l.Box = nb
					changed = true
				}
				next = l.Box.Bottom()
			}
		}
		var nb geom.Rect
		if horizontal {
			nb = geom.Rect{X: pool.Box.X, Y: lo - top, W: next - pool.Box.X, H: hi - lo + top}
		} else {
			nb = geom.Rect{X: lo - left, Y: pool.Box.Y, W: hi - lo + left, H: next - pool.Box.Y}
		}
		if !near(nb, pool.Box) {
			pool.Box = nb
			changed = true
		}
	}
	return changed
}

// Arrange separates siblings and fits containers until both are settled.
func (d *Diagram) Arrange(gap float64, separate, containers bool, log *Log) {
	for round := 0; round < 20; round++ {
		before := d.Save()
		if separate {
			d.Separate(gap, log)
		}
		if containers {
			d.FitContainers(log)
		}
		if d.sameAs(before) {
			return
		}
	}
}

// Unchanged reports whether no node moved or resized since the snapshot.
func (d *Diagram) Unchanged(s Snapshot) bool { return d.sameAs(s) }

func (d *Diagram) sameAs(s Snapshot) bool {
	for i, n := range d.Nodes {
		if n.Box != s.boxes[i] {
			return false
		}
	}
	return true
}
