package tidy

import (
	"testing"

	"github.com/luytbq/diakempt/geom"
)

// compactDiagram builds a diagram from node boxes, with the other boxes
// standing for everything else on the page.
func compactDiagram(nodes, others []geom.Rect) *Diagram {
	d := &Diagram{Others: others}
	for _, b := range nodes {
		d.Nodes = append(d.Nodes, &Node{Box: b})
	}
	return d
}

func compactBoxes(d *Diagram) []geom.Rect {
	out := make([]geom.Rect, len(d.Nodes))
	for i, n := range d.Nodes {
		out[i] = n.Box
	}
	return out
}

// compactXCovers reports whether the node covers part of the box on the x
// axis: its whole width for a node, either edge for a container.
func compactXCovers(n *Node, b geom.Rect) bool {
	inside := func(v float64) bool { return v > b.X && v < b.Right() }
	if n.Container {
		return inside(n.Box.X) || inside(n.Box.Right())
	}
	return n.Box.X < b.Right() && n.Box.Right() > b.X
}

// TestCompactLeavesBandEndingOnAnotherDiagramOpen: a wide band whose far edge
// is a box Compact cannot move stays open, with the nodes on its near side
// where they were, and nothing is reported.
func TestCompactLeavesBandEndingOnAnotherDiagramOpen(t *testing.T) {
	d := compactDiagram(
		[]geom.Rect{{X: 0, Y: 0, W: 100, H: 60}, {X: 120, Y: 0, W: 100, H: 60}},
		[]geom.Rect{{X: 600, Y: 0, W: 100, H: 60}},
	)
	before := compactBoxes(d)
	log := &Log{}
	d.Compact(20, log)
	if n := log.Counts["compact"]; n != 0 {
		t.Errorf("closed %d bands, want 0", n)
	}
	if after := compactBoxes(d); after[0] != before[0] || after[1] != before[1] {
		t.Errorf("boxes %v, want %v", after, before)
	}
}

// TestCompactClosesBandAfterOneItCannotClose: the band ending on a box that
// cannot move is skipped, and the wider one behind it still closes, once.
func TestCompactClosesBandAfterOneItCannotClose(t *testing.T) {
	d := compactDiagram(
		[]geom.Rect{{X: 0, Y: 0, W: 100, H: 60}, {X: 900, Y: 0, W: 100, H: 60}, {X: 1020, Y: 0, W: 100, H: 60}},
		[]geom.Rect{{X: 500, Y: 0, W: 100, H: 60}},
	)
	log := &Log{}
	d.Compact(20, log)
	if n := log.Counts["compact"]; n != 1 {
		t.Errorf("closed %d bands, want 1", n)
	}
	if got := d.Nodes[0].Box.X; got != 0 {
		t.Errorf("node before both bands moved to x %s, want 0", geom.Format(got))
	}
	// The unclosable band ends at 600, so the moved nodes keep their gap to it.
	if got := d.Nodes[1].Box.X; got != 640 {
		t.Errorf("node behind the band at x %s, want 640", geom.Format(got))
	}
	if got := d.Nodes[2].Box.X; got != 760 {
		t.Errorf("node after it at x %s, want 760", geom.Format(got))
	}
}

// TestCompactLeavesBandEndingOnSharedEdgeOpen: a node and a box that cannot
// move that begin at one coordinate keep the band before them, so the node is
// not pulled away from the box.
func TestCompactLeavesBandEndingOnSharedEdgeOpen(t *testing.T) {
	d := compactDiagram(
		[]geom.Rect{{X: 0, Y: 0, W: 100, H: 60}, {X: 900, Y: 0, W: 100, H: 60}},
		[]geom.Rect{{X: 900, Y: 0, W: 100, H: 60}},
	)
	before := compactBoxes(d)
	log := &Log{}
	d.Compact(20, log)
	if n := log.Counts["compact"]; n != 0 {
		t.Errorf("closed %d bands, want 0", n)
	}
	if after := compactBoxes(d); after[0] != before[0] || after[1] != before[1] {
		t.Errorf("boxes %v, want %v", after, before)
	}
}

// TestCompactDoesNotLandNodesOnAnotherDiagram: closing the band before a node
// must not push content that sits behind an unmovable box onto it, so that
// band stays open and the node behind the box stays where it was.
func TestCompactDoesNotLandNodesOnAnotherDiagram(t *testing.T) {
	other := geom.Rect{X: 300, Y: 0, W: 100, H: 60}
	d := compactDiagram(
		[]geom.Rect{
			{X: 0, Y: 0, W: 100, H: 60},
			{X: 250, Y: 200, W: 100, H: 60},
			{X: 410, Y: 0, W: 100, H: 60},
		},
		[]geom.Rect{other},
	)
	log := &Log{}
	d.Compact(20, log)
	for _, n := range d.Nodes {
		if n.Box.Overlaps(other) {
			t.Errorf("node at %v landed on the other diagram at %v", n.Box, other)
		}
	}
	if got := d.Nodes[1].Box.X; got != 250 {
		t.Errorf("node behind the band moved to x %s, want 250", geom.Format(got))
	}
	if got := d.Nodes[2].Box.X; got != 410 {
		t.Errorf("node behind the other diagram moved to x %s, want 410", geom.Format(got))
	}
	// Only the band across the other axis closes; the one before the node
	// would land the node behind the box on it.
	if n := log.Counts["compact"]; n != 1 {
		t.Errorf("closed %d bands, want 1", n)
	}
}

// TestCompactStaysClearOfOtherDiagramInsideContainer: a container whose empty
// inside holds another diagram's box closes the band before it but never puts
// a node or one of its edges on that box, and does not spin.
func TestCompactStaysClearOfOtherDiagramInsideContainer(t *testing.T) {
	other := geom.Rect{X: 800, Y: 0, W: 100, H: 60}
	d := compactDiagram([]geom.Rect{{X: 0, Y: 0, W: 100, H: 60}}, []geom.Rect{other})
	d.Nodes = append(d.Nodes, &Node{Box: geom.Rect{X: 600, Y: 0, W: 400, H: 200}, Container: true})
	log := &Log{}
	d.Compact(20, log)
	if n := log.Counts["compact"]; n != 3 {
		t.Errorf("closed %d bands, want 3", n)
	}
	for _, n := range d.Nodes {
		if compactXCovers(n, other) {
			t.Errorf("box %v covers the other diagram at %v", n.Box, other)
		}
	}
}
