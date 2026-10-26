package diakempt

import (
	"os"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/view"
)

// TestCommunicationLayout: objects are laid out in columns from the actor of
// the first message, each one right of the object it is first reached from,
// and no link runs through a shape.
func TestCommunicationLayout(t *testing.T) {
	for _, f := range []string{"corpus/clean/communication/01-borrow.drawio", "corpus/clean/communication/02-borrow-scattered.drawio"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Tidy(data, Options{})
		if err != nil {
			t.Fatal(err)
		}
		dr := res.Report.Diagrams[0]
		if dr.Kind != "communication" || len(dr.Operations) == 0 || dr.Operations[0].Op != "commlayout" {
			t.Fatalf("%s: kind %s, operations %v, step downs %v", f, dr.Kind, dr.Operations, dr.StepDowns)
		}
		if dr.After.WiresThroughNodes > 0 || dr.After.LabelOverlaps > 0 {
			t.Errorf("%s: %d wires through shapes, %d label overlaps", f, dr.After.WiresThroughNodes, dr.After.LabelOverlaps)
		}
		d, _, _ := doc.Parse(res.Output)
		v := view.Build(d.Pages[0])
		for _, w := range v.Wires {
			if w.Src == w.Dst {
				continue
			}
			if w.Src.Box.Right() >= w.Dst.Box.X {
				t.Errorf("%s: link %q does not run left to right", f, w.Text)
			}
		}
	}
}
