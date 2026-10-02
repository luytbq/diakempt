package layout

import (
	"fmt"
	"math"
	"sort"
)

// Finding is a finding of the geometry self-check. Code is a stable machine
// code for callers to classify by; Msg is the human-readable message.
type Finding struct {
	Level string // "error" or "warning"
	Code  string
	Msg   string
}

func shrink(b box, d float64) box { return box{b[0] + d, b[1] + d, b[2] - d, b[3] - d} }

// collinearOverlap is the length over which two segments overlap on the same
// horizontal or vertical line. Two segments less than half a pixel apart count
// as the same line, because once drawn the eye cannot tell them apart.
func collinearOverlap(a1, b1, a2, b2 [2]float64) float64 {
	if math.Abs(a1[1]-b1[1]) < 0.01 && math.Abs(a2[1]-b2[1]) < 0.01 && math.Abs(a1[1]-a2[1]) < 0.5 {
		lo := fmax(fmin(a1[0], b1[0]), fmin(a2[0], b2[0]))
		hi := fmin(fmax(a1[0], b1[0]), fmax(a2[0], b2[0]))
		return hi - lo
	}
	if math.Abs(a1[0]-b1[0]) < 0.01 && math.Abs(a2[0]-b2[0]) < 0.01 && math.Abs(a1[0]-a2[0]) < 0.5 {
		lo := fmax(fmin(a1[1], b1[1]), fmin(a2[1], b2[1]))
		hi := fmin(fmax(a1[1], b1[1]), fmax(a2[1], b2[1]))
		return hi - lo
	}
	return 0
}

// Check looks for geometry errors in a laid-out diagram: overlapping nodes,
// nodes extending outside their lane, oblique wire segments, wires crossing
// nodes, overlapping wires with different sources and different targets, and
// labels overlapping nodes, other labels or other wires.
//
// A correct engine never produces these errors; Check is a safety net for the
// engine itself. The result is deduplicated and keeps the order of first
// detection.
func Check(r Result) []Finding {
	var issues []Finding
	add := func(level, code, format string, a ...any) {
		issues = append(issues, Finding{level, code, fmt.Sprintf(format, a...)})
	}

	boxes := map[string]box{}
	for _, it := range r.Items {
		boxes[it.ID] = it.box()
	}
	ids := make([]string, 0, len(r.Items))
	for _, it := range r.Items {
		ids = append(ids, it.ID)
	}
	sort.Strings(ids)
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			if area(boxes[a], boxes[b]) > 0 {
				add("error", "check.node-overlap", "%s and %s overlap", a, b)
			}
		}
	}
	for _, it := range r.Items {
		lx, lw := r.LaneX[it.Lane], r.LaneW[it.Lane]
		if it.X < lx || it.X+it.W > lx+lw {
			add("error", "check.outside-lane", "%s extends outside its lane", it.ID)
		}
	}

	type wire struct {
		edge int
		a, b [2]float64
	}
	var all []wire
	for ei, e := range r.Edges {
		pts := e.Pts
		for k := 0; k+1 < len(pts); k++ {
			a, b := pts[k], pts[k+1]
			if math.Abs(a[0]-b[0]) > 0.01 && math.Abs(a[1]-b[1]) > 0.01 {
				add("error", "check.oblique-segment", "%s: segment %d is not orthogonal", e.ID, k)
			}
			for _, it := range r.Items {
				// The first and last segments may touch the source and target
				// nodes themselves, since they start and end on those nodes' edges.
				if (it.ID == e.Src || it.ID == e.Dst) && (k == 0 || k == len(pts)-2) {
					continue
				}
				if segHits(a, b, shrink(boxes[it.ID], 1)) {
					add("error", "check.edge-crosses-node", "%s: wire crosses %s", e.ID, it.ID)
				}
			}
			all = append(all, wire{ei, a, b})
		}
	}
	for i, w1 := range all {
		for _, w2 := range all[i+1:] {
			e1, e2 := r.Edges[w1.edge], r.Edges[w2.edge]
			// Wires with the same source or the same target may overlap: they
			// merge into one shared line.
			if w1.edge == w2.edge || e1.Dst == e2.Dst || e1.Src == e2.Src {
				continue
			}
			if collinearOverlap(w1.a, w1.b, w2.a, w2.b) > 1 {
				add("error", "check.edge-overlap", "%s and %s have overlapping wires", e1.ID, e2.ID)
			}
		}
	}

	type label struct {
		edge int
		b    box
	}
	var labels []label
	for ei, e := range r.Edges {
		if e.Label != nil {
			labels = append(labels, label{ei, *e.Label})
		}
	}
	for i, lb := range labels {
		id := r.Edges[lb.edge].ID
		for _, it := range r.Items {
			if area(lb.b, boxes[it.ID]) > 0 {
				add("warning", "check.label-over-node", "label %s overlaps %s", id, it.ID)
			}
		}
		for _, lb2 := range labels[i+1:] {
			if area(lb.b, lb2.b) > 0 {
				add("warning", "check.label-over-label", "label %s overlaps label %s", id, r.Edges[lb2.edge].ID)
			}
		}
		for _, w := range all {
			if w.edge != lb.edge && segHits(w.a, w.b, lb.b) {
				add("warning", "check.label-over-edge", "label %s overlaps wire %s", id, r.Edges[w.edge].ID)
			}
		}
	}

	seen := map[Finding]bool{}
	var out []Finding
	for _, f := range issues {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}
