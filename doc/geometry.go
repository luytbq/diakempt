package doc

import (
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/internal/xmltree"
)

// draw.io stores a vertex position relative to its parent when the parent is a
// vertex (a container or a group), and absolute when the parent is a layer. Wire
// points are relative to the wire's parent in the same way. Every method here
// that says "absolute" converts through the chain of parents.

// maxDepth bounds parent chains, so a file with a parent cycle cannot hang.
const maxDepth = 64

// Origin returns the absolute position that coordinates of c's children are
// relative to.
func (c *Cell) Origin() geom.Point {
	var o geom.Point
	cur := c
	for i := 0; cur != nil && i < maxDepth; i++ {
		if cur.IsEdge() {
			// Children of a wire are its labels, placed along the wire rather than
			// by coordinates; their origin is the wire's own coordinate system.
			cur = cur.Page.Cell(cur.Parent())
			continue
		}
		if !cur.IsVertex() || cur.RelativeGeometry() {
			break
		}
		b := cur.Bounds()
		o = o.Add(geom.Point{X: b.X, Y: b.Y})
		cur = cur.Page.Cell(cur.Parent())
	}
	return o
}

// parentOrigin returns the origin c's own coordinates are relative to.
func (c *Cell) parentOrigin() geom.Point {
	if par := c.Page.Cell(c.Parent()); par != nil {
		return par.Origin()
	}
	return geom.Point{}
}

// Bounds returns the geometry as stored, relative to the parent.
func (c *Cell) Bounds() geom.Rect {
	g := c.geometry(false)
	if g == nil {
		return geom.Rect{}
	}
	return geom.Rect{
		X: geom.ParseNumber(g.Attr("x")), Y: geom.ParseNumber(g.Attr("y")),
		W: geom.ParseNumber(g.Attr("width")), H: geom.ParseNumber(g.Attr("height")),
	}
}

// AbsBounds returns the geometry in page coordinates.
func (c *Cell) AbsBounds() geom.Rect {
	return c.Bounds().Move(c.parentOrigin())
}

// SetBounds writes the geometry, relative to the parent. Coordinates that are
// zero and were absent stay absent, as draw.io writes them.
func (c *Cell) SetBounds(r geom.Rect) {
	g := c.geometry(true)
	setNum(g, "x", r.X)
	setNum(g, "y", r.Y)
	setNum(g, "width", r.W)
	setNum(g, "height", r.H)
}

// SetAbsBounds writes the geometry from page coordinates.
func (c *Cell) SetAbsBounds(r geom.Rect) {
	o := c.parentOrigin()
	c.SetBounds(r.Move(geom.Point{X: -o.X, Y: -o.Y}))
}

func setNum(g *xmltree.Element, k string, v float64) {
	if _, ok := g.Get(k); !ok && geom.Format(v) == "0" {
		return
	}
	g.Set(k, geom.Format(v))
}

// Points returns a wire's waypoints in page coordinates.
func (c *Cell) Points() []geom.Point {
	g := c.geometry(false)
	if g == nil {
		return nil
	}
	o := c.parentOrigin()
	for _, arr := range g.FindAll("Array") {
		if arr.Attr("as") != "points" {
			continue
		}
		var out []geom.Point
		for _, p := range arr.FindAll("mxPoint") {
			out = append(out, geom.Point{X: geom.ParseNumber(p.Attr("x")), Y: geom.ParseNumber(p.Attr("y"))}.Add(o))
		}
		return out
	}
	return nil
}

// SetPoints replaces a wire's waypoints, given in page coordinates. No points
// removes the list.
func (c *Cell) SetPoints(pts []geom.Point) {
	g := c.geometry(true)
	var arr *xmltree.Element
	for _, a := range g.FindAll("Array") {
		if a.Attr("as") == "points" {
			arr = a
		}
	}
	if len(pts) == 0 {
		if arr != nil {
			g.Remove(arr)
		}
		return
	}
	if arr == nil {
		arr = g.Add("Array", "as", "points")
	}
	arr.Children = nil
	arr.Text = ""
	o := c.parentOrigin()
	for _, p := range pts {
		arr.Add("mxPoint", "x", geom.Format(p.X-o.X), "y", geom.Format(p.Y-o.Y))
	}
}

// TerminalPoint returns the stored position of a wire end in page coordinates.
// draw.io keeps it for free ends; for attached ends it is often stale or absent.
func (c *Cell) TerminalPoint(source bool) (geom.Point, bool) {
	g := c.geometry(false)
	if g == nil {
		return geom.Point{}, false
	}
	as := "targetPoint"
	if source {
		as = "sourcePoint"
	}
	for _, p := range g.FindAll("mxPoint") {
		if p.Attr("as") == as {
			pt := geom.Point{X: geom.ParseNumber(p.Attr("x")), Y: geom.ParseNumber(p.Attr("y"))}
			return pt.Add(c.parentOrigin()), true
		}
	}
	return geom.Point{}, false
}

// SetTerminalPoint writes the stored position of a wire end, in page
// coordinates.
func (c *Cell) SetTerminalPoint(source bool, pt geom.Point) {
	g := c.geometry(true)
	as := "targetPoint"
	if source {
		as = "sourcePoint"
	}
	o := c.parentOrigin()
	for _, p := range g.FindAll("mxPoint") {
		if p.Attr("as") == as {
			p.Set("x", geom.Format(pt.X-o.X))
			p.Set("y", geom.Format(pt.Y-o.Y))
			return
		}
	}
	g.Add("mxPoint", "x", geom.Format(pt.X-o.X), "y", geom.Format(pt.Y-o.Y), "as", as)
}

// LabelPosition returns where a wire's own label sits: x is the relative
// position along the wire from -1 (source) to 1 (target), y the distance from
// the wire, and offset an extra shift in pixels.
func (c *Cell) LabelPosition() (x, y float64, offset geom.Point) {
	g := c.geometry(false)
	if g == nil {
		return 0, 0, geom.Point{}
	}
	for _, p := range g.FindAll("mxPoint") {
		if p.Attr("as") == "offset" {
			offset = geom.Point{X: geom.ParseNumber(p.Attr("x")), Y: geom.ParseNumber(p.Attr("y"))}
		}
	}
	return geom.ParseNumber(g.Attr("x")), geom.ParseNumber(g.Attr("y")), offset
}

// SetLabelPosition writes where a wire's own label sits; see LabelPosition.
func (c *Cell) SetLabelPosition(x, y float64, offset geom.Point) {
	g := c.geometry(true)
	setNum(g, "x", x)
	setNum(g, "y", y)
	for _, p := range g.FindAll("mxPoint") {
		if p.Attr("as") == "offset" {
			if offset == (geom.Point{}) {
				g.Remove(p)
				return
			}
			p.Set("x", geom.Format(offset.X))
			p.Set("y", geom.Format(offset.Y))
			return
		}
	}
	if offset != (geom.Point{}) {
		g.Add("mxPoint", "x", geom.Format(offset.X), "y", geom.Format(offset.Y), "as", "offset")
	}
}
