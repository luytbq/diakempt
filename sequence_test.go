package diakempt

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/view"
)

// TestDrawnSequenceLayout lays out a sequence diagram whose messages have free
// ends on UML lifelines and whose frames are plain rectangles: every message
// ends up horizontal on lifeline center lines or on the sd frame's edge, and
// every frame still encloses the messages it enclosed.
func TestDrawnSequenceLayout(t *testing.T) {
	for _, f := range []string{"corpus/clean/sequence/08-drawn.drawio", "corpus/clean/sequence/09-drawn-scattered.drawio"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		res, err := Tidy(data, Options{})
		if err != nil {
			t.Fatal(err)
		}
		if n := len(res.Report.Diagrams); n != 1 {
			t.Fatalf("%s: %d diagrams, want 1", f, n)
		}
		dr := res.Report.Diagrams[0]
		if dr.Kind != "sequence" || len(dr.Operations) == 0 || dr.Operations[0].Op != "seqlayout" {
			t.Fatalf("%s: kind %s, operations %v, step downs %v", f, dr.Kind, dr.Operations, dr.StepDowns)
		}
		before, _, _ := doc.Parse(data)
		after, _, _ := doc.Parse(res.Output)
		vb, va := view.Build(before.Pages[0]), view.Build(after.Pages[0])
		var lines []float64
		var sd geom.Rect
		for _, n := range va.Nodes {
			if n.Shape == "umlLifeline" {
				lines = append(lines, n.Box.Center().X)
			}
			if strings.HasPrefix(n.Text, "sd ") {
				sd = n.Box
			}
		}
		onLine := func(x float64) bool {
			if math.Abs(x-sd.X) < 0.5 {
				return true
			}
			for _, l := range lines {
				if math.Abs(x-l) < 0.5 {
					return true
				}
			}
			return false
		}
		for _, w := range va.Wires {
			a, b := w.Path[0], w.Path[len(w.Path)-1]
			if w.Style.Value("endArrow", "") == "none" {
				continue
			}
			if len(w.Points) == 0 && math.Abs(a.Y-b.Y) > 0.5 {
				t.Errorf("%s: message %s is not horizontal: %v", f, w.Cell.ID, w.Path)
			}
			if (w.Src == nil && !onLine(a.X)) || (w.Dst == nil && !onLine(b.X)) {
				t.Errorf("%s: message %s does not end on lifelines: %v", f, w.Cell.ID, w.Path)
			}
		}
		for _, fb := range vb.Nodes {
			if !fb.Overlay || strings.HasPrefix(fb.Text, "sd ") {
				continue
			}
			fa := va.NodeOf(fb.ID)
			for i, w := range vb.Wires {
				if w.Style.Value("endArrow", "") == "none" {
					continue
				}
				mid := when(w)
				midA := when(va.Wires[i])
				if fb.Box.Contains(mid) != fa.Box.Contains(midA) {
					t.Errorf("%s: message %s was inside %q: %v, now %v", f, w.Cell.ID, fb.Text, fb.Box.Contains(mid), fa.Box.Contains(midA))
				}
			}
		}
	}
}

// when is the point a message happens at: its first waypoint, which is where a
// self call starts, else its middle.
func when(w *view.Wire) geom.Point {
	if len(w.Points) > 0 {
		return w.Points[0]
	}
	a, b := w.Path[0], w.Path[len(w.Path)-1]
	return geom.Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}
}
