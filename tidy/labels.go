package tidy

import (
	"math"

	"github.com/luytbq/diakempt/geom"
)

// Labels moves labels off shapes and off each other. A free text moves to the
// nearest spot that clears everything, within reach of where it was; a wire
// label slides along its wire. Labels that cannot be cleared stay put.
func (d *Diagram) Labels(log *Log) {
	for _, n := range d.Nodes {
		if !n.Text || n.Container || d.labelHits(n.Box, n, nil, -1) == 0 {
			continue
		}
		orig := n.Box
		best, bestHits := geom.Point{}, d.labelHits(orig, n, nil, -1)
		for r := 5.0; r <= 60 && bestHits > 0; r += 5 {
			for _, dir := range []geom.Point{{Y: -1}, {X: 1}, {Y: 1}, {X: -1}, {X: 1, Y: -1}, {X: 1, Y: 1}, {X: -1, Y: 1}, {X: -1, Y: -1}} {
				off := geom.Point{X: dir.X * r, Y: dir.Y * r}
				if h := d.labelHits(orig.Move(off), n, nil, -1); h < bestHits {
					best, bestHits = off, h
				}
			}
		}
		if best != (geom.Point{}) {
			d.Move(n, best)
			log.add("labels", []string{n.V.ID}, "moved text %s by (%s, %s)", n.V.ID, geom.Format(best.X), geom.Format(best.Y))
		}
	}
	for _, w := range d.Wires {
		for i := range w.labels {
			boxes := w.LabelBoxes()
			if d.labelHits(boxes[i], nil, w, i) == 0 {
				continue
			}
			orig := w.labels[i]
			bestHits := d.labelHits(boxes[i], nil, w, i)
			bestPos := orig.pos
			for k := -9; k <= 9; k++ {
				pos := float64(k) / 10
				w.labels[i].pos = pos
				h := d.labelHits(w.LabelBoxes()[i], nil, w, i)
				// Fewer hits wins; among equal, the spot nearest the original.
				if h < bestHits || (h == bestHits && bestPos != orig.pos && math.Abs(pos-orig.pos) < math.Abs(bestPos-orig.pos)) {
					bestHits, bestPos = h, pos
				}
			}
			w.labels[i].pos = bestPos
			if bestPos != orig.pos {
				id := w.V.Cell.ID
				if orig.cell != nil {
					id = orig.cell.ID
				}
				log.add("labels", []string{id}, "slid label %s along wire %s", id, w.V.Cell.ID)
			}
		}
	}
}

// labelHits counts what a label box overlaps: solid shapes, other labels and
// free texts. self and (wire, index) identify the label itself.
func (d *Diagram) labelHits(b geom.Rect, self *Node, wire *Wire, index int) int {
	in := b.Inset(1)
	hits := 0
	for _, n := range d.Nodes {
		if n == self || n.Container {
			continue
		}
		if in.Overlaps(n.Box.Inset(1)) {
			hits++
		}
	}
	for _, w := range d.Wires {
		for i, lb := range w.LabelBoxes() {
			if w == wire && i == index {
				continue
			}
			if in.Overlaps(lb.Inset(1)) {
				hits++
			}
		}
	}
	return hits
}
