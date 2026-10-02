// Package view builds the picture a person sees from a page's cells: logical
// nodes, containers and wires in page coordinates. It writes nothing; later
// steps read the view to decide what to change and write through doc.
package view

import (
	"html"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
)

// View is one page seen as nodes and wires.
type View struct {
	Page  *doc.Page
	Nodes []*Node
	Wires []*Wire
	// byCell maps every cell that belongs to a node to that node.
	byCell map[string]*Node
}

// Node is what a person sees as one shape. Cells[0] is the primary cell, the
// one wires attach to; the others are text boxes laid over it.
type Node struct {
	ID    string
	Cells []*doc.Cell
	Box   geom.Rect
	Shape string
	Text  string
	Style doc.Style
	// Container is set for swimlanes, pools and shapes holding other shapes.
	Container bool
	// TextOnly is set for shapes that draw no outline: text, labels, notes
	// without a border.
	TextOnly bool
	// Parent is the container this node sits in, nil at the top.
	Parent   *Node
	Children []*Node
}

// Wire is a draw.io edge seen in page coordinates.
type Wire struct {
	Cell *doc.Cell
	// Src and Dst are the attached nodes, nil for a free end.
	Src, Dst *Node
	// SrcRef and DstRef are the raw terminal ids, which may name cells that are
	// not nodes, such as other wires.
	SrcRef, DstRef string
	// Path runs from the source end through the waypoints to the target end.
	Path geom.Polyline
	// Labels are label cells placed on the wire, plus the wire's own value.
	Labels []*doc.Cell
	Text   string
	Style  doc.Style
}

// Build reads a page into a view.
func Build(p *doc.Page) *View {
	v := &View{Page: p, byCell: map[string]*Node{}}
	var shapes []*doc.Cell
	for _, c := range p.Cells {
		if c.IsVertex() && !c.RelativeGeometry() && !isGroup(c) && !c.AbsBounds().Empty() {
			shapes = append(shapes, c)
		}
	}
	nodes := map[string]*Node{}
	for _, c := range shapes {
		st := c.Style()
		n := &Node{
			ID: c.ID, Cells: []*doc.Cell{c}, Box: c.AbsBounds(), Shape: st.Shape(), Style: st,
			Text: PlainText(c.Label(), st.Value("html", "0") == "1"),
		}
		n.TextOnly = isTextOnly(st)
		n.Container = isContainer(c, st)
		nodes[c.ID] = n
	}
	// A text box laid over a shape is part of that shape.
	merged := map[string]bool{}
	for _, c := range shapes {
		n := nodes[c.ID]
		if !n.TextOnly || n.Container {
			continue
		}
		if host := hostOf(n, shapes, nodes); host != nil {
			host.Cells = append(host.Cells, c)
			if host.Text == "" {
				host.Text = n.Text
			} else if n.Text != "" {
				host.Text += "\n" + n.Text
			}
			merged[c.ID] = true
			v.byCell[c.ID] = host
		}
	}
	for _, c := range shapes {
		if merged[c.ID] {
			continue
		}
		n := nodes[c.ID]
		v.Nodes = append(v.Nodes, n)
		v.byCell[c.ID] = n
	}
	// Parents: the nearest container cell up the chain, seeing through groups.
	for _, n := range v.Nodes {
		cur := p.Cell(n.Cells[0].Parent())
		for i := 0; cur != nil && i < 64; i++ {
			if pn := v.byCell[cur.ID]; pn != nil && pn.Container {
				n.Parent = pn
				pn.Children = append(pn.Children, n)
				break
			}
			cur = p.Cell(cur.Parent())
		}
	}
	for _, c := range p.Cells {
		if c.IsEdge() {
			v.Wires = append(v.Wires, v.buildWire(c))
		}
	}
	return v
}

// hostOf finds the non-text shape a text box mostly lies on: at least 80% of the
// text box's area inside it. The smallest such shape wins.
func hostOf(t *Node, shapes []*doc.Cell, nodes map[string]*Node) *Node {
	var best *Node
	for _, c := range shapes {
		n := nodes[c.ID]
		if n == t || n.TextOnly || n.Container {
			continue
		}
		in := t.Box.Intersect(n.Box)
		if in.W*in.H < 0.8*t.Box.W*t.Box.H {
			continue
		}
		if best == nil || n.Box.W*n.Box.H < best.Box.W*best.Box.H {
			best = n
		}
	}
	return best
}

func isGroup(c *doc.Cell) bool {
	st := c.Style()
	return st.Shape() == "group" || (len(st.Names()) > 0 && st.Names()[0] == "group")
}

func isTextOnly(st doc.Style) bool {
	switch st.Shape() {
	case "text", "edgeLabel", "label":
		return true
	}
	return st.Value("strokeColor", "") == "none" && st.Value("fillColor", "none") == "none"
}

func isContainer(c *doc.Cell, st doc.Style) bool {
	if st.Shape() == "swimlane" || st.Shape() == "table" || st.Value("container", "0") == "1" || st.Value("swimlane", "") != "" {
		return true
	}
	for _, ch := range c.Page.Children(c.ID) {
		if ch.IsVertex() && !ch.RelativeGeometry() {
			return true
		}
	}
	return false
}

// NodeOf returns the node a cell belongs to, or nil.
func (v *View) NodeOf(id string) *Node { return v.byCell[id] }

func (v *View) buildWire(c *doc.Cell) *Wire {
	w := &Wire{Cell: c, SrcRef: c.Source(), DstRef: c.Target(), Style: c.Style()}
	w.Src, w.Dst = v.byCell[w.SrcRef], v.byCell[w.DstRef]
	text := []string{PlainText(c.Label(), w.Style.Value("html", "0") == "1")}
	for _, ch := range c.Page.Children(c.ID) {
		if ch.IsVertex() && ch.RelativeGeometry() {
			w.Labels = append(w.Labels, ch)
			text = append(text, PlainText(ch.Label(), ch.Style().Value("html", "0") == "1"))
		}
	}
	w.Text = strings.TrimSpace(strings.Join(text, "\n"))
	w.Path = v.path(w)
	return w
}

// End returns the position of a wire end: the stored point of a free end, or
// for an attached end the point where the wire meets its node.
func (w *Wire) End(source bool) (geom.Point, bool) {
	if len(w.Path) == 0 {
		return geom.Point{}, false
	}
	if source {
		return w.Path[0], true
	}
	return w.Path[len(w.Path)-1], true
}

// path approximates the line draw.io draws. Attached ends sit on the node
// outline at the fixed port given by exitX/exitY or entryX/entryY, or where the
// line toward the next point leaves the node. An orthogonal wire without
// waypoints between offset nodes gets the elbows draw.io would add.
func (v *View) path(w *Wire) geom.Polyline {
	pts := w.Cell.Points()
	srcPt, srcOK := w.Cell.TerminalPoint(true)
	dstPt, dstOK := w.Cell.TerminalPoint(false)
	if w.Src == nil && w.SrcRef != "" {
		if c := v.Page.Cell(w.SrcRef); c != nil && c.IsVertex() {
			srcPt, srcOK = c.AbsBounds().Center(), true
		}
	}
	if w.Dst == nil && w.DstRef != "" {
		if c := v.Page.Cell(w.DstRef); c != nil && c.IsVertex() {
			dstPt, dstOK = c.AbsBounds().Center(), true
		}
	}
	// Aim each attached end at its neighbor point: the first or last waypoint,
	// else the other end.
	aimSrc, aimDst := dstPt, srcPt
	if w.Dst != nil {
		aimSrc = w.Dst.Box.Center()
	}
	if w.Src != nil {
		aimDst = w.Src.Box.Center()
	}
	if len(pts) > 0 {
		aimSrc, aimDst = pts[0], pts[len(pts)-1]
	}
	ortho := w.Style.Value("edgeStyle", "") == "orthogonalEdgeStyle" || w.Style.Value("edgeStyle", "") == "elbowEdgeStyle"
	if w.Src != nil {
		srcPt, srcOK = port(w.Src.Box, w.Style, "exit", aimSrc, ortho), true
	}
	if w.Dst != nil {
		dstPt, dstOK = port(w.Dst.Box, w.Style, "entry", aimDst, ortho), true
	}
	if !srcOK || !dstOK {
		if !srcOK && len(pts) > 0 {
			srcPt, srcOK = pts[0], true
		}
		if !dstOK && len(pts) > 0 {
			dstPt, dstOK = pts[len(pts)-1], true
		}
		if !srcOK || !dstOK {
			return nil
		}
	}
	out := geom.Polyline{srcPt}
	out = append(out, pts...)
	out = append(out, dstPt)
	if ortho {
		out = orthogonalize(out)
	}
	return out
}

// port returns where a wire meets a node box.
func port(box geom.Rect, st doc.Style, prefix string, aim geom.Point, ortho bool) geom.Point {
	if xs, ok := st.Get(prefix + "X"); ok {
		if ys, ok := st.Get(prefix + "Y"); ok {
			dx := geom.ParseNumber(st.Value(prefix+"Dx", "0"))
			dy := geom.ParseNumber(st.Value(prefix+"Dy", "0"))
			return geom.Point{X: box.X + geom.ParseNumber(xs)*box.W + dx, Y: box.Y + geom.ParseNumber(ys)*box.H + dy}
		}
	}
	c := box.Center()
	if ortho {
		// draw.io leaves an orthogonal wire from the side facing the next point,
		// straight when the point is level with the box.
		switch {
		case aim.Y >= box.Y && aim.Y <= box.Bottom() && aim.X > box.Right():
			return geom.Point{X: box.Right(), Y: aim.Y}
		case aim.Y >= box.Y && aim.Y <= box.Bottom() && aim.X < box.X:
			return geom.Point{X: box.X, Y: aim.Y}
		case aim.X >= box.X && aim.X <= box.Right() && aim.Y > box.Bottom():
			return geom.Point{X: aim.X, Y: box.Bottom()}
		case aim.X >= box.X && aim.X <= box.Right() && aim.Y < box.Y:
			return geom.Point{X: aim.X, Y: box.Y}
		}
		dx, dy := aim.X-c.X, aim.Y-c.Y
		if math.Abs(dx)*box.H > math.Abs(dy)*box.W {
			if dx > 0 {
				return geom.Point{X: box.Right(), Y: c.Y}
			}
			return geom.Point{X: box.X, Y: c.Y}
		}
		if dy > 0 {
			return geom.Point{X: c.X, Y: box.Bottom()}
		}
		return geom.Point{X: c.X, Y: box.Y}
	}
	return clipToBox(box, c, aim)
}

// clipToBox returns where the line from the center toward aim leaves the box.
func clipToBox(box geom.Rect, c, aim geom.Point) geom.Point {
	dx, dy := aim.X-c.X, aim.Y-c.Y
	if dx == 0 && dy == 0 {
		return c
	}
	tx, ty := math.Inf(1), math.Inf(1)
	if dx != 0 {
		tx = (box.W / 2) / math.Abs(dx)
	}
	if dy != 0 {
		ty = (box.H / 2) / math.Abs(dy)
	}
	t := math.Min(tx, ty)
	return geom.Point{X: c.X + dx*t, Y: c.Y + dy*t}
}

// orthogonalize inserts an elbow between consecutive points that are not on a
// common horizontal or vertical line, the way draw.io bends orthogonal wires.
func orthogonalize(pl geom.Polyline) geom.Polyline {
	bent := func(a, b geom.Point) bool { return math.Abs(a.X-b.X) > 0.5 && math.Abs(a.Y-b.Y) > 0.5 }
	if len(pl) == 2 {
		a, b := pl[0], pl[1]
		if !bent(a, b) {
			return pl
		}
		if math.Abs(b.X-a.X) >= math.Abs(b.Y-a.Y) {
			mx := (a.X + b.X) / 2
			return geom.Polyline{a, {X: mx, Y: a.Y}, {X: mx, Y: b.Y}, b}
		}
		my := (a.Y + b.Y) / 2
		return geom.Polyline{a, {X: a.X, Y: my}, {X: b.X, Y: my}, b}
	}
	out := geom.Polyline{pl[0]}
	for i := 1; i < len(pl); i++ {
		a, b := out[len(out)-1], pl[i]
		if bent(a, b) {
			out = append(out, geom.Point{X: a.X, Y: b.Y})
		}
		out = append(out, b)
	}
	return out
}

var (
	breakTags = regexp.MustCompile(`(?i)<\s*(br|/div|/p|/li|/tr|/h[1-6])\s*/?>`)
	anyTag    = regexp.MustCompile(`<[^>]*>`)
)

// PlainText turns a label into plain lines: HTML tags removed, line breaks kept,
// entities decoded.
func PlainText(label string, isHTML bool) string {
	if !isHTML {
		return strings.TrimSpace(label)
	}
	s := breakTags.ReplaceAllString(label, "\n")
	s = anyTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// Leaves returns the nodes that are not containers, in document order.
func (v *View) Leaves() []*Node {
	var out []*Node
	for _, n := range v.Nodes {
		if !n.Container {
			out = append(out, n)
		}
	}
	return out
}

// Containers returns the container nodes, innermost first.
func (v *View) Containers() []*Node {
	var out []*Node
	for _, n := range v.Nodes {
		if n.Container {
			out = append(out, n)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return depth(out[i]) > depth(out[j]) })
	return out
}

func depth(n *Node) int {
	d := 0
	for p := n.Parent; p != nil && d < 64; p = p.Parent {
		d++
	}
	return d
}
