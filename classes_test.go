package diakempt

import (
	"os"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/view"
)

// TestAnalysisClassesRanked: an analysis class diagram joined only by
// associations is laid out with boundaries on top, controls in the middle and
// entities at the bottom, and every association between two rows leaves the
// upper class from its bottom and enters the lower one from its top.
func TestAnalysisClassesRanked(t *testing.T) {
	data, err := os.ReadFile("corpus/clean/class/05-analysis-row.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dr := res.Report.Diagrams[0]
	if len(dr.Operations) == 0 || dr.Operations[0].Op != "classlayout" {
		t.Fatalf("operations %v, step downs %v", dr.Operations, dr.StepDowns)
	}
	d, _, _ := doc.Parse(res.Output)
	v := view.Build(d.Pages[0])
	row := map[string]int{"boundary": 0, "control": 1, "entity": 2}
	of := map[*view.Node]int{}
	for _, n := range v.Nodes {
		for k, r := range row {
			if strings.Contains(n.Text, "«"+k+"»") {
				of[n] = r
			}
		}
	}
	for a, ra := range of {
		for b, rb := range of {
			if ra < rb && a.Box.Bottom() >= b.Box.Y {
				t.Errorf("%q (row %d) is not above %q (row %d)", a.Text, ra, b.Text, rb)
			}
		}
	}
	for _, w := range v.Wires {
		up, down := w.Src, w.Dst
		if of[up] == of[down] {
			continue
		}
		if of[up] > of[down] {
			up, down = down, up
		}
		top, bottom := w.Path[0], w.Path[len(w.Path)-1]
		if w.Src != up {
			top, bottom = bottom, top
		}
		if top.Y != up.Box.Bottom() || bottom.Y != down.Box.Y {
			t.Errorf("wire %s runs %v, want from the bottom of %q to the top of %q", w.Cell.ID, w.Path, up.Text, down.Text)
		}
	}
}

// TestInterfacesRanked: in a context diagram drawn with plain boxes, clients
// go above the interfaces they depend on, and the interfaces above the
// subsystem realizing them.
func TestInterfacesRanked(t *testing.T) {
	data, err := os.ReadFile("corpus/clean/class/06-interfaces-plain.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dr := res.Report.Diagrams[0]
	if dr.Kind != "class" || len(dr.Operations) == 0 || dr.Operations[0].Op != "classlayout" {
		t.Fatalf("kind %s, operations %v, step downs %v", dr.Kind, dr.Operations, dr.StepDowns)
	}
	d, _, _ := doc.Parse(res.Output)
	nodes := view.Build(d.Pages[0]).Nodes
	box := map[string]float64{}
	for _, name := range []string{"CheckoutService", "IPaymentGateway", "PaymentSubsystem"} {
		for _, n := range nodes {
			if strings.Contains(n.Text, name) {
				box[name] = n.Box.Y
			}
		}
	}
	for _, pair := range [][2]string{{"CheckoutService", "IPaymentGateway"}, {"IPaymentGateway", "PaymentSubsystem"}} {
		if box[pair[0]] >= box[pair[1]] {
			t.Errorf("%s (y %v) is not above %s (y %v)", pair[0], box[pair[0]], pair[1], box[pair[1]])
		}
	}
}
