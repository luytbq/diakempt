package view

import (
	"math"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
)

// LifelineReach is how far, in pixels, a free message end may be from a
// lifeline's center line and still be read as on it.
const LifelineReach = 6

// IsLifeline reports whether a node is the editor's UML lifeline shape: a
// head with a dashed line below it.
func IsLifeline(n *Node) bool { return n != nil && n.Shape == "umlLifeline" }

// LifelineOf returns the UML lifeline a node is or sits on, such as an
// activation bar, or nil.
func LifelineOf(n *Node) *Node {
	for k := 0; n != nil && k < 64; k++ {
		if IsLifeline(n) {
			return n
		}
		n = n.Parent
	}
	return nil
}

// LifelineHead is the height of a UML lifeline's head.
func LifelineHead(st doc.Style) float64 {
	return geom.ParseNumber(st.Value("size", "40"))
}

// LifelineLine is the dashed center line of a UML lifeline drawn in box:
// from below its head to its bottom.
func LifelineLine(st doc.Style, box geom.Rect) geom.Segment {
	c := box.Center().X
	return geom.Segment{A: geom.Point{X: c, Y: box.Y + LifelineHead(st)}, B: geom.Point{X: c, Y: box.Bottom()}}
}

// OnLifeline reports whether a point lies on the center line of a UML
// lifeline: within LifelineReach of its center x, inside its height.
// Messages drawn with free ends between lifelines end there.
func OnLifeline(n *Node, pt geom.Point) bool {
	return IsLifeline(n) && math.Abs(pt.X-n.Box.Center().X) <= LifelineReach &&
		pt.Y >= n.Box.Y && pt.Y <= n.Box.Bottom()
}
