// Package sequence lays out sequence diagrams. Those drawn with the editor's
// UML lifeline shape get lifelines in a row in the author's order, spaced so
// message labels fit; messages in their time order, one per row, all
// horizontal; activation bars spanning exactly the messages they send and
// receive; frames and notes following the messages they belong with.
// Hand-drawn ones (boxes, dashed lines and free arrows) get their heads in a
// row, each line straight below its head, and their messages one per row.
package sequence

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
)

// Spacing, in pixels.
const (
	minGap      = 40 // between two lifeline heads
	labelPad    = 30 // around a message label between two lifelines
	firstRow    = 30 // from the bottom of the heads to the first message
	minRow      = 36 // between two messages
	selfWidth   = 35 // how far a self call reaches out
	labelRoom   = 5  // beside a label where no other measure applies
	barPad      = 10 // an activation bar beyond its first and last message
	tailPad     = 40 // lifeline below the last message
	frameHead   = 40 // above a frame's first message, for its title
	frameFoot   = 20 // below its last message
	titleRoom   = 20 // more room above a message where a frame opens
	footRoom    = 10 // more room below a message where a frame closes
	frameMargin = 60 // beside the outermost lifeline in a frame
)

// Unsupported explains why a diagram cannot be laid out as a sequence.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

type message struct {
	w        *tidy.Wire
	src, dst *tidy.Node // lifelines, or the shapes on them; nil at a gate
	// srcGate and dstGate are set for an end that comes from outside the
	// lifelines: a frame's edge or a found message's dot.
	srcGate, dstGate *gate
	self             bool
	origY            float64
	y                float64 // new row
	doc              int
}

// gate is a message end that touches no lifeline.
type gate struct {
	at    geom.Point // before layout
	frame *tidy.Node // the frame whose edge it lies on, or nil
	left  bool       // on the frame's left edge
	dot   *tidy.Node // the shape drawn at the end, such as a found message's dot
	place geom.Point // after layout
}

// item is one entry of the time line: a message, an operand separator or a
// frame holding no message (an interaction use such as "ref Login").
type item struct {
	msg   *message
	sep   *tidy.Wire
	frame *tidy.Node
	origY float64
	doc   int
	y     float64
	h     float64 // rows it takes, in pixels
}

// frame is a frame over the messages, with what it encloses.
type frame struct {
	n      *tidy.Node
	items  []*item
	inner  []*frame
	heads  []*tidy.Node // lifelines whose head it encloses, for an sd frame
	covers []*tidy.Node // lifelines whose center line it crosses
	use    *item        // the frame's own row when it holds no message
}

// Layout places the lifelines, messages, bars, frames and notes of a working
// copy.
func Layout(d *tidy.Diagram) error {
	var lifelines []*tidy.Node
	for _, n := range d.Nodes {
		if n.V.Shape == "umlLifeline" {
			lifelines = append(lifelines, n)
		}
	}
	if len(lifelines) == 0 {
		return layoutHand(d)
	}
	if len(lifelines) < 2 {
		return &Unsupported{"fewer than two UML lifelines"}
	}
	for _, n := range d.Nodes {
		if n.Container && n.V.Shape != "umlLifeline" {
			return &Unsupported{fmt.Sprintf("container %s is not a lifeline", n.V.ID)}
		}
	}
	var frames []*tidy.Node
	for _, n := range d.Nodes {
		if n.Overlay {
			frames = append(frames, n)
		}
	}
	end := func(w *tidy.Wire, source bool) (*tidy.Node, *gate) {
		n, pt := w.Dst, w.DstPoint
		if source {
			n, pt = w.Src, w.SrcPoint
		}
		if n != nil {
			if lifeline(n) != nil {
				return n, nil
			}
			c := n.Box.Center()
			return nil, &gate{at: c, dot: n}
		}
		if pt == nil {
			return nil, nil
		}
		for _, l := range lifelines {
			if math.Abs(pt.X-l.Box.Center().X) <= endTolerance && pt.Y >= l.Box.Y && pt.Y <= l.Box.Bottom() {
				return l, nil
			}
		}
		return nil, &gate{at: *pt}
	}
	var items []*item
	dots := map[*tidy.Node]bool{}
	for i, w := range d.Wires {
		src, sg := end(w, true)
		dst, dg := end(w, false)
		if src == nil && dst == nil {
			if f := separatorOf(w, frames); f != nil {
				items = append(items, &item{sep: w, frame: f, origY: rowOf(w), doc: i})
				continue
			}
			return &Unsupported{fmt.Sprintf("message %s does not touch a lifeline", w.V.Cell.ID)}
		}
		if (src == nil && sg == nil) || (dst == nil && dg == nil) {
			return &Unsupported{fmt.Sprintf("message %s has an end without a position", w.V.Cell.ID)}
		}
		for _, g := range []*gate{sg, dg} {
			if g == nil {
				continue
			}
			if g.dot != nil {
				if g.dot.Container || dots[g.dot] {
					return &Unsupported{fmt.Sprintf("message %s is attached to %s, which is not on a lifeline", w.V.Cell.ID, g.dot.V.ID)}
				}
				dots[g.dot] = true
			}
			g.frame, g.left = edgeOf(g.at, frames)
		}
		self := src != nil && dst != nil && lifeline(src) == lifeline(dst)
		if self && len(w.Points) == 0 && w.Src == nil && w.Dst == nil {
			return &Unsupported{fmt.Sprintf("message %s starts and ends at the same point", w.V.Cell.ID)}
		}
		m := &message{w: w, src: lifeline(src), dst: lifeline(dst), srcGate: sg, dstGate: dg, self: self, origY: rowOf(w), doc: i}
		items = append(items, &item{msg: m, origY: m.origY, doc: i})
	}
	sort.SliceStable(lifelines, func(i, j int) bool { return lifelines[i].Box.Center().X < lifelines[j].Box.Center().X })
	index := map[*tidy.Node]int{}
	origCenter := map[*tidy.Node]float64{}
	for i, l := range lifelines {
		index[l] = i
		origCenter[l] = l.Box.Center().X
	}
	fs := readFrames(frames, items, lifelines, origCenter)
	for _, f := range fs {
		if len(f.items) == 0 && len(f.heads) == 0 {
			f.use = &item{frame: f.n, origY: f.n.Box.Y, doc: -1, h: f.n.Box.H}
			items = append(items, f.use)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if math.Abs(items[i].origY-items[j].origY) > 1 {
			return items[i].origY < items[j].origY
		}
		return items[i].doc < items[j].doc
	})
	var msgs []*message
	for _, it := range items {
		if it.msg != nil {
			msgs = append(msgs, it.msg)
		}
	}
	top, head := math.Inf(1), 0.0
	for _, l := range lifelines {
		top = math.Min(top, l.Box.Y)
		head = math.Max(head, geom.ParseNumber(l.V.Style.Value("size", "40")))
	}
	step := float64(minRow)
	for _, it := range items {
		var w *tidy.Wire
		switch {
		case it.msg != nil:
			w = it.msg.w
		case it.sep != nil:
			w = it.sep
		default:
			continue
		}
		_, h := d.TextSize(w.V.Text, w.Style)
		step = math.Max(step, h+18)
	}
	// Rows. Each frame opening before an item, or closing after one, adds
	// room for its title or its bottom edge.
	opens, closes := map[*item]int{}, map[*item]int{}
	for _, f := range fs {
		if len(f.items) > 0 {
			opens[f.items[0]]++
			closes[f.items[len(f.items)-1]]++
		}
	}
	y := top + head + firstRow
	for _, it := range items {
		y += float64(opens[it]) * titleRoom
		switch {
		case it.msg != nil:
			it.y = y
			it.msg.y = y
			it.h = step
			if it.msg.self {
				it.h += step
			}
		case it.sep != nil:
			it.y = y
			it.h = step
		default:
			// An interaction use takes its own height, from half a row below
			// the previous message to half a row above the next.
			it.y = y - step/2
			it.h += step
		}
		y += it.h + float64(closes[it])*footRoom
	}
	bottom := y - step + tailPad
	centers := spacing(d, lifelines, msgs, index)
	for i, l := range lifelines {
		l.Box = geom.Rect{X: centers[i] - l.Box.W/2, Y: top, W: l.Box.W, H: math.Max(bottom-top, head+tailPad)}
	}
	var pts []rowPt
	for _, it := range items {
		if it.msg != nil || it.sep != nil {
			pts = append(pts, rowPt{it.origY, it.y})
		}
	}
	remap := rowMapPts(pts, top)
	placeBars(d, lifelines, msgs, origCenter, remap, step)
	placeFrames(fs, lifelines, origCenter, remap, step)
	placeOthers(d, lifelines, msgs, origCenter, remap, dots)
	placeGates(msgs, lifelines, origCenter)
	for _, it := range items {
		if it.sep != nil {
			b := it.frame.Box
			it.sep.SrcPoint = &geom.Point{X: b.X, Y: it.y}
			it.sep.DstPoint = &geom.Point{X: b.Right(), Y: it.y}
			if it.sep.V.SrcPoint != nil && it.sep.V.DstPoint != nil && it.sep.V.SrcPoint.X > it.sep.V.DstPoint.X {
				it.sep.SrcPoint, it.sep.DstPoint = it.sep.DstPoint, it.sep.SrcPoint
			}
			it.sep.Points = nil
			it.sep.Routed = true
		}
	}
	routeMessages(msgs, step)
	placeMessageLabels(d, msgs, lifelines, index)
	keepCornerFree(d)
	return nil
}

// nestPad is the room, in pixels, between a frame and a frame nested in it.
const nestPad = 10

// separatorOf returns the frame a wire divides into operands: a line without
// arrowheads, with both ends free, running across the frame.
func separatorOf(w *tidy.Wire, frames []*tidy.Node) *tidy.Node {
	st := w.Style
	if st.Value("endArrow", "classic") != "none" || st.Value("startArrow", "none") != "none" || w.SrcPoint == nil || w.DstPoint == nil || len(w.Points) > 0 {
		return nil
	}
	a, b := *w.SrcPoint, *w.DstPoint
	if math.Abs(a.Y-b.Y) > 2 {
		return nil
	}
	var best *tidy.Node
	for _, f := range frames {
		if f.Box.Inset(-endTolerance).Contains(a) && f.Box.Inset(-endTolerance).Contains(b) &&
			math.Abs(b.X-a.X) >= 0.8*f.Box.W && (best == nil || f.Box.W*f.Box.H < best.Box.W*best.Box.H) {
			best = f
		}
	}
	return best
}

// edgeOf returns the smallest frame whose left or right edge a point lies on.
func edgeOf(p geom.Point, frames []*tidy.Node) (*tidy.Node, bool) {
	var best *tidy.Node
	left := false
	for _, f := range frames {
		if p.Y < f.Box.Y || p.Y > f.Box.Bottom() {
			continue
		}
		l := math.Abs(p.X-f.Box.X) <= endTolerance
		r := math.Abs(p.X-f.Box.Right()) <= endTolerance
		if (l || r) && (best == nil || f.Box.W*f.Box.H < best.Box.W*best.Box.H) {
			best, left = f, l
		}
	}
	return best, left
}

// readFrames works out what each frame encloses before anything moves.
func readFrames(frames []*tidy.Node, items []*item, lifelines []*tidy.Node, origCenter map[*tidy.Node]float64) []*frame {
	var fs []*frame
	byNode := map[*tidy.Node]*frame{}
	for _, n := range frames {
		f := &frame{n: n}
		b := n.Box
		for _, l := range lifelines {
			c := origCenter[l]
			if c >= b.X && c <= b.Right() {
				f.covers = append(f.covers, l)
				if b.Y <= l.Box.Y+1 && b.Bottom() >= l.Box.Y {
					f.heads = append(f.heads, l)
				}
			}
		}
		fs = append(fs, f)
		byNode[n] = f
	}
	for _, it := range items {
		if it.sep != nil {
			f := byNode[it.frame]
			f.items = append(f.items, it)
			continue
		}
		m := it.msg
		x0, x1 := math.Inf(1), math.Inf(-1)
		for _, e := range []struct {
			l *tidy.Node
			g *gate
		}{{m.src, m.srcGate}, {m.dst, m.dstGate}} {
			x := 0.0
			if e.l != nil {
				x = origCenter[e.l]
			} else {
				x = e.g.at.X
			}
			x0, x1 = math.Min(x0, x), math.Max(x1, x)
		}
		for _, f := range fs {
			b := f.n.Box
			if m.origY >= b.Y && m.origY <= b.Bottom() && x1 >= b.X-endTolerance && x0 <= b.Right()+endTolerance {
				f.items = append(f.items, it)
			}
		}
	}
	// Nesting: a frame inside another. Smaller frames come first, so inner
	// frames are placed before the frames around them.
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].n.Box.W*fs[i].n.Box.H < fs[j].n.Box.W*fs[j].n.Box.H })
	for i, f := range fs {
		for _, g := range fs[:i] {
			if f.n.Box.Inset(-1).ContainsRect(g.n.Box) {
				f.inner = append(f.inner, g)
			}
		}
	}
	for _, f := range fs {
		sort.SliceStable(f.items, func(i, j int) bool {
			if math.Abs(f.items[i].origY-f.items[j].origY) > 1 {
				return f.items[i].origY < f.items[j].origY
			}
			return f.items[i].doc < f.items[j].doc
		})
	}
	return fs
}

// placeFrames redraws each frame: an sd frame around the lifelines it
// enclosed, keeping its margins; another frame around the messages it
// enclosed, past the outermost lifeline involved and any frame nested in it;
// an interaction use across the lifelines it crossed, at its row.
func placeFrames(fs []*frame, lifelines []*tidy.Node, origCenter map[*tidy.Node]float64, remap func(float64) float64, step float64) {
	for _, f := range fs {
		n := f.n
		o := n.Orig
		switch {
		case len(f.heads) > 0:
			var was, now geom.Rect
			for _, l := range f.covers {
				was = was.Union(geom.Rect{X: origCenter[l] - l.Orig.W/2, Y: l.Orig.Y, W: l.Orig.W, H: l.Orig.H})
				now = now.Union(l.Box)
			}
			n.Box = geom.Rect{X: now.X - (was.X - o.X), Y: now.Y - (was.Y - o.Y), W: 0, H: 0}
			n.Box.W = now.Right() + (o.Right() - was.Right()) - n.Box.X
			n.Box.H = now.Bottom() + (o.Bottom() - was.Bottom()) - n.Box.Y
		case len(f.items) > 0:
			lo, hi := math.Inf(1), math.Inf(-1)
			left, right := math.Inf(1), math.Inf(-1)
			for _, it := range f.items {
				lo, hi = math.Min(lo, it.y), math.Max(hi, it.y+it.h-step)
				if it.msg == nil {
					continue
				}
				for _, l := range []*tidy.Node{it.msg.src, it.msg.dst} {
					if l == nil {
						continue
					}
					c := lifeline(l).Box.Center().X
					left, right = math.Min(left, c), math.Max(right, c)
				}
			}
			if math.IsInf(left, 1) {
				for _, l := range f.covers {
					c := l.Box.Center().X
					left, right = math.Min(left, c), math.Max(right, c)
				}
			}
			// Frames opening or closing at the same message stack their
			// edges, the outer ones further out.
			above, below := 0, 0
			for _, g := range f.inner {
				if len(g.items) > 0 && g.items[0] == f.items[0] {
					above++
				}
				if len(g.items) > 0 && g.items[len(g.items)-1] == f.items[len(f.items)-1] {
					below++
				}
			}
			b := geom.Rect{X: left - frameMargin, Y: lo - frameHead - float64(above)*titleRoom}
			b.W = right - left + 2*frameMargin
			b.H = hi + frameFoot + float64(below)*footRoom - b.Y
			for _, g := range f.inner {
				b = b.Union(g.n.Box.Inset(-nestPad))
			}
			n.Box = b
		case len(f.covers) > 0:
			first, last := f.covers[0], f.covers[len(f.covers)-1]
			x0 := first.Box.Center().X - (origCenter[first] - o.X)
			x1 := last.Box.Center().X + (o.Right() - origCenter[last])
			n.Box = geom.Rect{X: x0, Y: f.use.y, W: x1 - x0, H: o.H}
		default:
			n.Box = geom.Rect{X: o.X, Y: f.use.y, W: o.W, H: o.H}
		}
	}
}

// placeGates puts each gate on its frame's new edge, or keeps its offset from
// the nearest lifeline, at its message's row, with its dot centered on it.
func placeGates(msgs []*message, lifelines []*tidy.Node, origCenter map[*tidy.Node]float64) {
	for _, m := range msgs {
		for _, g := range []*gate{m.srcGate, m.dstGate} {
			if g == nil {
				continue
			}
			x := g.at.X
			switch {
			case g.frame != nil && g.left:
				x = g.frame.Box.X
			case g.frame != nil:
				x = g.frame.Box.Right()
			default:
				var best *tidy.Node
				bd := math.Inf(1)
				for _, l := range lifelines {
					if dd := math.Abs(origCenter[l] - g.at.X); dd < bd {
						best, bd = l, dd
					}
				}
				x += best.Box.Center().X - origCenter[best]
			}
			g.place = geom.Point{X: x, Y: m.y}
			if g.dot != nil {
				g.dot.Box = geom.Rect{X: x - g.dot.Box.W/2, Y: m.y - g.dot.Box.H/2, W: g.dot.Box.W, H: g.dot.Box.H}
			}
		}
	}
}

func lifeline(n *tidy.Node) *tidy.Node {
	for k := 0; n != nil && k < 64; k++ {
		if n.V.Shape == "umlLifeline" {
			return n
		}
		n = n.Parent
	}
	return nil
}

// rowOf reads when a message happens: the height of its first waypoint, or of
// its middle when it has none.
func rowOf(w *tidy.Wire) float64 {
	if len(w.Points) > 0 {
		return w.Points[0].Y
	}
	pl := w.Path()
	if len(pl) == 0 {
		return 0
	}
	return (pl[0].Y + pl[len(pl)-1].Y) / 2
}

// spacing returns the new lifeline centers: heads at least minGap apart, and
// every message label in a gap wide enough for it. The label of a message
// spanning several gaps goes in the gap nearest the middle of the span, and a
// self call's label in the gap to its right. The first lifeline keeps its
// center.
func spacing(d *tidy.Diagram, lifelines []*tidy.Node, msgs []*message, index map[*tidy.Node]int) []float64 {
	n := len(lifelines)
	gaps := make([]float64, n-1)
	for i := range gaps {
		gaps[i] = (lifelines[i].Box.W+lifelines[i+1].Box.W)/2 + minGap
	}
	grow := func(i int, need float64) {
		if gaps[i] < need {
			gaps[i] = need
		}
	}
	for _, m := range msgs {
		if m.src == nil || m.dst == nil {
			continue
		}
		w := 0.0
		for _, s := range m.w.LabelSpans() {
			w = math.Max(w, s.W)
		}
		if w == 0 {
			continue
		}
		a, b := index[m.src], index[m.dst]
		if a > b {
			a, b = b, a
		}
		if m.self {
			if a+1 >= n {
				continue
			}
			out := float64(selfWidth)
			if m.w.Src != nil && m.w.Src != m.src {
				out += barInset(m.w.Src, m.src, true)
			}
			grow(a, out+w+2*labelRoom)
			continue
		}
		if b-a < 1 || b >= n {
			continue
		}
		g, left, right, ok := labelGap(m, index, lifelines)
		if !ok {
			continue
		}
		// Bars on the lifelines at the gap's edges reach into the gap too;
		// this over-reserves when a bar sits on another row, which is safer
		// than reserving too little.
		for _, bar := range d.Nodes {
			if bar.Container || bar.Text || bar.Overlay {
				continue
			}
			idx, ok := index[lifeline(bar.Parent)]
			if !ok {
				continue
			}
			if idx == g {
				left = math.Max(left, barInset(bar, lifelines[g], true))
			}
			if idx == g+1 {
				right = math.Max(right, barInset(bar, lifelines[g+1], false))
			}
		}
		grow(g, w+labelPad+left+right)
	}
	centers := make([]float64, n)
	centers[0] = lifelines[0].Box.Center().X
	for i := 1; i < n; i++ {
		centers[i] = centers[i-1] + gaps[i-1]
	}
	return centers
}

// labelGap is the gap a message's label goes in and how far the room on each
// side starts from the lifeline center: the gap nearest the middle of the
// lifelines the message joins, with an end's room starting at the edge of the
// activation bar it sits on rather than at the center line. Spacing reserves
// this gap and placeMessageLabels puts the label in it, so both use this to
// agree on which gap and how much room.
func labelGap(m *message, index map[*tidy.Node]int, lifelines []*tidy.Node) (g int, left, right float64, ok bool) {
	if m.src == nil || m.dst == nil {
		return 0, 0, 0, false
	}
	a, b := index[m.src], index[m.dst]
	if a > b {
		a, b = b, a
	}
	if b-a < 1 || b >= len(lifelines) {
		return 0, 0, 0, false
	}
	g = a + (b-a-1)/2
	if g == a {
		end, life := m.w.Dst, m.dst
		if index[m.src] == a {
			end, life = m.w.Src, m.src
		}
		left = barInset(end, life, true)
	}
	if g == b-1 {
		end, life := m.w.Src, m.src
		if index[m.src] != b {
			end, life = m.w.Dst, m.dst
		}
		right = barInset(end, life, false)
	}
	return g, left, right, true
}

// barInset is how far the side of a message end lies from its lifeline's center
// in the direction the message leaves it. An activation bar is drawn beside the
// line, so the label's room starts at the bar's edge, not the center line.
func barInset(end, life *tidy.Node, right bool) float64 {
	if end == nil || end == life {
		return 0
	}
	off := end.Box.Center().X - life.Box.Center().X
	if right {
		return math.Max(0, off+end.Box.W/2)
	}
	return math.Max(0, end.Box.W/2-off)
}

// rowMap maps an old height to a new one, piecewise linearly through the
// messages' old and new heights, for everything placed by height: bars
// without messages, notes, frames without messages.
func rowMap(msgs []*message, top float64) func(float64) float64 {
	pts := make([]rowPt, len(msgs))
	for i, m := range msgs {
		pts[i] = rowPt{m.origY, m.y}
	}
	return rowMapPts(pts, top)
}

// rowPt is a height before and after layout.
type rowPt struct{ from, to float64 }

// rowMapPts is rowMap through any heights, in time order.
func rowMapPts(rows []rowPt, top float64) func(float64) float64 {
	pts := []rowPt{{top, top}}
	for _, r := range rows {
		if r.from > pts[len(pts)-1].from+0.5 {
			pts = append(pts, r)
		}
	}
	return func(y float64) float64 {
		if y <= pts[0].from {
			return y - pts[0].from + pts[0].to
		}
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			if y <= b.from {
				return a.to + (y-a.from)*(b.to-a.to)/(b.from-a.from)
			}
		}
		last := pts[len(pts)-1]
		return y - last.from + last.to
	}
}

// placeBars stretches each activation bar over the messages it sends and
// receives, centered where it was relative to its lifeline.
func placeBars(d *tidy.Diagram, lifelines []*tidy.Node, msgs []*message, origCenter map[*tidy.Node]float64, remap func(float64) float64, step float64) {
	for _, n := range d.Nodes {
		l := lifeline(n.Parent)
		if l == nil || n == l {
			continue
		}
		off := n.Box.Center().X - origCenter[l]
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, m := range msgs {
			if m.w.Src != n && m.w.Dst != n {
				continue
			}
			lo, hi = math.Min(lo, m.y), math.Max(hi, m.y)
			if m.self {
				hi = math.Max(hi, m.y+step)
			}
		}
		var y0, y1 float64
		if math.IsInf(lo, 1) {
			y0, y1 = remap(n.Box.Y), remap(n.Box.Bottom())
		} else {
			y0, y1 = lo-barPad, hi+barPad
		}
		c := l.Box.Center().X + off
		n.Box = geom.Rect{X: c - n.Box.W/2, Y: y0, W: n.Box.W, H: math.Max(y1-y0, 2*barPad)}
	}
}

// placeOthers moves notes and other shapes along with the nearest lifeline
// and the rows. Frames and gate dots are placed on their own.
func placeOthers(d *tidy.Diagram, lifelines []*tidy.Node, msgs []*message, origCenter map[*tidy.Node]float64, remap func(float64) float64, dots map[*tidy.Node]bool) {
	nearest := func(x float64) *tidy.Node {
		var best *tidy.Node
		bd := math.Inf(1)
		for _, l := range lifelines {
			if dd := math.Abs(origCenter[l] - x); dd < bd {
				best, bd = l, dd
			}
		}
		return best
	}
	for _, n := range d.Nodes {
		if n.V.Shape == "umlLifeline" || lifeline(n.Parent) != nil || n.Overlay || dots[n] {
			continue
		}
		l := nearest(n.Box.Center().X)
		dx := l.Box.Center().X - origCenter[l]
		y0 := remap(n.Box.Y)
		n.Box = geom.Rect{X: n.Box.X + dx, Y: y0, W: n.Box.W, H: n.Box.H}
	}
}

// routeMessages draws every message as a horizontal line at its row between
// the centers of the shapes it joins, and a self call as a loop out to the
// right and back one row lower. Free ends move with the line; attached ends
// get waypoints at the row.
func routeMessages(msgs []*message, step float64) {
	for _, m := range msgs {
		w := m.w
		at := func(n *tidy.Node, g *gate) float64 {
			if g != nil {
				return g.place.X
			}
			return n.Box.Center().X
		}
		var srcNode, dstNode *tidy.Node
		if m.srcGate == nil {
			srcNode = endNode(w.Src, m.src)
		}
		if m.dstGate == nil {
			dstNode = endNode(w.Dst, m.dst)
		}
		sx, dx := at(srcNode, m.srcGate), at(dstNode, m.dstGate)
		right := dx >= sx
		// A message leaves an activation bar from the side facing its target
		// and enters one on the side facing its source; a bare lifeline is met
		// at its center line.
		if srcNode != nil && lifeline(srcNode) != srcNode {
			if right {
				sx = srcNode.Box.Right()
			} else {
				sx = srcNode.Box.X
			}
		}
		if dstNode != nil && lifeline(dstNode) != dstNode {
			if right {
				dx = dstNode.Box.X
			} else {
				dx = dstNode.Box.Right()
			}
		}
		y1 := m.y
		if m.self {
			y1 = m.y + step
			out := srcNode.Box.Right() + selfWidth
			if lifeline(srcNode) == srcNode {
				out = srcNode.Box.Center().X + selfWidth
			}
			w.Points = []geom.Point{{X: out, Y: m.y}, {X: out, Y: y1}}
			if lifeline(srcNode) != srcNode {
				sx = srcNode.Box.Right()
			}
			if lifeline(dstNode) != dstNode {
				dx = dstNode.Box.Right()
			}
		} else {
			// An end attached to a lifeline or bar is drawn to its outline at
			// any height; a waypoint pins it to the row. Free ends and gate
			// dots already sit on the row.
			w.Points = nil
			if w.Src != nil && m.srcGate == nil {
				w.Points = append(w.Points, geom.Point{X: sx, Y: m.y})
			}
			if w.Dst != nil && m.dstGate == nil {
				w.Points = append(w.Points, geom.Point{X: dx, Y: m.y})
			}
		}
		if w.Src == nil {
			w.SrcPoint = &geom.Point{X: sx, Y: m.y}
		}
		if w.Dst == nil {
			w.DstPoint = &geom.Point{X: dx, Y: y1}
		}
		for _, k := range []string{"exitX", "exitY", "exitDx", "exitDy", "exitPerimeter", "entryX", "entryY", "entryDx", "entryDy", "entryPerimeter"} {
			w.Style.Del(k)
		}
		w.Routed = true
	}
}

// endNode is the shape a message end is drawn to: the activation bar or
// lifeline it is attached to, or for a free end the lifeline it lies on.
func endNode(attached, life *tidy.Node) *tidy.Node {
	if attached != nil {
		return attached
	}
	return life
}

// placeMessageLabels moves a message label that a lifeline's center line or an
// activation bar strikes into the gap spacing reserved for it. A label that is
// already clear keeps the place the author gave it. Only a wire's first label
// is placed here; further labels (child edge-label cells) are left to the
// labels operation. A message with a free end has no gap reserved for it, so
// its label is left alone as well.
func placeMessageLabels(d *tidy.Diagram, msgs []*message, lifelines []*tidy.Node, index map[*tidy.Node]int) {
	for _, m := range msgs {
		if m.src == nil || m.dst == nil {
			continue
		}
		boxes := m.w.LabelBoxes()
		if len(boxes) == 0 || !labelStruck(boxes[0], d, lifelines) {
			continue
		}
		if m.self {
			a := index[m.src]
			out := 0.0
			for _, p := range m.w.Path() {
				out = math.Max(out, p.X)
			}
			center := out + boxes[0].W/2 + labelRoom
			if a+1 < len(lifelines) {
				center = (out + lifelines[a+1].Box.Center().X) / 2
			}
			m.w.PlaceLabel(0, geom.Point{X: center - out})
			continue
		}
		g, leftInset, rightInset, ok := labelGap(m, index, lifelines)
		if !ok {
			continue
		}
		left := lifelines[g].Box.Center().X + leftInset
		right := lifelines[g+1].Box.Center().X - rightInset
		// Bars on the lifelines at the gap's edges reach into the gap too.
		for _, n := range d.Nodes {
			if n.Container || n.Text || n.Overlay {
				continue
			}
			idx, ok := index[lifeline(n.Parent)]
			if !ok || n.Box.Bottom() < boxes[0].Y || n.Box.Y > boxes[0].Bottom() {
				continue
			}
			if idx == g {
				left = math.Max(left, n.Box.Right())
			}
			if idx == g+1 {
				right = math.Min(right, n.Box.X)
			}
		}
		pl := m.w.Path()
		if right <= left || len(pl) < 2 || pl[0].X == pl[len(pl)-1].X {
			continue
		}
		t := ((left+right)/2 - pl[0].X) / (pl[len(pl)-1].X - pl[0].X)
		m.w.PlaceLabel(math.Max(-1, math.Min(1, 2*t-1)), geom.Point{})
	}
}

// labelStruck reports whether a label box would count as struck by a lifeline's
// center line or an activation bar, the way Measure counts them.
func labelStruck(b geom.Rect, d *tidy.Diagram, lifelines []*tidy.Node) bool {
	for _, l := range lifelines {
		head := geom.ParseNumber(l.V.Style.Value("size", "40"))
		line := geom.Segment{A: geom.Point{X: l.Box.Center().X, Y: l.Box.Y + head}, B: geom.Point{X: l.Box.Center().X, Y: l.Box.Bottom()}}
		if line.CrossesRect(b, 2) {
			return true
		}
	}
	for _, n := range d.Nodes {
		if n.Container || n.Text || n.Overlay || lifeline(n.Parent) == nil {
			continue
		}
		if b.Inset(1).Overlaps(n.Box.Inset(1)) && !n.Box.ContainsRect(b) {
			return true
		}
	}
	return false
}
