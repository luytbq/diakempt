package tidy

import (
	"math"

	"github.com/luytbq/diakempt/geom"
)

// Shift is how far a node's corner has moved from where it was drawn.
func (n *Node) Shift() geom.Point {
	return geom.Point{X: n.Box.X - n.Orig.X, Y: n.Box.Y - n.Orig.Y}
}

// Nearest returns the node of among that n was drawn nearest to, by the gap
// between their original boxes. Ties go to the earlier node, so the answer
// does not depend on map order. It returns nil when among is empty.
func Nearest(n *Node, among []*Node) *Node {
	var best *Node
	bd := math.Inf(1)
	for _, m := range among {
		if d := n.Orig.Gap(m.Orig); d < bd {
			best, bd = m, d
		}
	}
	return best
}

// Follow moves each node of free, which a layout did not place, along with
// the placed node it belongs to: the placed container it sits in, or else
// the placed node it was drawn nearest to. Notes and texts so keep their
// place beside what they explain.
func (d *Diagram) Follow(free, placed []*Node) {
	in := map[*Node]bool{}
	for _, m := range placed {
		in[m] = true
	}
	for _, n := range free {
		var host *Node
		for p := n.Parent; p != nil && host == nil; p = p.Parent {
			if in[p] {
				host = p
			}
		}
		if host == nil {
			host = Nearest(n, placed)
		}
		if host != nil {
			d.Move(n, host.Shift().Sub(n.Shift()))
		}
	}
}

// KeepCorner moves the whole working copy, waypoints and free wire ends
// included, so the diagram's top-left corner is where it was drawn. It
// returns how far it moved.
func (d *Diagram) KeepCorner() geom.Point {
	var was, now geom.Rect
	for _, n := range d.Nodes {
		was = was.Union(n.Orig)
		now = now.Union(n.Box)
	}
	by := geom.Point{X: was.X - now.X, Y: was.Y - now.Y}
	if by == (geom.Point{}) {
		return by
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
	return by
}
