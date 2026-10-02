// Package messify damages clean diagrams in controlled, seeded ways and records
// what it did, so tests can compare diakempt's repair with the known original.
package messify

import (
	"math"
	"math/rand"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/view"
)

// Detached is the ground truth for one wire end that was cut loose.
type Detached struct {
	Page   int
	Wire   string
	Source bool
	// Node is the cell the end was attached to.
	Node string
	// Offset is how far the loose end was moved off the outline: negative is
	// inside the shape, positive outside.
	Offset float64
}

// DetachOptions controls Detach.
type DetachOptions struct {
	// Share is the probability that a given attached end is cut loose.
	Share float64
	// MaxInside and MaxOutside bound how far the loose end moves, in pixels.
	MaxInside, MaxOutside float64
}

// Detach cuts attached wire ends loose, leaving each end near where the wire met
// its shape, as when a user releases the mouse slightly off target.
func Detach(d *doc.Document, rng *rand.Rand, o DetachOptions) []Detached {
	var out []Detached
	for _, p := range d.Pages {
		v := view.Build(p)
		for _, w := range v.Wires {
			for _, source := range []bool{true, false} {
				n := w.Dst
				if source {
					n = w.Src
				}
				if n == nil || rng.Float64() >= o.Share {
					continue
				}
				end, ok := w.End(source)
				if !ok {
					continue
				}
				off := -o.MaxInside + rng.Float64()*(o.MaxInside+o.MaxOutside)
				pt := push(n.Box, end, off)
				attr := "target"
				if source {
					attr = "source"
				}
				w.Cell.Node.Del(attr)
				w.Cell.SetTerminalPoint(source, pt)
				out = append(out, Detached{Page: p.Index, Wire: w.Cell.ID, Source: source, Node: n.Cells[0].ID, Offset: off})
			}
		}
	}
	return out
}

// push moves a point on a box outline along the outward normal of its nearest
// side. A negative distance moves it inside, never past the center.
func push(b geom.Rect, pt geom.Point, dist float64) geom.Point {
	dl, dr := math.Abs(pt.X-b.X), math.Abs(pt.X-b.Right())
	dt, db := math.Abs(pt.Y-b.Y), math.Abs(pt.Y-b.Bottom())
	m := math.Min(math.Min(dl, dr), math.Min(dt, db))
	if dist < 0 {
		lim := math.Min(b.W, b.H)/2 - 1
		dist = math.Max(dist, -lim)
	}
	switch m {
	case dl:
		return geom.Point{X: b.X - dist, Y: pt.Y}
	case dr:
		return geom.Point{X: b.Right() + dist, Y: pt.Y}
	case dt:
		return geom.Point{X: pt.X, Y: b.Y - dist}
	}
	return geom.Point{X: pt.X, Y: b.Bottom() + dist}
}
