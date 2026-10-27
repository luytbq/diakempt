package report

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTextExplainsAWorseKeptResult: a diagram whose kept geometry is worse
// says so and names the numbers in the same terms the never-worse rule uses.
func TestTextExplainsAWorseKeptResult(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after Metrics
		want          string
	}{
		{
			name:   "more defects",
			before: Metrics{LabelOverlaps: 1, Area: 1000, WireLength: 500, Defects: 20},
			after:  Metrics{LabelOverlaps: 2, Area: 1000, WireLength: 500, Defects: 40},
			want:   "worse: the kept result grew its defect score from 20.0 to 40.0",
		},
		{
			name:   "same defects, bigger area and wire length",
			before: Metrics{LabelOverlaps: 1, Area: 1000, WireLength: 500, Defects: 20},
			after:  Metrics{LabelOverlaps: 1, Area: 2000, WireLength: 900, Defects: 20},
			want:   "worse: the kept result grew its area from 1000 to 2000 and its wire length from 500 to 900",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Diagram{ID: `Page-1 #1 "A"`, Kind: "flowchart", Level: "normal", Applied: "normal",
				Before: tc.before, After: tc.after, Worse: true}
			text := File{Pages: 1, Diagrams: []Diagram{d}}.Text("a.drawio", false)
			if !strings.Contains(text, tc.want) {
				t.Errorf("text\n%s\nwant it to contain %q", text, tc.want)
			}
		})
	}
}

// TestFileWorseOnlyWhenADiagramIsWorse: --strict reads the per-diagram verdict,
// not the metrics.
func TestFileWorseOnlyWhenADiagramIsWorse(t *testing.T) {
	for _, tc := range []struct {
		name string
		ds   []Diagram
		want bool
	}{
		{"no diagrams", nil, false},
		{"none worse", []Diagram{{Worse: false}, {Worse: false}}, false},
		{"one worse", []Diagram{{Worse: false}, {Worse: true}}, true},
	} {
		if got := (File{Diagrams: tc.ds}).Worse(); got != tc.want {
			t.Errorf("%s: Worse = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestDiagramJSONCarriesWorseOnlyWhenTrue: the field is absent from the JSON of
// a diagram the never-worse rule accepted.
func TestDiagramJSONCarriesWorseOnlyWhenTrue(t *testing.T) {
	for _, tc := range []struct {
		name  string
		worse bool
		want  bool
	}{
		{"not worse", false, false},
		{"worse", true, true},
	} {
		data, err := json.Marshal(Diagram{ID: "Page-1 #1", Worse: tc.worse})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(data), `"worse"`); got != tc.want {
			t.Errorf("%s: json %s, want worse present = %v", tc.name, data, tc.want)
		}
	}
}

// TestTextKeepsQuietAboutAResultThatIsNotWorse: the line appears only for a
// diagram the never-worse rule rejects.
func TestTextKeepsQuietAboutAResultThatIsNotWorse(t *testing.T) {
	d := Diagram{ID: `Page-1 #1 "A"`, Kind: "flowchart", Level: "normal", Applied: "normal",
		Before: Metrics{Area: 1000, WireLength: 500, Defects: 20},
		After:  Metrics{Area: 1100, WireLength: 500, Defects: 20}}
	text := File{Pages: 1, Diagrams: []Diagram{d}}.Text("a.drawio", false)
	if strings.Contains(text, "worse:") {
		t.Errorf("text mentions a worse result:\n%s", text)
	}
}

// TestMetricsDefectsSurvivesJSON: the text report quotes Defects, so a report
// read back from JSON must still know it.
func TestMetricsDefectsSurvivesJSON(t *testing.T) {
	in := Metrics{LabelOverlaps: 2, Area: 1000, WireLength: 500, Score: 60, Defects: 40}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out Metrics
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if out.Defects != in.Defects {
		t.Errorf("defects after a round trip = %v, want %v (json %s)", out.Defects, in.Defects, data)
	}
}
