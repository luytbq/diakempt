package diakempt

import (
	"os"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/view"
)

// TestUseCaseLayout: use cases end up in one column inside the system
// boundary, every actor on the side of it it was drawn on, and no wire runs
// through a shape.
func TestUseCaseLayout(t *testing.T) {
	for _, f := range []string{"corpus/clean/usecase/01-library.drawio", "corpus/clean/usecase/02-library-scattered.drawio"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Tidy(data, Options{})
		if err != nil {
			t.Fatal(err)
		}
		dr := res.Report.Diagrams[0]
		if dr.Kind != "usecase" || len(dr.Operations) == 0 || dr.Operations[0].Op != "usecaselayout" {
			t.Fatalf("%s: kind %s, operations %v, step downs %v", f, dr.Kind, dr.Operations, dr.StepDowns)
		}
		if dr.After.WiresThroughNodes > 0 {
			t.Errorf("%s: %d wires through shapes", f, dr.After.WiresThroughNodes)
		}
		before, _, _ := doc.Parse(data)
		after, _, _ := doc.Parse(res.Output)
		vb, va := view.Build(before.Pages[0]), view.Build(after.Pages[0])
		var boundary *view.Node
		for _, n := range va.Nodes {
			if n.Overlay {
				boundary = n
			}
		}
		if boundary == nil {
			t.Fatalf("%s: no boundary", f)
		}
		mid := vb.NodeOf(boundary.ID).Box.Center().X
		x := -1.0
		for _, n := range va.Nodes {
			switch n.Shape {
			case "ellipse":
				if !boundary.Box.ContainsRect(n.Box) {
					t.Errorf("%s: use case %q is outside the boundary", f, n.Text)
				}
				if x >= 0 && n.Box.Center().X != x {
					t.Errorf("%s: use case %q is out of the column", f, n.Text)
				}
				x = n.Box.Center().X
			case "umlActor":
				left := vb.NodeOf(n.ID).Box.Center().X < mid
				if left != (n.Box.Right() < boundary.Box.X) || !left && n.Box.X < boundary.Box.Right() {
					t.Errorf("%s: actor %q changed sides", f, n.Text)
				}
			}
		}
	}
}
