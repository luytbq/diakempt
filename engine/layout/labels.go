package layout

import (
	"math"
	"strings"
	"time"

	"github.com/luytbq/diakempt/engine/num"
)

type box = [4]float64 // x1, y1, x2, y2

func boxOf(it *Item) box { return box{it.X, it.Y, it.X + it.W, it.Y + it.H} }

// area is the area of the overlap between two boxes.
func area(a, b box) float64 {
	w := fmin(a[2], b[2]) - fmax(a[0], b[0])
	h := fmin(a[3], b[3]) - fmax(a[1], b[1])
	if w > 0 && h > 0 {
		return float64(w * h)
	}
	return 0
}

// segHits reports whether the segment from a to b cuts into the box. Merely
// touching the border does not count.
func segHits(a, b [2]float64, bx box) bool {
	x1, x2 := fmin(a[0], b[0]), fmax(a[0], b[0])
	y1, y2 := fmin(a[1], b[1]), fmax(a[1], b[1])
	return x1 < bx[2] && x2 > bx[0] && y1 < bx[3] && y2 > bx[1]
}

func pathLen(pts [][2]float64) float64 {
	total := 0.0
	for i := 1; i < len(pts); i++ {
		total += math.Abs(pts[i][0]-pts[i-1][0]) + math.Abs(pts[i][1]-pts[i-1][1])
	}
	return total
}

type candidate struct {
	b      box
	dist   float64    // distance along the path from the source to the anchor
	anchor [2]float64 // point on the path the label attaches to
}

// labelCandidates lists the possible label positions in order of preference:
// nearest the source first, and on each long enough segment try right at the
// start of the segment and then its middle, each on both sides of the path.
func labelCandidates(e *Edge) []candidate {
	w, h := e.LW, e.LH
	acc := 0.0
	var out []candidate
	for i := 1; i < len(e.Pts); i++ {
		a, b := e.Pts[i-1], e.Pts[i]
		L := math.Abs(b[0]-a[0]) + math.Abs(b[1]-a[1])
		if L >= 12 {
			if math.Abs(a[1]-b[1]) < 0.01 {
				d := -1.0
				if b[0] > a[0] {
					d = 1
				}
				for _, cx := range []float64{a[0] + float64(d*(6+w/2)), (a[0] + b[0]) / 2} {
					ax := fmin(fmax(cx, fmin(a[0], b[0])), fmax(a[0], b[0]))
					for _, cy := range []float64{a[1] - 3 - h/2, a[1] + 3 + h/2} {
						out = append(out, candidate{
							box{cx - w/2, cy - h/2, cx + w/2, cy + h/2},
							acc + math.Abs(ax-a[0]), [2]float64{ax, a[1]},
						})
					}
				}
			} else {
				d := -1.0
				if b[1] > a[1] {
					d = 1
				}
				for _, cy := range []float64{a[1] + float64(d*(6+h/2)), (a[1] + b[1]) / 2} {
					ay := fmin(fmax(cy, fmin(a[1], b[1])), fmax(a[1], b[1]))
					for _, cx := range []float64{a[0] + 5 + w/2, a[0] - 5 - w/2} {
						out = append(out, candidate{
							box{cx - w/2, cy - h/2, cx + w/2, cy + h/2},
							acc + math.Abs(ay-a[1]), [2]float64{a[0], ay},
						})
					}
				}
			}
		}
		acc += L
	}
	return out
}

// placeLabels places each edge's label at the candidate with the lowest cost.
//
// The cost adds up the area overlapping nodes, already placed labels and the
// header, plus a fixed amount for each other wire crossing it. The first
// candidate with zero cost is accepted immediately. The order of additions is
// part of the result, because these sums are compared with each other and
// floating-point addition is not associative.
func (l *Layout) placeLabels() {
	type named struct {
		id string
		b  box
	}
	var boxes []named
	for _, it := range l.ItemOrder {
		boxes = append(boxes, named{it.ID, boxOf(it)})
	}
	boxes = append(boxes, named{"header", box{-1e6, -1e6, 1e6, float64(l.Cfg.PoolHeader + l.Cfg.LaneHeader)}})

	type wire struct {
		id   string // empty for lane separator lines
		a, b [2]float64
	}
	var wires []wire
	for _, x := range l.LaneX[1:] {
		wires = append(wires, wire{"", [2]float64{x, 0}, [2]float64{x, l.PoolH}})
	}
	for _, e := range l.Edges {
		for i := 1; i < len(e.Pts); i++ {
			wires = append(wires, wire{e.ID, e.Pts[i-1], e.Pts[i]})
		}
	}

	var placed []box
	for _, e := range l.Edges {
		if len(e.Lines) == 0 {
			continue
		}
		var best *candidate
		bestCost := 0.0
		for _, c := range labelCandidates(e) {
			cost := 0.0
			for _, nb := range boxes {
				cost += float64(area(c.b, nb.b) * 4)
			}
			for _, pb := range placed {
				cost += float64(area(c.b, pb) * 4)
			}
			for _, w := range wires {
				if w.id != e.ID && segHits(w.a, w.b, c.b) {
					cost += 100
				}
			}
			if best == nil || cost < bestCost {
				cc := c
				best, bestCost = &cc, cost
			}
			if cost == 0 {
				break
			}
		}
		if best == nil {
			l.warn("layout.label-no-room", e.ID, "%s: path too short, no room for the label", e.ID)
			continue
		}
		if bestCost > 0 {
			l.warn("layout.label-crowded", e.ID, "%s: label \"%s\" could not find a completely free spot",
				e.ID, strings.Join(e.Lines, " "))
		}
		placed = append(placed, best.b)
		b := best.b
		e.Label = &b
		if total := pathLen(e.Pts); total != 0 {
			e.LabelT = num.Round(float64(2*best.dist)/total-1, 4)
		} else {
			e.LabelT = 0
		}
		cx, cy := (b[0]+b[2])/2, (b[1]+b[3])/2
		e.LabelOff = [2]float64{num.Round(cx-best.anchor[0], 2), num.Round(cy-best.anchor[1], 2)}
	}
}

// run runs the grid phases in the proper order: place, route, geometry twice,
// labels.
//
// Geometry is computed in two passes. The first pass does not yet know how long
// the horizontal part of B and C edges is, and so whether their labels fit; the
// second pass reserves extra space for the labels that did not fit and
// recomputes.
func (l *Layout) run() *Layout {
	t0 := time.Now()
	l.Place()
	l.tracePlace(t0)
	t0 = time.Now()
	l.Route()
	l.traceRoute(t0)
	t0 = time.Now()
	lzG, lzC := l.labelReservations(nil)
	l.computeGeometry(lzG, lzC)
	l.resolvePaths()
	l.traceGeometry(1, len(lzG)+len(lzC), t0)
	t0 = time.Now()
	lzG, lzC = l.labelReservations(l.horizontalSpans())
	l.computeGeometry(lzG, lzC)
	l.resolvePaths()
	l.traceGeometry(2, len(lzG)+len(lzC), t0)
	t0 = time.Now()
	before := len(l.Warnings)
	l.placeLabels()
	l.traceLabels(before, t0)
	return l
}
