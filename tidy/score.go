package tidy

import (
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/report"
)

// Weights of each defect in the score. Heavy defects make a diagram wrong to
// read, crossings only make it harder; area and wire length only break ties
// between otherwise equal layouts. Changing them changes which results are kept
// (docs/adr/0004), so the corpus goldens must be reviewed with them.
const (
	weightThrough      = 100
	weightNodeOverlap  = 100
	weightLabelOverlap = 20
	weightWireOverlap  = 10
	weightCrossing     = 5
	weightArea         = 0.5 / 10000 // per square pixel
	weightLength       = 0.1 / 100   // per pixel
)

// overlapTol is how much two segments must share to count as one wire running
// on top of another.
const overlapTol = 5

// Measure counts the defects of the diagram's current geometry.
func (d *Diagram) Measure() report.Metrics {
	var m report.Metrics
	paths := make([]geom.Polyline, len(d.Wires))
	for i, w := range d.Wires {
		paths[i] = w.Path()
		m.WireLength += paths[i].Len()
	}
	solid := d.solidNodes()
	for i, w := range d.Wires {
		for _, n := range solid {
			if (w.Src != nil && w.Src.Within(n)) || (w.Dst != nil && w.Dst.Within(n)) {
				continue
			}
			if crosses(paths[i], n.Box) {
				m.WiresThroughNodes++
			}
		}
		if throughOwnEnd(paths[i], w) {
			m.WiresThroughNodes++
		}
	}
	for i := 0; i < len(solid); i++ {
		for j := i + 1; j < len(solid); j++ {
			a, b := solid[i], solid[j]
			if a.Within(b) || b.Within(a) {
				continue
			}
			if a.Box.Inset(1).Overlaps(b.Box.Inset(1)) {
				m.NodeOverlaps++
			}
		}
	}
	for _, n := range d.Nodes {
		if !n.Container {
			continue
		}
		for _, o := range d.Nodes {
			if o.Container || o.Text || o.Overlay || o.Within(n) || n.Within(o) {
				continue
			}
			if n.Box.Inset(1).Overlaps(o.Box.Inset(1)) && !n.Box.ContainsRect(o.Box) {
				m.NodeOverlaps++
			}
		}
	}
	// A shape sticking out of its container crosses the container's border.
	for _, n := range d.Nodes {
		if n.Parent != nil && !n.Text && !n.Overlay && !n.Parent.Box.Inset(-1).ContainsRect(n.Box) {
			m.NodeOverlaps++
		}
	}
	var labels []geom.Rect
	for _, w := range d.Wires {
		labels = append(labels, w.LabelBoxes()...)
	}
	for _, n := range d.Nodes {
		if n.Text && !n.Overlay {
			labels = append(labels, n.Box)
		}
	}
	for i := 0; i < len(labels); i++ {
		for j := i + 1; j < len(labels); j++ {
			if labels[i].Inset(1).Overlaps(labels[j].Inset(1)) {
				m.LabelOverlaps++
			}
		}
		for _, n := range solid {
			if labels[i].Inset(1).Overlaps(n.Box.Inset(1)) && !n.Box.ContainsRect(labels[i]) {
				m.LabelOverlaps++
			}
		}
	}
	for i := 0; i < len(d.Wires); i++ {
		for j := i + 1; j < len(d.Wires); j++ {
			m.WireCrossings += crossings(paths[i], paths[j])
			if !shareEnd(d.Wires[i], d.Wires[j]) && overlaps(paths[i], paths[j]) {
				m.WireOverlaps++
			}
		}
	}
	m.Area = d.Bounds().W * d.Bounds().H
	m.Score = weightThrough*float64(m.WiresThroughNodes) + weightNodeOverlap*float64(m.NodeOverlaps) +
		weightLabelOverlap*float64(m.LabelOverlaps) + weightWireOverlap*float64(m.WireOverlaps) +
		weightCrossing*float64(m.WireCrossings) + weightArea*m.Area + weightLength*m.WireLength
	return m
}

// solidNodes returns the leaves that draw a shape, as opposed to free text.
func (d *Diagram) solidNodes() []*Node {
	var out []*Node
	for _, n := range d.Nodes {
		if !n.Container && !n.Text && !n.Overlay {
			out = append(out, n)
		}
	}
	return out
}

// Bounds returns the box around every node and wire.
func (d *Diagram) Bounds() geom.Rect {
	var b geom.Rect
	for _, n := range d.Nodes {
		b = b.Union(n.Box)
	}
	for _, w := range d.Wires {
		b = b.Union(w.Path().Bounds())
	}
	return b
}

func crosses(pl geom.Polyline, r geom.Rect) bool {
	for _, s := range pl.Segments() {
		if s.CrossesRect(r, 2) {
			return true
		}
	}
	return false
}

// throughOwnEnd reports whether a wire passes through its own source or target
// on the way to its port, as it does when stale waypoints sit on the far side.
func throughOwnEnd(pl geom.Polyline, w *Wire) bool {
	segs := pl.Segments()
	if len(segs) == 0 {
		return false
	}
	if w.Src != nil && !w.Src.Container {
		in := w.Src.Box.Inset(2)
		for k, s := range segs {
			if k == 0 {
				if !in.Empty() && in.Contains(s.B) {
					return true
				}
				continue
			}
			if s.CrossesRect(w.Src.Box, -1) {
				return true
			}
		}
	}
	if w.Dst != nil && !w.Dst.Container && w.Dst != w.Src {
		in := w.Dst.Box.Inset(2)
		last := len(segs) - 1
		for k, s := range segs {
			if k == last {
				if !in.Empty() && in.Contains(s.A) {
					return true
				}
				continue
			}
			if s.CrossesRect(w.Dst.Box, -1) {
				return true
			}
		}
	}
	return false
}

func crossings(a, b geom.Polyline) int {
	n := 0
	for _, s := range a.Segments() {
		for _, t := range b.Segments() {
			if s.Intersects(t) {
				n++
			}
		}
	}
	return n
}

func overlaps(a, b geom.Polyline) bool {
	for _, s := range a.Segments() {
		for _, t := range b.Segments() {
			if s.Overlaps(t, overlapTol) {
				return true
			}
		}
	}
	return false
}

// shareEnd reports whether two wires meet at a node, where running together is
// how draw.io joins them.
func shareEnd(a, b *Wire) bool {
	return (a.Src != nil && (a.Src == b.Src || a.Src == b.Dst)) ||
		(a.Dst != nil && (a.Dst == b.Src || a.Dst == b.Dst))
}

// DefectScore is the part of the score that counts defects, without area and
// wire length.
func DefectScore(m report.Metrics) float64 {
	return weightThrough*float64(m.WiresThroughNodes) + weightNodeOverlap*float64(m.NodeOverlaps) +
		weightLabelOverlap*float64(m.LabelOverlaps) + weightWireOverlap*float64(m.WireOverlaps) +
		weightCrossing*float64(m.WireCrossings)
}
