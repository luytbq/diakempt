package sequence

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/tidy"
)

// endTolerance is how far, in pixels, a message end may be from a hand-drawn
// lifeline and still be read as on it.
const endTolerance = 6

// participant is a hand-drawn lifeline: a head shape and the dashed line
// hanging from it.
type participant struct {
	head *tidy.Node
	line *tidy.Wire
	x    float64 // the line's x before layout
	// srcTop is set when the line's source end is its top.
	srcTop bool
}

// layoutHand lays out a sequence diagram drawn by hand: boxes for heads,
// dashed lines for lifelines, and arrows with free ends between the lines.
// Heads go in a row in their left-to-right order, each line straight below
// its head, and the messages in their time order, one per row.
func layoutHand(d *tidy.Diagram) error {
	parts, err := participants(d)
	if err != nil {
		return err
	}
	byLine := map[*tidy.Wire]bool{}
	for _, p := range parts {
		byLine[p.line] = true
	}
	of := map[*tidy.Node]*participant{}
	for _, p := range parts {
		of[p.head] = p
	}
	on := func(w *tidy.Wire, source bool) *participant {
		n, pt := w.Dst, w.DstPoint
		if source {
			n, pt = w.Src, w.SrcPoint
		}
		if n != nil {
			return of[n]
		}
		if pt == nil {
			return nil
		}
		var best *participant
		bd := float64(endTolerance)
		for _, p := range parts {
			if dd := math.Abs(pt.X - p.x); dd <= bd {
				best, bd = p, dd
			}
		}
		return best
	}
	var msgs []*message
	for i, w := range d.Wires {
		if byLine[w] {
			continue
		}
		if (w.Src != nil && of[w.Src] == nil) || (w.Dst != nil && of[w.Dst] == nil) {
			return &Unsupported{fmt.Sprintf("message %s is attached to a shape that is not a lifeline head", w.V.Cell.ID)}
		}
		if w.Src != nil || w.Dst != nil {
			return &Unsupported{fmt.Sprintf("message %s is attached to a lifeline head", w.V.Cell.ID)}
		}
		src, dst := on(w, true), on(w, false)
		if src == nil || dst == nil {
			return &Unsupported{fmt.Sprintf("message %s does not join two lifelines", w.V.Cell.ID)}
		}
		if src == dst && len(w.Points) == 0 {
			return &Unsupported{fmt.Sprintf("message %s starts and ends at the same point", w.V.Cell.ID)}
		}
		msgs = append(msgs, &message{w: w, src: src.head, dst: dst.head, self: src == dst, origY: rowOf(w), doc: i})
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		if math.Abs(msgs[i].origY-msgs[j].origY) > 1 {
			return msgs[i].origY < msgs[j].origY
		}
		return msgs[i].doc < msgs[j].doc
	})
	sort.SliceStable(parts, func(i, j int) bool { return parts[i].x < parts[j].x })
	heads := make([]*tidy.Node, len(parts))
	index := map[*tidy.Node]int{}
	origCenter := map[*tidy.Node]float64{}
	top, headH := math.Inf(1), 0.0
	for i, p := range parts {
		heads[i] = p.head
		index[p.head] = i
		origCenter[p.head] = p.x
		top = math.Min(top, p.head.Box.Y)
		headH = math.Max(headH, p.head.Box.H)
	}
	step := float64(minRow)
	for _, m := range msgs {
		_, h := d.TextSize(m.w.V.Text, m.w.Style)
		step = math.Max(step, h+18)
	}
	y := top + headH + firstRow
	for _, m := range msgs {
		m.y = y
		y += step
		if m.self {
			y += step
		}
	}
	bottom := y - step + tailPad
	// spacing anchors on the first head's center; a head drawn off its line
	// starts from the line.
	heads[0].Box.X = parts[0].x - heads[0].Box.W/2
	centers := spacing(d, heads, msgs, index)
	for i, p := range parts {
		h := p.head
		h.Box = geom.Rect{X: centers[i] - h.Box.W/2, Y: top, W: h.Box.W, H: h.Box.H}
		hang(p, bottom)
	}
	for _, m := range msgs {
		sx, dx := m.src.Box.Center().X, m.dst.Box.Center().X
		if m.self {
			out := sx + selfWidth
			m.w.SrcPoint = &geom.Point{X: sx, Y: m.y}
			m.w.DstPoint = &geom.Point{X: sx, Y: m.y + step}
			m.w.Points = []geom.Point{{X: out, Y: m.y}, {X: out, Y: m.y + step}}
		} else {
			m.w.SrcPoint = &geom.Point{X: sx, Y: m.y}
			m.w.DstPoint = &geom.Point{X: dx, Y: m.y}
			m.w.Points = nil
		}
		m.w.Routed = true
	}
	remap := rowMap(msgs, top)
	nearest := func(x float64) *tidy.Node {
		var best *tidy.Node
		bd := math.Inf(1)
		for _, h := range heads {
			if dd := math.Abs(origCenter[h] - x); dd < bd {
				best, bd = h, dd
			}
		}
		return best
	}
	for _, n := range d.Nodes {
		if of[n] != nil {
			continue
		}
		h := nearest(n.Box.Center().X)
		n.Box = geom.Rect{X: n.Box.X + h.Box.Center().X - origCenter[h], Y: remap(n.Box.Y), W: n.Box.W, H: n.Box.H}
	}
	keepCornerFree(d)
	return nil
}

// participants pairs each hand-drawn lifeline with its head: the shape its top
// end is attached to, or the shape just above a free top end.
func participants(d *tidy.Diagram) ([]*participant, error) {
	var parts []*participant
	taken := map[*tidy.Node]*tidy.Wire{}
	for _, w := range d.Wires {
		var src, dst *geom.Rect
		if w.Src != nil {
			src = &w.Src.Box
		}
		if w.Dst != nil {
			dst = &w.Dst.Box
		}
		x, top, _, ok := segment.HandLifeline(w.Style, w.Path(), len(w.Points), src, dst)
		if !ok {
			continue
		}
		p := &participant{line: w, x: x}
		switch {
		case w.Src != nil:
			p.head, p.srcTop = w.Src, true
		case w.Dst != nil:
			p.head = w.Dst
		default:
			p.head = headAbove(d, x, top)
			p.srcTop = w.SrcPoint.Y <= w.DstPoint.Y
		}
		if p.head == nil {
			return nil, &Unsupported{fmt.Sprintf("lifeline %s hangs from no shape", w.V.Cell.ID)}
		}
		if other := taken[p.head]; other != nil {
			return nil, &Unsupported{fmt.Sprintf("lifelines %s and %s hang from one shape", other.V.Cell.ID, w.V.Cell.ID)}
		}
		taken[p.head] = w
		parts = append(parts, p)
	}
	if len(parts) < 2 {
		return nil, &Unsupported{"fewer than two lifelines"}
	}
	for _, n := range d.Nodes {
		if n.Container {
			return nil, &Unsupported{fmt.Sprintf("container %s in a hand-drawn sequence", n.V.ID)}
		}
	}
	return parts, nil
}

// headAbove is segment's rule for the shape a free lifeline top hangs from,
// on the working copy.
func headAbove(d *tidy.Diagram, x, top float64) *tidy.Node {
	var best *tidy.Node
	bd := math.Inf(1)
	for _, n := range d.Nodes {
		if n.Container || n.Text || x < n.Box.X-4 || x > n.Box.Right()+4 {
			continue
		}
		dd := top - n.Box.Bottom()
		if dd < -4 || dd > segment.AttachDistance || n.Box.Y > top {
			continue
		}
		if dd < bd {
			best, bd = n, dd
		}
	}
	return best
}

// hang draws a participant's line straight down from the bottom center of its
// head to bottom, moving only its free ends.
func hang(p *participant, bottom float64) {
	h := p.head.Box
	top := geom.Point{X: h.Center().X, Y: h.Bottom()}
	end := geom.Point{X: h.Center().X, Y: bottom}
	w := p.line
	if p.srcTop {
		if w.Src == nil {
			w.SrcPoint = &top
		}
		w.DstPoint = &end
	} else {
		if w.Dst == nil {
			w.DstPoint = &top
		}
		w.SrcPoint = &end
	}
	w.Points = nil
}

// keepCornerFree moves everything, free wire ends included, so the diagram's
// top-left corner is where it was.
func keepCornerFree(d *tidy.Diagram) {
	var was, now geom.Rect
	for _, n := range d.Nodes {
		was = was.Union(n.Orig)
		now = now.Union(n.Box)
	}
	by := geom.Point{X: was.X - now.X, Y: was.Y - now.Y}
	if by == (geom.Point{}) {
		return
	}
	for _, n := range d.Nodes {
		n.Box = n.Box.Move(by)
	}
	for _, w := range d.Wires {
		for i := range w.Points {
			w.Points[i] = w.Points[i].Add(by)
		}
		if w.SrcPoint != nil {
			p := w.SrcPoint.Add(by)
			w.SrcPoint = &p
		}
		if w.DstPoint != nil {
			p := w.DstPoint.Add(by)
			w.DstPoint = &p
		}
	}
}
