// Package usecase lays out UML use case diagrams: the use cases in a column
// inside the system boundary, the actors in a column on the side of the
// boundary they were drawn on, each actor level with the use cases it takes
// part in, and the order chosen so the associations cross as little as
// possible. Includes and extends between use cases that are not neighbors run
// beside the column instead of through it.
package usecase

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
	caseGap     = 24 // between two use cases
	actorGap    = 30 // between two actors, labels included
	sideGap     = 90 // between an actor's column and the boundary
	padX        = 50 // inside the boundary, beside the use cases
	padTop      = 50 // inside the boundary, above the first use case, for its title
	padBottom   = 30
	bypass      = 20 // from the column to the first include or extend running beside it
	bypassGap   = 12 // between two of them
	partnerPull = 3  // weight of an include or extend when ordering use cases
	noteGap     = 30 // between the boundary and a note beside it
	sweeps      = 8
)

// Unsupported explains why a diagram cannot be laid out as a use case diagram.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

type actor struct {
	n     *tidy.Node
	right bool
	label float64 // height of the label below the figure
	width float64 // of the figure or its label, whichever is wider
	y     float64 // center of the figure
}

// Layout places the actors, use cases and boundary of a working copy.
func Layout(d *tidy.Diagram) error {
	var cases []*tidy.Node
	var actors []*actor
	var boundary *tidy.Node
	var others []*tidy.Node
	for _, n := range d.Nodes {
		switch {
		case n.Container:
			return &Unsupported{fmt.Sprintf("container %s in a use case diagram", n.V.ID)}
		case n.V.Shape == "umlActor":
			actors = append(actors, &actor{n: n})
		case detect.IsUseCase(n.V):
			cases = append(cases, n)
		case n.Overlay && boundary == nil:
			boundary = n
		case n.Overlay:
			return &Unsupported{"more than one system boundary"}
		case n.Text || n.V.Shape == "note":
			others = append(others, n)
		default:
			return &Unsupported{fmt.Sprintf("shape %s is neither an actor nor a use case", n.V.ID)}
		}
	}
	if len(cases) == 0 || len(actors) == 0 {
		return &Unsupported{"no actor or no use case"}
	}
	isCase := map[*tidy.Node]bool{}
	for _, c := range cases {
		isCase[c] = true
	}
	of := map[*tidy.Node]*actor{}
	for _, a := range actors {
		of[a.n] = a
	}
	// A line from a note to the wire it explains follows on its own.
	var wires []*tidy.Wire
	for _, w := range d.Wires {
		if onWire(w) {
			continue
		}
		wires = append(wires, w)
		if w.Src == nil || w.Dst == nil {
			return &Unsupported{fmt.Sprintf("wire %s has a free end", w.V.Cell.ID)}
		}
		for _, n := range []*tidy.Node{w.Src, w.Dst} {
			if !isCase[n] && of[n] == nil {
				return &Unsupported{fmt.Sprintf("wire %s ends on %s, which is neither an actor nor a use case", w.V.Cell.ID, n.V.ID)}
			}
		}
	}
	// Which side each actor stands on: the side of the boundary, or of the
	// use cases, it was drawn on.
	mid := 0.0
	if boundary != nil {
		mid = boundary.Box.Center().X
	} else {
		for _, c := range cases {
			mid += c.Box.Center().X
		}
		mid /= float64(len(cases))
	}
	for _, a := range actors {
		a.right = a.n.Box.Center().X > mid
		lw, lh := d.TextSize(a.n.V.Text, a.n.V.Style)
		a.label = lh
		a.width = math.Max(a.n.Box.W, lw)
	}
	links := map[*tidy.Node][]*tidy.Node{}
	for _, w := range wires {
		if w.Src != w.Dst {
			links[w.Src] = append(links[w.Src], w.Dst)
			links[w.Dst] = append(links[w.Dst], w.Src)
		}
	}
	// The order, starting from the drawing: use cases and actors by height.
	byY := func(ns []*tidy.Node) {
		sort.SliceStable(ns, func(i, j int) bool {
			a, b := ns[i].Box.Center(), ns[j].Box.Center()
			if math.Abs(a.Y-b.Y) > 1 {
				return a.Y < b.Y
			}
			return a.X < b.X
		})
	}
	byY(cases)
	sort.SliceStable(actors, func(i, j int) bool { return actors[i].n.Box.Center().Y < actors[j].n.Box.Center().Y })
	origin := geom.Point{X: math.Inf(1), Y: math.Inf(1)}
	for _, n := range d.Nodes {
		origin.X, origin.Y = math.Min(origin.X, n.Box.X), math.Min(origin.Y, n.Box.Y)
	}
	top := 0.0
	if boundary != nil {
		top = padTop
	}
	// Two use cases joined by a labeled include or extend need room between
	// them for the label.
	labelRoom := map[[2]*tidy.Node]float64{}
	for _, w := range wires {
		if isCase[w.Src] && isCase[w.Dst] {
			for _, l := range w.LabelSpans() {
				k := [2]*tidy.Node{w.Src, w.Dst}
				labelRoom[k] = math.Max(labelRoom[k], l.H+caseGap)
				labelRoom[[2]*tidy.Node{w.Dst, w.Src}] = labelRoom[k]
			}
		}
	}
	caseY := map[*tidy.Node]float64{}
	stack := func() {
		y := top
		for i, c := range cases {
			caseY[c] = y + c.Box.H/2
			y += c.Box.H + caseGap
			if i+1 < len(cases) {
				y += labelRoom[[2]*tidy.Node{c, cases[i+1]}]
			}
		}
	}
	stack()
	center := func(n *tidy.Node) float64 {
		if a := of[n]; a != nil {
			return a.y
		}
		return caseY[n]
	}
	placeActors := func() {
		for _, right := range []bool{false, true} {
			var side []*actor
			for _, a := range actors {
				if a.right != right {
					continue
				}
				sum, cnt := 0.0, 0
				for _, o := range links[a.n] {
					if isCase[o] {
						sum += caseY[o]
						cnt++
					}
				}
				if cnt > 0 {
					a.y = sum / float64(cnt)
				}
				side = append(side, a)
			}
			sort.SliceStable(side, func(i, j int) bool { return side[i].y < side[j].y })
			spread(side)
		}
	}
	for s := 0; s < sweeps; s++ {
		placeActors()
		key := map[*tidy.Node]float64{}
		for _, c := range cases {
			sum, cnt := 0.0, 0
			// An include or extend pulls harder than an association, so
			// the use cases it joins end up near each other.
			for _, o := range links[c] {
				k := 1
				if isCase[o] {
					k = partnerPull
				}
				sum += float64(k) * center(o)
				cnt += k
			}
			if cnt > 0 {
				key[c] = sum / float64(cnt)
			} else {
				key[c] = caseY[c]
			}
		}
		sort.SliceStable(cases, func(i, j int) bool { return key[cases[i]] < key[cases[j]] })
		stack()
	}
	placeActors()
	// Columns, left to right: actors on the left, the boundary and its use
	// cases, actors on the right.
	leftW, rightW, caseW := 0.0, 0.0, 0.0
	for _, a := range actors {
		if a.right {
			rightW = math.Max(rightW, a.width)
		} else {
			leftW = math.Max(leftW, a.width)
		}
	}
	for _, c := range cases {
		caseW = math.Max(caseW, c.Box.W)
	}
	// One width for the column, so every use case's leftmost and rightmost
	// points line up: an association reaching one of them from the side
	// stays clear of the others.
	for _, c := range cases {
		c.Box = geom.Rect{X: c.Box.Center().X - caseW/2, Y: c.Box.Y, W: caseW, H: c.Box.H}
	}
	x := 0.0
	if leftW > 0 {
		x = leftW + sideGap
	}
	inner := caseW
	if boundary != nil {
		inner += 2 * padX
	}
	colX := x + inner/2
	// Includes and extends running beside the column stay in the boundary.
	if long, half := detours(wires, cases, isCase); len(long) > 0 {
		inner += bypass + half + float64(len(long)-1)*bypassGap
	}
	// Notes stand in a column right of the boundary, before the actors on
	// that side.
	notesW := 0.0
	for _, n := range others {
		notesW = math.Max(notesW, n.Box.W+noteGap)
	}
	moved := map[*tidy.Node]geom.Point{}
	put := func(n *tidy.Node, c geom.Point) {
		moved[n] = geom.Point{X: c.X - n.Box.Center().X, Y: c.Y - n.Box.Center().Y}
		n.Box = geom.Rect{X: c.X - n.Box.W/2, Y: c.Y - n.Box.H/2, W: n.Box.W, H: n.Box.H}
	}
	bottom := 0.0
	for _, c := range cases {
		put(c, geom.Point{X: colX, Y: caseY[c]})
		bottom = math.Max(bottom, c.Box.Bottom())
	}
	for _, a := range actors {
		ax := leftW / 2
		if a.right {
			ax = x + inner + notesW + sideGap + rightW/2
		}
		put(a.n, geom.Point{X: ax, Y: a.y})
	}
	if boundary != nil {
		boundary.Box = geom.Rect{X: x, Y: 0, W: inner, H: bottom + padBottom}
	}
	// Each note level with the shape it stood nearest, below the note
	// above it.
	want := map[*tidy.Node]float64{}
	for _, n := range others {
		var best *tidy.Node
		bd := math.Inf(1)
		for m := range moved {
			if dd := gap(n.Orig, m.Orig); dd < bd {
				best, bd = m, dd
			}
		}
		want[n] = n.Box.Y
		if best != nil {
			want[n] = n.Box.Y + moved[best].Y
		}
	}
	sort.SliceStable(others, func(i, j int) bool { return want[others[i]] < want[others[j]] })
	below := math.Inf(-1)
	for _, n := range others {
		y := math.Max(want[n], below)
		n.Box = geom.Rect{X: x + inner + noteGap, Y: y, W: n.Box.W, H: n.Box.H}
		below = y + n.Box.H + caseGap
	}
	// Keep the diagram's top-left corner.
	now := geom.Point{X: math.Inf(1), Y: math.Inf(1)}
	for _, n := range d.Nodes {
		now.X, now.Y = math.Min(now.X, n.Box.X), math.Min(now.Y, n.Box.Y)
	}
	by := geom.Point{X: origin.X - now.X, Y: origin.Y - now.Y}
	for _, n := range d.Nodes {
		n.Box = n.Box.Move(by)
	}
	route(wires, cases, isCase, of)
	return nil
}

// spread places actors standing on one side in their order, each as near its
// wanted height as the ones above allow, then moves them all back up by the
// average overshoot.
func spread(side []*actor) {
	prev := math.Inf(-1)
	over := 0.0
	for _, a := range side {
		h := a.n.Box.H + a.label
		want := a.y
		a.y = math.Max(want, prev+a.n.Box.H/2)
		prev = a.y - a.n.Box.H/2 + h + actorGap
		over += a.y - want
	}
	if len(side) > 0 {
		shift := over / float64(len(side))
		lo := math.Inf(1)
		for _, a := range side {
			lo = math.Min(lo, a.y-shift-a.n.Box.H/2)
		}
		// Never above the top of the column.
		if lo < 0 {
			shift += lo
		}
		for _, a := range side {
			a.y -= shift
		}
	}
}

// route draws every wire straight. An association meets its use case at the
// middle of the side facing the actor. An include or extend between use cases
// that are not neighbors in the column runs beside the column, on the right,
// each on a track of its own.
func route(wires []*tidy.Wire, cases []*tidy.Node, isCase map[*tidy.Node]bool, of map[*tidy.Node]*actor) {
	right := math.Inf(-1)
	for _, c := range cases {
		right = math.Max(right, c.Box.Right())
	}
	for _, w := range wires {
		w.Points = nil
		for _, k := range []string{"exitX", "exitY", "exitDx", "exitDy", "exitPerimeter", "entryX", "entryY", "entryDx", "entryDy", "entryPerimeter"} {
			w.Style.Del(k)
		}
		w.Routed = true
		for _, e := range []struct {
			c, o   *tidy.Node
			prefix string
		}{{w.Src, w.Dst, "exit"}, {w.Dst, w.Src, "entry"}} {
			if a := of[e.o]; isCase[e.c] && a != nil {
				x := "0"
				if a.right {
					x = "1"
				}
				w.Style.Set(e.prefix+"X", x)
				w.Style.Set(e.prefix+"Y", "0.5")
				w.Style.Set(e.prefix+"Dx", "0")
				w.Style.Set(e.prefix+"Dy", "0")
			}
		}
	}
	long, half := detours(wires, cases, isCase)
	for i, w := range long {
		x := right + bypass + half + float64(i)*bypassGap
		w.Points = []geom.Point{{X: x, Y: w.Src.Box.Center().Y}, {X: x, Y: w.Dst.Box.Center().Y}}
	}
}

// detours lists the includes and extends between use cases that are not
// neighbors in the column, shorter ones first, so they run nearer the column
// and do not cross longer ones. half is half the widest label among them: a
// label sits centered on its track, so the tracks keep that clear of the
// column.
func detours(wires []*tidy.Wire, cases []*tidy.Node, isCase map[*tidy.Node]bool) (long []*tidy.Wire, half float64) {
	index := map[*tidy.Node]int{}
	for i, c := range cases {
		index[c] = i
	}
	span := func(w *tidy.Wire) int {
		k := index[w.Src] - index[w.Dst]
		if k < 0 {
			k = -k
		}
		return k
	}
	for _, w := range wires {
		if isCase[w.Src] && isCase[w.Dst] && span(w) > 1 {
			long = append(long, w)
			for _, l := range w.LabelSpans() {
				half = math.Max(half, l.W/2)
			}
		}
	}
	sort.SliceStable(long, func(i, j int) bool { return span(long[i]) < span(long[j]) })
	return long, half
}

// onWire reports a wire with an end on another wire, such as a note's line
// to the arrow it explains.
func onWire(w *tidy.Wire) bool {
	for _, ref := range []string{w.V.SrcRef, w.V.DstRef} {
		if c := w.V.Cell.Page.Cell(ref); ref != "" && c != nil && c.IsEdge() {
			return true
		}
	}
	return false
}

// gap returns the distance between two boxes, zero when they touch or overlap.
func gap(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}
