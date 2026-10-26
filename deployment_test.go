package diakempt

import (
	"os"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/view"
)

// TestDeploymentLayout: a hub's neighbors go on both sides of it, artifacts
// stay inside the nodes holding them, and nothing overlaps.
func TestDeploymentLayout(t *testing.T) {
	data, err := os.ReadFile("corpus/clean/deployment/01-shop.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dr := res.Report.Diagrams[0]
	if dr.Kind != "deployment" || len(dr.Operations) == 0 || dr.Operations[0].Op != "deploylayout" {
		t.Fatalf("kind %s, operations %v, step downs %v", dr.Kind, dr.Operations, dr.StepDowns)
	}
	if dr.After.NodeOverlaps > 0 || dr.After.WiresThroughNodes > 0 || dr.After.LabelOverlaps > 0 {
		t.Errorf("%d node overlaps, %d wires through shapes, %d label overlaps", dr.After.NodeOverlaps, dr.After.WiresThroughNodes, dr.After.LabelOverlaps)
	}
	d, _, _ := doc.Parse(res.Output)
	v := view.Build(d.Pages[0])
	var hub *view.Node
	left, right := 0, 0
	for _, n := range v.Nodes {
		if n.Parent != nil && !n.Parent.Box.ContainsRect(n.Box) {
			t.Errorf("%q sticks out of %q", n.Text, n.Parent.Text)
		}
		if n.Parent == nil && len(n.Children) > 0 && n.Text == "«execution environment»\nApp server" {
			hub = n
		}
	}
	if hub == nil {
		t.Fatal("no hub")
	}
	for _, n := range v.Nodes {
		switch {
		case n.Parent != nil || n == hub:
		case n.Box.Right() <= hub.Box.X:
			left++
		case n.Box.X >= hub.Box.Right():
			right++
		}
	}
	if left == 0 || right == 0 {
		t.Errorf("%d nodes left of the hub, %d right; want both sides", left, right)
	}
}
