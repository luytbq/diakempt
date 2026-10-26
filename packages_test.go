package diakempt

import (
	"os"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/view"
)

// TestPackageLayers: layers keep their drawn order top to bottom, every
// package stays inside the package holding it, and an interface goes above
// the subsystem realizing it.
func TestPackageLayers(t *testing.T) {
	data, err := os.ReadFile("corpus/clean/package/01-layers.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{})
	if err != nil {
		t.Fatal(err)
	}
	dr := res.Report.Diagrams[0]
	if dr.Kind != "package" || len(dr.Operations) == 0 || dr.Operations[0].Op != "pkglayout" {
		t.Fatalf("kind %s, operations %v, step downs %v", dr.Kind, dr.Operations, dr.StepDowns)
	}
	if dr.After.NodeOverlaps > 0 || dr.After.WiresThroughNodes > 0 {
		t.Errorf("%d node overlaps, %d wires through shapes", dr.After.NodeOverlaps, dr.After.WiresThroughNodes)
	}
	d, _, _ := doc.Parse(res.Output)
	v := view.Build(d.Pages[0])
	y := map[string]float64{}
	for _, n := range v.Nodes {
		for _, name := range []string{"Presentation", "Application", "Infrastructure", "IPayments", "PaymentSubsystem"} {
			if strings.Contains(n.Text, name) {
				y[name] = n.Box.Y
			}
		}
		if n.Parent != nil && !n.Parent.Box.ContainsRect(n.Box) {
			t.Errorf("%q sticks out of %q", n.Text, n.Parent.Text)
		}
	}
	for _, pair := range [][2]string{{"Presentation", "Application"}, {"Application", "Infrastructure"}, {"IPayments", "PaymentSubsystem"}} {
		if y[pair[0]] >= y[pair[1]] {
			t.Errorf("%s is not above %s", pair[0], pair[1])
		}
	}
}
