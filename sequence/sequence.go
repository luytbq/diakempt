// Package sequence lays out UML sequence diagrams drawn with the editor's
// lifeline shape: lifelines in a row in the author's order, spaced so message
// labels fit; messages in their time order, one per row, all horizontal;
// activation bars spanning exactly the messages they send and receive; frames
// and notes following the messages they belong with.
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
	barPad      = 10 // an activation bar beyond its first and last message
	tailPad     = 40 // lifeline below the last message
	frameHead   = 30 // above a frame's first message, for its tab
	frameFoot   = 20 // below its last message
	frameMargin = 60 // beside the outermost lifeline in a frame
)

// Unsupported explains why a diagram cannot be laid out as a sequence.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

type message struct {
	w        *tidy.Wire
	src, dst *tidy.Node // lifelines
	self     bool
	origY    float64
	y        float64 // new row
	doc      int
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
	if len(lifelines) < 2 {
		return &Unsupported{"fewer than two UML lifelines (hand-drawn lifelines are not laid out)"}
	}
	for _, n := range d.Nodes {
		if n.Container && n.V.Shape != "umlLifeline" {
			return &Unsupported{fmt.Sprintf("container %s is not a lifeline", n.V.ID)}
		}
	}
	var msgs []*message
	for i, w := range d.Wires {
		src, dst := lifeline(w.Src), lifeline(w.Dst)
		if src == nil || dst == nil {
			return &Unsupported{fmt.Sprintf("message %s does not join two lifelines", w.V.Cell.ID)}
		}
		msgs = append(msgs, &message{w: w, src: src, dst: dst, self: src == dst, origY: rowOf(w), doc: i})
	}
	sort.SliceStable(msgs, func(i, j int) bool {
		if math.Abs(msgs[i].origY-msgs[j].origY) > 1 {
			return msgs[i].origY < msgs[j].origY
		}
		return msgs[i].doc < msgs[j].doc
	})
	sort.SliceStable(lifelines, func(i, j int) bool { return lifelines[i].Box.Center().X < lifelines[j].Box.Center().X })
	index := map[*tidy.Node]int{}
	for i, l := range lifelines {
		index[l] = i
	}
	top, head := math.Inf(1), 0.0
	for _, l := range lifelines {
		top = math.Min(top, l.Box.Y)
		head = math.Max(head, geom.ParseNumber(l.V.Style.Value("size", "40")))
	}
	step := float64(minRow)
	for _, m := range msgs {
		_, h := d.TextSize(m.w.V.Text, m.w.Style)
		step = math.Max(step, h+18)
	}
	// Rows.
	y := top + head + firstRow
	for _, m := range msgs {
		m.y = y
		y += step
		if m.self {
			y += step
		}
	}
	bottom := y - step + tailPad
	centers := spacing(d, lifelines, msgs, index)
	origCenter := map[*tidy.Node]float64{}
	for i, l := range lifelines {
		origCenter[l] = l.Box.Center().X
		l.Box = geom.Rect{X: centers[i] - l.Box.W/2, Y: top, W: l.Box.W, H: math.Max(bottom-top, head+tailPad)}
	}
	remap := rowMap(msgs, top)
	placeBars(d, lifelines, msgs, origCenter, remap, step)
	placeOthers(d, lifelines, msgs, origCenter, remap, step)
	routeMessages(msgs, step)
	keepCorner(d)
	return nil
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
// wide enough apart for every message label between them. The first lifeline
// keeps its center.
func spacing(d *tidy.Diagram, lifelines []*tidy.Node, msgs []*message, index map[*tidy.Node]int) []float64 {
	n := len(lifelines)
	gaps := make([]float64, n-1)
	for i := range gaps {
		gaps[i] = (lifelines[i].Box.W+lifelines[i+1].Box.W)/2 + minGap
	}
	need := func(m *message) (lo, hi int, width float64) {
		w, _ := d.TextSize(m.w.V.Text, m.w.Style)
		a, b := index[m.src], index[m.dst]
		if a > b {
			a, b = b, a
		}
		if m.self {
			return a, a + 1, selfWidth + w + 10
		}
		return a, b, w + labelPad
	}
	for _, m := range msgs {
		lo, hi, width := need(m)
		if lo == hi || hi >= n {
			continue
		}
		have := 0.0
		for k := lo; k < hi; k++ {
			have += gaps[k]
		}
		if have < width {
			extra := (width - have) / float64(hi-lo)
			for k := lo; k < hi; k++ {
				gaps[k] += extra
			}
		}
	}
	centers := make([]float64, n)
	centers[0] = lifelines[0].Box.Center().X
	for i := 1; i < n; i++ {
		centers[i] = centers[i-1] + gaps[i-1]
	}
	return centers
}

// rowMap maps an old height to a new one, piecewise linearly through the
// messages' old and new heights, for everything placed by height: bars
// without messages, notes, frames without messages.
func rowMap(msgs []*message, top float64) func(float64) float64 {
	type pt struct{ from, to float64 }
	pts := []pt{{top, top}}
	for _, m := range msgs {
		if m.origY > pts[len(pts)-1].from+0.5 {
			pts = append(pts, pt{m.origY, m.y})
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

// placeOthers moves frames around the messages they enclosed, and notes and
// other shapes along with the nearest lifeline and the rows.
func placeOthers(d *tidy.Diagram, lifelines []*tidy.Node, msgs []*message, origCenter map[*tidy.Node]float64, remap func(float64) float64, step float64) {
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
		if n.V.Shape == "umlLifeline" || lifeline(n.Parent) != nil {
			continue
		}
		if n.V.Shape == "umlFrame" {
			var inside []*message
			for _, m := range msgs {
				x0 := math.Min(origCenter[m.src], origCenter[m.dst])
				x1 := math.Max(origCenter[m.src], origCenter[m.dst])
				if m.origY >= n.Box.Y && m.origY <= n.Box.Bottom() && x1 >= n.Box.X && x0 <= n.Box.Right() {
					inside = append(inside, m)
				}
			}
			if len(inside) > 0 {
				lo, hi := math.Inf(1), math.Inf(-1)
				left, right := math.Inf(1), math.Inf(-1)
				for _, m := range inside {
					lo, hi = math.Min(lo, m.y), math.Max(hi, m.y)
					if m.self {
						hi = math.Max(hi, m.y+step)
					}
					for _, l := range []*tidy.Node{m.src, m.dst} {
						c := l.Box.Center().X
						left, right = math.Min(left, c), math.Max(right, c)
					}
				}
				n.Box = geom.Rect{X: left - frameMargin, Y: lo - frameHead, W: right - left + 2*frameMargin, H: hi - lo + frameHead + frameFoot}
				continue
			}
		}
		l := nearest(n.Box.Center().X)
		dx := l.Box.Center().X - origCenter[l]
		y0 := remap(n.Box.Y)
		n.Box = geom.Rect{X: n.Box.X + dx, Y: y0, W: n.Box.W, H: n.Box.H}
	}
}

// routeMessages draws every message as a horizontal line at its row between
// the centers of the shapes it joins, and a self call as a loop out to the
// right and back one row lower.
func routeMessages(msgs []*message, step float64) {
	for _, m := range msgs {
		w := m.w
		// A message leaves an activation bar from the side facing its target
		// and enters one on the side facing its source; a bare lifeline is met
		// at its center line.
		sx, dx := w.Src.Box.Center().X, w.Dst.Box.Center().X
		right := dx >= sx
		if lifeline(w.Src) != w.Src {
			if right {
				sx = w.Src.Box.Right()
			} else {
				sx = w.Src.Box.X
			}
		}
		if lifeline(w.Dst) != w.Dst {
			if right {
				dx = w.Dst.Box.X
			} else {
				dx = w.Dst.Box.Right()
			}
		}
		if m.self {
			out := w.Src.Box.Right() + selfWidth
			w.Points = []geom.Point{{X: out, Y: m.y}, {X: out, Y: m.y + step}}
		} else {
			w.Points = []geom.Point{{X: sx, Y: m.y}, {X: dx, Y: m.y}}
		}
		for _, k := range []string{"exitX", "exitY", "exitDx", "exitDy", "exitPerimeter", "entryX", "entryY", "entryDx", "entryDy", "entryPerimeter"} {
			w.Style.Del(k)
		}
		w.Routed = true
	}
}

// keepCorner moves everything so the diagram's top-left corner is where it was.
func keepCorner(d *tidy.Diagram) {
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
	}
}
