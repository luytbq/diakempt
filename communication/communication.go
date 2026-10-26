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
	w      *tidy.Wire
	number []int
	text   geom.Point // size of its label
}

// Layout places the objects and actors of a working copy and routes the
// links between them.
func Layout(d *tidy.Diagram) error {
	var nodes, others []*tidy.Node
	for _, n := range d.Nodes {
		switch {
		case n.Container:
			return &Unsupported{fmt.Sprintf("container %s in a communication diagram", n.V.ID)}
		case n.Text || n.V.Shape == "note" || n.Overlay:
			others = append(others, n)
		default:
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
		l := &link{w: w, number: detect.MessageNumber(w.V.Text)}
		lw, lh := d.TextSize(w.V.Text, w.Style)
		l.text = geom.Point{X: lw, Y: lh}
		links = append(links, l)
		adj[w.Src] = append(adj[w.Src], l)
		if w.Dst != w.Src {
			adj[w.Dst] = append(adj[w.Dst], l)
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
	// The diagram starts at the actor of the first message, else at either
	// end of it, else at the best connected object.
	var root *tidy.Node
	if len(ordered) > 0 && ordered[0].number != nil {
		w := ordered[0].w
		root = w.Src
		if w.Dst.V.Shape == "umlActor" && w.Src.V.Shape != "umlActor" {
			root = w.Dst
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
	col := map[*tidy.Node]int{root: 0}
	columns := [][]*tidy.Node{{root}}
	queue := []*tidy.Node{root}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, l := range adj[n] {
			o := l.w.Dst
			if o == n {
				o = l.w.Src
			}
			if _, seen := col[o]; seen {
				continue
			}
			c := col[n] + 1
			col[o] = c
			if c == len(columns) {
				columns = append(columns, nil)
			}
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
	// Room for labels: a column gap fits the widest label of the links
	// crossing it, and objects in a column sit far enough apart that the
	// labels of links fanning out to them do not touch.
	gapX := make([]float64, len(columns))
	tallest := make([]float64, len(columns))
	loopRoom := map[*tidy.Node]float64{}
	for _, l := range links {
		a, b := col[l.w.Src], col[l.w.Dst]
		if l.w.Src == l.w.Dst {
			loopRoom[l.w.Src] = math.Max(loopRoom[l.w.Src], loopUp+l.text.Y)
			continue
		}
		if a > b {
			a, b = b, a
		}
		if b == a+1 {
			gapX[a] = math.Max(gapX[a], l.text.X+2*labelPad)
		}
		tallest[b] = math.Max(tallest[b], l.text.Y)
	}
	x := 0.0
	center := map[*tidy.Node]geom.Point{}
	for c, ns := range columns {
		// One width per column, so a link reaching an object at the middle
		// of its side stays clear of the others in the column. Actors keep
		// their figure's size.
		w := 0.0
		for _, n := range ns {
			w = math.Max(w, n.Box.W)
		}
		for _, n := range ns {
			if n.V.Shape != "umlActor" {
				n.Box = geom.Rect{X: n.Box.Center().X - w/2, Y: n.Box.Y, W: w, H: n.Box.H}
			}
		}
		y := 0.0
		for _, n := range ns {
			y += loopRoom[n]
			center[n] = geom.Point{X: x + w/2, Y: y + n.Box.H/2}
			step := math.Max(n.Box.H+minGapY, 2*(tallest[c]+labelPad))
			y += step
		}
		// The column centers on the objects it is reached from.
		if c > 0 {
			sum, cnt := 0.0, 0
			for _, n := range ns {
				for _, l := range adj[n] {
					o := l.w.Dst
					if o == n {
						o = l.w.Src
					}
					if col[o] == c-1 {
						sum += center[o].Y
						cnt++
					}
				}
			}
			if cnt > 0 {
				lo, hi := center[ns[0]].Y, center[ns[len(ns)-1]].Y
				shift := sum/float64(cnt) - (lo+hi)/2
				if top := center[ns[0]].Y - ns[0].Box.H/2 - loopRoom[ns[0]]; top+shift < 0 {
					shift = -top
				}
				for _, n := range ns {
					p := center[n]
					center[n] = geom.Point{X: p.X, Y: p.Y + shift}
				}
			}
		}
		x += w + math.Max(minGapX, gapX[c])
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
		if w.Src == w.Dst {
			b := w.Src.Box
			c := b.Center().X
			w.Points = []geom.Point{{X: c - loopHalf, Y: b.Y - loopUp}, {X: c + loopHalf, Y: b.Y - loopUp}}
			port(w, "exit", (c-loopHalf-b.X)/b.W, 0)
			port(w, "entry", (c+loopHalf-b.X)/b.W, 0)
			continue
		}
		a, b := col[w.Src], col[w.Dst]
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
			if n != w.Src && n != w.Dst && seg.CrossesRect(n.Box, -4) {
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

// gap returns the distance between two boxes, zero when they touch or overlap.
func gap(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}
