package diakempt

import (
	"os"
	"strings"
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
	want := map[string]string{
		`Page-1 #1 "Shipping"`: `Page-1 #1 is "Order"`,
		"Page-3 #1":            "page 3 holds no diagram",
	}
	n := 0
	for _, is := range res.Report.Issues {
		if is.Code != "type.no_diagram" {
			continue
		}
		n++
		if hint := want[is.Diagram]; hint == "" || !strings.HasSuffix(is.Msg, hint) {
			t.Errorf("%s: %q does not end in %q", is.Diagram, is.Msg, hint)
		}
	}
	if n != 2 {
		t.Errorf("type.no_diagram warnings: %d, want 2", n)
	}
}

func TestDiagramReferencesForgiveCaseAndSpacing(t *testing.T) {
	for _, ref := range []string{"page-1#2", "Page-1  #2", `PAGE-1 #2  "Warehouse"`} {
		if !refMatches(ref, 0, 1, "Warehouse") {
			t.Errorf("%q does not name Page-1 #2", ref)
		}
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
	conflict := Options{DiagramKinds: map[string]string{"Page-1 #2": "class", `Page-1 #2 "Warehouse"`: "er"}}
	if err := conflict.Validate(); err == nil {
		t.Error("two types for Page-1 #2 accepted")
	}
	other := Options{DiagramKinds: map[string]string{`Page-1 #2 "A"`: "class", `Page-1 #2 "B"`: "er"}}
	if err := other.Validate(); err != nil {
		t.Errorf("two names at one index are two diagrams, in two files: %v", err)
	}
}
