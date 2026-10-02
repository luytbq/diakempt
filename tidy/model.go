// Package tidy holds the working copy of one diagram's geometry, the quality
// score, and the general operations of the safe and normal levels. Operations
// change the working copy only; Patch writes the result into the document.
package tidy

import (
	"math"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/view"
)

// Diagram is the working copy of one diagram.
type Diagram struct {
	Seg   *segment.Diagram
	Nodes []*Node // parents before children
	Wires []*Wire
	byV   map[*view.Node]*Node
	tm    *text.Measure
}

// Node is a node's working geometry.
type Node struct {
	V         *view.Node
	Box       geom.Rect
	Orig      geom.Rect
	Parent    *Node
	Children  []*Node
	Container bool
	Text      bool
}

// Wire is a wire's working geometry.
type Wire struct {
	V        *view.Wire
	Src, Dst *Node
	Points   []geom.Point
	SrcPoint *geom.Point
	DstPoint *geom.Point
	Style    doc.Style
	// Routed is set when an operation chose this wire's route, so its anchors
	// are written with it.
	Routed bool

	origPoints       []geom.Point
	origSrc, origDst *geom.Point
	origStyle        string
	labels           []label
	origLabels       []label
}

type label struct {
	cell   *doc.Cell // nil for the wire's own value
	pos    float64   // -1 at the source to 1 at the target
	offset geom.Point
	w, h   float64
}

// New builds the working copy of a segmented diagram.
func New(seg *segment.Diagram, tm *text.Measure) *Diagram {
	d := &Diagram{Seg: seg, byV: map[*view.Node]*Node{}, tm: tm}
	var add func(vn *view.Node)
	in := map[*view.Node]bool{}
	for _, vn := range seg.Nodes {
		in[vn] = true
	}
	added := map[*view.Node]bool{}
	add = func(vn *view.Node) {
		if added[vn] || !in[vn] {
			return
		}
		if vn.Parent != nil {
			add(vn.Parent)
		}
		added[vn] = true
		n := &Node{V: vn, Box: vn.Box, Orig: vn.Box, Container: vn.Container, Text: vn.TextOnly}
		if vn.Parent != nil {
			if p := d.byV[vn.Parent]; p != nil {
				n.Parent = p
				p.Children = append(p.Children, n)
			}
		}
		d.byV[vn] = n
		d.Nodes = append(d.Nodes, n)
	}
	for _, vn := range seg.Nodes {
		add(vn)
	}
	for _, vw := range seg.Wires {
		w := &Wire{V: vw, Src: d.byV[vw.Src], Dst: d.byV[vw.Dst], Style: vw.Style}
		w.Points = append([]geom.Point(nil), vw.Points...)
		w.origPoints = vw.Points
		if vw.SrcPoint != nil && w.Src == nil {
			p := *vw.SrcPoint
			w.SrcPoint, w.origSrc = &p, vw.SrcPoint
		}
		if vw.DstPoint != nil && w.Dst == nil {
			p := *vw.DstPoint
			w.DstPoint, w.origDst = &p, vw.DstPoint
		}
		w.origStyle = vw.Style.String()
		w.labels = d.readLabels(vw)
		w.origLabels = append([]label(nil), w.labels...)
		d.Wires = append(d.Wires, w)
	}
	return d
}

func (d *Diagram) readLabels(vw *view.Wire) []label {
	var out []label
	st := vw.Style
	if t := view.PlainText(vw.Cell.Label(), st.Value("html", "0") == "1"); t != "" {
		x, _, off := vw.Cell.LabelPosition()
		w, h := d.measure(t, st)
		out = append(out, label{pos: x, offset: off, w: w, h: h})
	}
	for _, c := range vw.Labels {
		cs := c.Style()
		t := view.PlainText(c.Label(), cs.Value("html", "0") == "1")
		if t == "" {
			continue
		}
		x, _, off := c.LabelPosition()
		w, h := d.measure(t, cs)
		out = append(out, label{cell: c, pos: x, offset: off, w: w, h: h})
	}
	return out
}

// measure returns the box of a text at its style's font size, measured as
// Verdana scaled from 12px.
func (d *Diagram) measure(t string, st doc.Style) (float64, float64) {
	size := geom.ParseNumber(st.Value("fontSize", "12"))
	if size <= 0 {
		size = 12
	}
	scale := size / 12
	var lines []string
	for _, l := range splitLines(t) {
		lines = append(lines, l)
	}
	w, h := d.tm.Box(lines)
	return w*scale + 4, h*scale + 2
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// Path returns the wire's current line.
func (w *Wire) Path() geom.Polyline {
	e := view.Ends{SrcPoint: w.SrcPoint, DstPoint: w.DstPoint}
	if w.Src != nil {
		e.Src = &w.Src.Box
	}
	if w.Dst != nil {
		e.Dst = &w.Dst.Box
	}
	return view.Route(e, w.Points, w.Style)
}

// LabelBoxes returns the boxes of the wire's labels on its current line.
func (w *Wire) LabelBoxes() []geom.Rect {
	if len(w.labels) == 0 {
		return nil
	}
	pl := w.Path()
	var out []geom.Rect
	for _, l := range w.labels {
		c := pointAlong(pl, (l.pos+1)/2).Add(l.offset)
		out = append(out, geom.Rect{X: c.X - l.w/2, Y: c.Y - l.h/2, W: l.w, H: l.h})
	}
	return out
}

// pointAlong returns the point at share t of a polyline's length.
func pointAlong(pl geom.Polyline, t float64) geom.Point {
	if len(pl) == 0 {
		return geom.Point{}
	}
	total := pl.Len()
	if total == 0 {
		return pl[0]
	}
	want := math.Max(0, math.Min(1, t)) * total
	for _, s := range pl.Segments() {
		l := s.Len()
		if want <= l {
			f := want / l
			return geom.Point{X: s.A.X + (s.B.X-s.A.X)*f, Y: s.A.Y + (s.B.Y-s.A.Y)*f}
		}
		want -= l
	}
	return pl[len(pl)-1]
}

// Leaves returns the nodes that are not containers.
func (d *Diagram) Leaves() []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		if !n.Container {
			out = append(out, n)
		}
	}
	return out
}

// Within reports whether n is m or inside m.
func (n *Node) Within(m *Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p == m {
			return true
		}
	}
	return false
}

// Move shifts a node and everything inside it, along with the waypoints of
// wires that run entirely inside it and the free ends of wires leaving it.
func (d *Diagram) Move(n *Node, by geom.Point) {
	if by == (geom.Point{}) {
		return
	}
	var shift func(m *Node)
	shift = func(m *Node) {
		m.Box = m.Box.Move(by)
		for _, c := range m.Children {
			shift(c)
		}
	}
	shift(n)
	for _, w := range d.Wires {
		srcIn := w.Src != nil && w.Src.Within(n)
		dstIn := w.Dst != nil && w.Dst.Within(n)
		if (srcIn || w.Src == nil) && (dstIn || w.Dst == nil) && (srcIn || dstIn) {
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
}

// Changed reports whether the working copy differs from what was read.
func (d *Diagram) Changed() bool {
	for _, n := range d.Nodes {
		if n.Box != n.Orig {
			return true
		}
	}
	for _, w := range d.Wires {
		if w.changed() {
			return true
		}
	}
	return false
}

func (w *Wire) changed() bool {
	if w.Style.String() != w.origStyle || !samePoints(w.Points, w.origPoints) {
		return true
	}
	for i := range w.labels {
		if w.labels[i] != w.origLabels[i] {
			return true
		}
	}
	return !samePtr(w.SrcPoint, w.origSrc) || !samePtr(w.DstPoint, w.origDst)
}

func samePoints(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func samePtr(a, b *geom.Point) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Snapshot is a saved copy of the working geometry.
type Snapshot struct {
	boxes  []geom.Rect
	points [][]geom.Point
	src    []*geom.Point
	dst    []*geom.Point
	styles []doc.Style
	routed []bool
	labels [][]label
}

// Save copies the working geometry.
func (d *Diagram) Save() Snapshot {
	var s Snapshot
	for _, n := range d.Nodes {
		s.boxes = append(s.boxes, n.Box)
	}
	for _, w := range d.Wires {
		s.points = append(s.points, append([]geom.Point(nil), w.Points...))
		s.src = append(s.src, copyPt(w.SrcPoint))
		s.dst = append(s.dst, copyPt(w.DstPoint))
		s.styles = append(s.styles, doc.ParseStyle(w.Style.String()))
		s.routed = append(s.routed, w.Routed)
		s.labels = append(s.labels, append([]label(nil), w.labels...))
	}
	return s
}

// Restore puts saved geometry back.
func (d *Diagram) Restore(s Snapshot) {
	for i, n := range d.Nodes {
		n.Box = s.boxes[i]
	}
	for i, w := range d.Wires {
		w.Points = append([]geom.Point(nil), s.points[i]...)
		w.SrcPoint, w.DstPoint = copyPt(s.src[i]), copyPt(s.dst[i])
		w.Style = doc.ParseStyle(s.styles[i].String())
		w.Routed = s.routed[i]
		w.labels = append([]label(nil), s.labels[i]...)
	}
}

func copyPt(p *geom.Point) *geom.Point {
	if p == nil {
		return nil
	}
	q := *p
	return &q
}
