package tidy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/view"
)

func diagrams(t *testing.T, path string) []*Diagram {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var out []*Diagram
	for _, p := range d.Pages {
		for _, sd := range segment.Split(view.Build(p)).Diagrams {
			out = append(out, New(sd, text.Default()))
		}
	}
	return out
}

// TestFlowcastDiagramsMeasureClean checks the defect counts against diagrams
// whose geometry flowcast already verified: no wire through a node, no
// overlapping nodes.
func TestFlowcastDiagramsMeasureClean(t *testing.T) {
	files, _ := filepath.Glob("../corpus/clean/flowcast/*/*.drawio")
	for _, f := range files {
		ds := diagrams(t, f)
		if len(ds) != 1 {
			t.Errorf("%s: %d diagrams, want 1", filepath.Base(f), len(ds))
			continue
		}
		m := ds[0].Measure()
		if m.WiresThroughNodes > 0 || m.NodeOverlaps > 0 {
			t.Errorf("%s: %+v", filepath.Base(f), m)
		}
		if m.LabelOverlaps > 0 || m.WireOverlaps > 0 {
			t.Logf("%s: labels %d, wire overlaps %d", filepath.Base(f), m.LabelOverlaps, m.WireOverlaps)
		}
	}
}
