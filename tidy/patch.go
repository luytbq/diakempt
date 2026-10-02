package tidy

import (
	"math"
	"sort"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
)

// Patch writes the working geometry into the document. Only geometry changes:
// positions, sizes, waypoints, free end positions, wire anchors of rerouted
// wires and label positions.
func (d *Diagram) Patch() {
	page := d.Seg.Nodes[0].Cells[0].Page
	// Every target is computed before anything is written, because writing a
	// container changes what its children's stored coordinates mean.
	targets := map[*doc.Cell]geom.Rect{}
	for _, n := range d.Nodes {
		targets[n.V.Cells[0]] = n.Box
		hostShift := geom.Point{X: n.Box.Center().X - n.Orig.Center().X, Y: n.Box.Center().Y - n.Orig.Center().Y}
		for _, m := range n.V.Cells[1:] {
			targets[m] = m.AbsBounds().Move(hostShift)
		}
	}
	// Cells in a convenience group are stored relative to the group, so the
	// group is refitted around its children first.
	for _, c := range page.Cells {
		if !isGroupCell(c) {
			continue
		}
		var box geom.Rect
		any := false
		for _, ch := range page.Children(c.ID) {
			if t, ok := targets[ch]; ok {
				box = box.Union(t)
				any = true
			} else if ch.IsVertex() && !ch.RelativeGeometry() {
				box = box.Union(ch.AbsBounds())
			}
		}
		if any && box != c.AbsBounds() {
			targets[c] = box
			for _, ch := range page.Children(c.ID) {
				if _, ok := targets[ch]; !ok && ch.IsVertex() && !ch.RelativeGeometry() {
					targets[ch] = ch.AbsBounds()
				}
			}
		}
	}
	// Parents are written before their children, whatever the document order.
	var cells []*doc.Cell
	for _, c := range page.Cells {
		if _, ok := targets[c]; ok {
			cells = append(cells, c)
		}
	}
	sort.SliceStable(cells, func(i, j int) bool { return cellDepth(cells[i]) < cellDepth(cells[j]) })
	for _, c := range cells {
		if t := targets[c]; !near(c.AbsBounds(), t) {
			c.SetAbsBounds(t)
		}
	}
	for _, n := range d.Nodes {
		if len(n.SetStyle) == 0 {
			continue
		}
		c := n.V.Cells[0]
		st := c.Style()
		keys := make([]string, 0, len(n.SetStyle))
		for k := range n.SetStyle {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			st.Set(k, n.SetStyle[k])
		}
		if st.String() != c.Style().String() {
			c.SetStyle(st)
		}
	}
	// Wire points are stored relative to the wire's parent, which may have moved
	// even when the wire did not, so they are compared in page coordinates
	// after the shapes are written.
	for _, w := range d.Wires {
		c := w.V.Cell
		if !nearPoints(c.Points(), w.Points) {
			c.SetPoints(w.Points)
		}
		if w.SrcPoint != nil {
			if p, ok := c.TerminalPoint(true); !ok || p.Dist(*w.SrcPoint) > 0.005 {
				c.SetTerminalPoint(true, *w.SrcPoint)
			}
		}
		if w.DstPoint != nil {
			if p, ok := c.TerminalPoint(false); !ok || p.Dist(*w.DstPoint) > 0.005 {
				c.SetTerminalPoint(false, *w.DstPoint)
			}
		}
		if !w.changed() {
			continue
		}
		if s := w.Style.String(); s != w.origStyle {
			c.SetStyle(w.Style)
		}
		for i, l := range w.labels {
			if l == w.origLabels[i] {
				continue
			}
			target := c
			if l.cell != nil {
				target = l.cell
			}
			_, y, _ := target.LabelPosition()
			target.SetLabelPosition(l.pos, y, l.offset)
		}
	}
}

func nearPoints(a, b []geom.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Dist(b[i]) > 0.005 {
			return false
		}
	}
	return true
}

func cellDepth(c *doc.Cell) int {
	k := 0
	for p := c.Page.Cell(c.Parent()); p != nil && k < 64; p = p.Page.Cell(p.Parent()) {
		k++
	}
	return k
}

func isGroupCell(c *doc.Cell) bool {
	if !c.IsVertex() {
		return false
	}
	st := c.Style()
	return st.Shape() == "group"
}

// near compares boxes at a tolerance above the 0.01px rounding of stored
// coordinates, so sizes summed from rounded parts still compare equal.
func near(a, b geom.Rect) bool {
	const eps = 0.02
	return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps && math.Abs(a.W-b.W) < eps && math.Abs(a.H-b.H) < eps
}
