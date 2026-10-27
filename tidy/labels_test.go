package tidy

import (
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/view"
)

// lifelineDiagram builds a sequence diagram's working copy with one lifeline
// and one horizontal wire carrying a label at pos.
func lifelineDiagram(pos float64) *Diagram {
	lifeline := &Node{
		V:         &view.Node{Shape: "umlLifeline"},
		Box:       geom.Rect{X: 100, Y: 0, W: 40, H: 400},
		Container: true,
	}
	w := &Wire{
		V:      &view.Wire{Cell: &doc.Cell{ID: "w"}},
		Points: []geom.Point{{X: 0, Y: 100}, {X: 400, Y: 100}},
		labels: []label{{cell: &doc.Cell{ID: "l"}, pos: pos, w: 40, h: 20}},
	}
	return &Diagram{Nodes: []*Node{lifeline}, Wires: []*Wire{w}}
}

// TestLabelsMovesAWireLabelOffALifelineCenterLine: Measure counts a label the
// dashed center line runs through, and Labels can slide it away along its wire.
func TestLabelsMovesAWireLabelOffALifelineCenterLine(t *testing.T) {
	d := lifelineDiagram(-0.4)
	before := d.Measure()
	if before.LabelOverlaps != 1 {
		t.Fatalf("label overlaps before = %d, want 1", before.LabelOverlaps)
	}
	d.Labels(&Log{})
	if pos := d.Wires[0].labels[0].pos; pos == -0.4 {
		t.Errorf("label stayed at pos %s", geom.Format(pos))
	}
	if after := d.Measure(); after.LabelOverlaps != 0 {
		t.Errorf("label overlaps after = %d, want 0", after.LabelOverlaps)
	}
}

// TestLabelsDoesNotMoveAnOverlayTextOffALifeline: Measure does not list overlay
// frames as labels, so one over a center line is not struck through and must
// stay where it was drawn.
func TestLabelsDoesNotMoveAnOverlayTextOffALifeline(t *testing.T) {
	lifeline := &Node{
		V:         &view.Node{Shape: "umlLifeline"},
		Box:       geom.Rect{X: 100, Y: 0, W: 40, H: 400},
		Container: true,
	}
	overlay := &Node{
		V:       &view.Node{ID: "f", Shape: "umlFrame"},
		Box:     geom.Rect{X: 80, Y: 90, W: 80, H: 20},
		Text:    true,
		Overlay: true,
	}
	d := &Diagram{Nodes: []*Node{lifeline, overlay}}
	if before := d.Measure(); before.LabelOverlaps != 0 {
		t.Fatalf("label overlaps before = %d, want 0", before.LabelOverlaps)
	}
	d.Labels(&Log{})
	if got := d.Nodes[1].Box; got != overlay.Box {
		t.Errorf("overlay moved to %v, want %v", got, overlay.Box)
	}
}

// TestLabelsDoesNotCountALabelOverTheLifelineHead: the center line starts below
// the head, so a label there is not struck through and stays put.
func TestLabelsDoesNotCountALabelOverTheLifelineHead(t *testing.T) {
	d := lifelineDiagram(-0.4)
	d.Wires[0].Points = []geom.Point{{X: 0, Y: 20}, {X: 400, Y: 20}}
	if before := d.Measure(); before.LabelOverlaps != 0 {
		t.Fatalf("label overlaps before = %d, want 0", before.LabelOverlaps)
	}
	d.Labels(&Log{})
	if pos := d.Wires[0].labels[0].pos; pos != -0.4 {
		t.Errorf("label moved to pos %s, want -0.4", geom.Format(pos))
	}
	if after := d.Measure(); after.LabelOverlaps != 0 {
		t.Errorf("label overlaps after = %d, want 0", after.LabelOverlaps)
	}
}
