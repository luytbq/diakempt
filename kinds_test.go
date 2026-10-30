package diakempt

import (
	"os"
	"testing"
)

func TestDiagramKindsForceSingleDiagramsAndWinOverKind(t *testing.T) {
	data, err := os.ReadFile("corpus/cases/snap.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{
		Kind: "flowchart",
		DiagramKinds: map[string]string{
			"Page-1 #1":          "unknown",
			`Page-1 #3 "Choose"`: "swimlane",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		`Page-1 #1 "Order"`:     "unknown",
		`Page-1 #2 "Warehouse"`: "flowchart",
		`Page-1 #3 "Choose"`:    "swimlane",
		`Page-1 #4 "Right"`:     "flowchart",
	}
	for _, d := range res.Report.Diagrams {
		if d.Kind != want[d.ID] || !d.Forced {
			t.Errorf("%s: kind %s, forced %v; want %s, forced", d.ID, d.Kind, d.Forced, want[d.ID])
		}
	}
	if n := countCode(res.Report.Issues, "type.no_diagram"); n != 0 {
		t.Errorf("%d type.no_diagram warnings for references that match", n)
	}
}

func TestDiagramKindWithAnotherNameDoesNotMatch(t *testing.T) {
	data, err := os.ReadFile("corpus/cases/snap.drawio")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{DiagramKinds: map[string]string{
		`Page-1 #1 "Shipping"`: "unknown",
		"Page-3 #1":            "class",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if d := res.Report.Diagrams[0]; d.Forced {
		t.Errorf("%s was forced by a reference naming another diagram", d.ID)
	}
	if n := countCode(res.Report.Issues, "type.no_diagram"); n != 2 {
		t.Errorf("type.no_diagram warnings: %d, want 2", n)
	}
}

func TestValidateRejectsMalformedDiagramReferences(t *testing.T) {
	for ref, kind := range map[string]string{
		"Page1":        "class",
		"Page-0 #1":    "class",
		"Page-1 #2 x":  "class",
		"Page-1 #2":    "mindmap",
		`Page-1 #2 "a`: "class",
	} {
		if err := (Options{DiagramKinds: map[string]string{ref: kind}}).Validate(); err == nil {
			t.Errorf("%q=%s accepted", ref, kind)
		}
	}
	if err := (Options{DiagramKinds: map[string]string{`Page-2 #10 "Log in"`: "sequence"}}).Validate(); err != nil {
		t.Error(err)
	}
}
