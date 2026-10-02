// Package geom holds the plane geometry shared by every step: points,
// rectangles, segments and polylines, in draw.io page coordinates (x grows to the
// right, y grows downward).
package geom

import (
	"math"
	"strconv"
)

// Point is a position on the page.
type Point struct{ X, Y float64 }

// Add returns p + q.
func (p Point) Add(q Point) Point { return Point{p.X + q.X, p.Y + q.Y} }

// Sub returns p - q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// Dist returns the distance between p and q.
func (p Point) Dist(q Point) float64 { return math.Hypot(p.X-q.X, p.Y-q.Y) }

// Rect is an axis-aligned rectangle given by its top-left corner and size.
type Rect struct{ X, Y, W, H float64 }

// Right returns the x of the right side.
func (r Rect) Right() float64 { return r.X + r.W }

// Bottom returns the y of the bottom side.
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Center returns the center point.
func (r Rect) Center() Point { return Point{r.X + r.W/2, r.Y + r.H/2} }

// Empty reports whether the rectangle has no area.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Move returns r shifted by d.
func (r Rect) Move(d Point) Rect { return Rect{r.X + d.X, r.Y + d.Y, r.W, r.H} }

// Inset returns r shrunk by d on every side; a negative d grows it.
func (r Rect) Inset(d float64) Rect { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }

// Contains reports whether p lies inside r or on its outline.
func (r Rect) Contains(p Point) bool {
	return p.X >= r.X && p.X <= r.Right() && p.Y >= r.Y && p.Y <= r.Bottom()
}

// ContainsRect reports whether o lies entirely inside r.
func (r Rect) ContainsRect(o Rect) bool {
	return o.X >= r.X && o.Y >= r.Y && o.Right() <= r.Right() && o.Bottom() <= r.Bottom()
}

// Overlaps reports whether r and o share area; touching sides do not count.
func (r Rect) Overlaps(o Rect) bool {
	return r.X < o.Right() && o.X < r.Right() && r.Y < o.Bottom() && o.Y < r.Bottom()
}

// Intersect returns the common area, empty when there is none.
func (r Rect) Intersect(o Rect) Rect {
	x0, y0 := math.Max(r.X, o.X), math.Max(r.Y, o.Y)
	x1, y1 := math.Min(r.Right(), o.Right()), math.Min(r.Bottom(), o.Bottom())
	if x1 <= x0 || y1 <= y0 {
		return Rect{}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Union returns the smallest rectangle holding both. An empty rectangle is
// ignored.
func (r Rect) Union(o Rect) Rect {
	if r.W == 0 && r.H == 0 {
		return o
	}
	if o.W == 0 && o.H == 0 {
		return r
	}
	x0, y0 := math.Min(r.X, o.X), math.Min(r.Y, o.Y)
	x1, y1 := math.Max(r.Right(), o.Right()), math.Max(r.Bottom(), o.Bottom())
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// DistToOutline returns the distance from p to the outline of r: zero on the
// outline, positive both inside and outside.
func (r Rect) DistToOutline(p Point) float64 {
	if r.Contains(p) {
		return math.Min(math.Min(p.X-r.X, r.Right()-p.X), math.Min(p.Y-r.Y, r.Bottom()-p.Y))
	}
	dx := math.Max(math.Max(r.X-p.X, 0), p.X-r.Right())
	dy := math.Max(math.Max(r.Y-p.Y, 0), p.Y-r.Bottom())
	return math.Hypot(dx, dy)
}

// Dist returns the distance from p to r: zero inside or on the outline.
func (r Rect) Dist(p Point) float64 {
	if r.Contains(p) {
		return 0
	}
	return r.DistToOutline(p)
}

// Segment is a straight piece of a wire.
type Segment struct{ A, B Point }

// Len returns the segment length.
func (s Segment) Len() float64 { return s.A.Dist(s.B) }

// CrossesRect reports whether the segment passes through the inside of r, more
// than margin away from its outline. A segment that only touches or runs along
// the outline does not cross.
func (s Segment) CrossesRect(r Rect, margin float64) bool {
	in := r.Inset(margin)
	if in.Empty() {
		return false
	}
	// Liang-Barsky clipping against the shrunk rectangle.
	t0, t1 := 0.0, 1.0
	dx, dy := s.B.X-s.A.X, s.B.Y-s.A.Y
	clip := func(p, q float64) bool {
		if p == 0 {
			return q > 0
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return false
			}
			if t > t0 {
				t0 = t
			}
		} else {
			if t < t0 {
				return false
			}
			if t < t1 {
				t1 = t
			}
		}
		return true
	}
	if !clip(-dx, s.A.X-in.X) || !clip(dx, in.Right()-s.A.X) ||
		!clip(-dy, s.A.Y-in.Y) || !clip(dy, in.Bottom()-s.A.Y) {
		return false
	}
	return t1-t0 > 1e-9
}

// Intersects reports whether two segments cross at a single interior point.
// Shared endpoints and collinear overlaps do not count as a crossing.
func (s Segment) Intersects(o Segment) bool {
	d1 := cross(o.A, o.B, s.A)
	d2 := cross(o.A, o.B, s.B)
	d3 := cross(s.A, s.B, o.A)
	d4 := cross(s.A, s.B, o.B)
	const eps = 1e-9
	return ((d1 > eps && d2 < -eps) || (d1 < -eps && d2 > eps)) &&
		((d3 > eps && d4 < -eps) || (d3 < -eps && d4 > eps))
}

// Overlaps reports whether two axis-parallel segments lie on the same line and
// share more than tol of length.
func (s Segment) Overlaps(o Segment, tol float64) bool {
	const eps = 0.5
	if math.Abs(s.A.Y-s.B.Y) < eps && math.Abs(o.A.Y-o.B.Y) < eps && math.Abs(s.A.Y-o.A.Y) < eps {
		lo := math.Max(math.Min(s.A.X, s.B.X), math.Min(o.A.X, o.B.X))
		hi := math.Min(math.Max(s.A.X, s.B.X), math.Max(o.A.X, o.B.X))
		return hi-lo > tol
	}
	if math.Abs(s.A.X-s.B.X) < eps && math.Abs(o.A.X-o.B.X) < eps && math.Abs(s.A.X-o.A.X) < eps {
		lo := math.Max(math.Min(s.A.Y, s.B.Y), math.Min(o.A.Y, o.B.Y))
		hi := math.Min(math.Max(s.A.Y, s.B.Y), math.Max(o.A.Y, o.B.Y))
		return hi-lo > tol
	}
	return false
}

func cross(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

// Polyline is a wire path from its first point to its last.
type Polyline []Point

// Segments returns the straight pieces, skipping zero-length ones.
func (pl Polyline) Segments() []Segment {
	var out []Segment
	for i := 1; i < len(pl); i++ {
		if pl[i] != pl[i-1] {
			out = append(out, Segment{pl[i-1], pl[i]})
		}
	}
	return out
}

// Len returns the total length.
func (pl Polyline) Len() float64 {
	n := 0.0
	for _, s := range pl.Segments() {
		n += s.Len()
	}
	return n
}

// Bounds returns the bounding box of the points.
func (pl Polyline) Bounds() Rect {
	if len(pl) == 0 {
		return Rect{}
	}
	x0, y0, x1, y1 := pl[0].X, pl[0].Y, pl[0].X, pl[0].Y
	for _, p := range pl[1:] {
		x0, y0 = math.Min(x0, p.X), math.Min(y0, p.Y)
		x1, y1 = math.Max(x1, p.X), math.Max(y1, p.Y)
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

// Format writes a coordinate the way diakempt stores it: rounded to two
// decimals, without trailing zeros.
func Format(v float64) string {
	r := math.Round(v*100) / 100
	if r == 0 {
		r = 0 // drop the sign of -0
	}
	return strconv.FormatFloat(r, 'f', -1, 64)
}

// ParseNumber reads a number attribute; missing or unreadable means 0.
func ParseNumber(s string) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return v
}
