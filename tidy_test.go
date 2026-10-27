package diakempt

import (
	"bytes"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/internal/messify"
	"github.com/luytbq/diakempt/internal/xmltree"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/view"
)

// input is one corpus file, possibly messified.
type input struct {
	name string
	data []byte
}

// corpus returns the hand-written cases and messified copies of the clean
// flowcast diagrams, a few seeds each.
func corpus(t *testing.T) []input {
	t.Helper()
	var out []input
	cases, _ := filepath.Glob("corpus/cases/*")
	for _, f := range cases {
		if strings.Contains(filepath.Base(f), ".drawio.") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, input{filepath.Base(f), data})
	}
	clean, _ := filepath.Glob("corpus/clean/flowcast/*/*.drawio")
	others, _ := filepath.Glob("corpus/clean/[^f]*/*.drawio")
	clean = append(others, clean...)
	if testing.Short() {
		clean = clean[:10]
	}
	for _, f := range clean {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for seed := int64(1); seed <= 2; seed++ {
			d, _, err := doc.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			rng := rand.New(rand.NewSource(seed))
			messify.Strip(d)
			messify.Jitter(d, rng, 0.5, 40)
			messify.Detach(d, rng, messify.DetachOptions{Share: 0.2, MaxInside: 6, MaxOutside: 6})
			out = append(out, input{filepath.Base(f) + "#" + string(rune('0'+seed)), d.Bytes()})
		}
	}
	return out
}

func tidyOrFail(t *testing.T, in input, opt Options) Result {
	t.Helper()
	res, err := Tidy(in.data, opt)
	if err != nil {
		t.Fatalf("%s: %v", in.name, err)
	}
	return res
}

func TestNeverWorseAndOrderKept(t *testing.T) {
	for _, lv := range Levels {
		for _, in := range corpus(t) {
			res := tidyOrFail(t, in, Options{Level: lv})
			for _, d := range res.Report.Diagrams {
				if worse(d.After, d.Before) {
					t.Errorf("%s %s %s: score %.2f --> %.2f", lv, in.name, d.ID, d.Before.Score, d.After.Score)
				}
				// A later pass retrying the same attempt must not report it twice.
				seen := map[string]bool{}
				for _, s := range d.StepDowns {
					if seen[s] {
						t.Errorf("%s %s %s: step down reported twice: %s", lv, in.name, d.ID, s)
					}
					seen[s] = true
				}
			}
		}
	}
}

// TestOnlyGeometryChanges compares input and output with every geometry
// element and geometry style key removed: what remains must be identical, apart
// from the wire ends snap attached.
func TestOnlyGeometryChanges(t *testing.T) {
	for _, in := range corpus(t) {
		res := tidyOrFail(t, in, Options{Level: Normal})
		snapped := map[string]bool{}
		for _, det := range res.Report.Snap.Details {
			snapped[det.Cells[0]] = true
		}
		a, b := skeleton(t, in.data, snapped), skeleton(t, res.Output, snapped)
		if a != b {
			t.Errorf("%s: content changed\n%s", in.name, firstDiff(a, b))
		}
	}
}

// geometryKeys are the style keys an operation may write.
var geometryKeys = []string{"exitX", "exitY", "exitDx", "exitDy", "exitPerimeter", "entryX", "entryY", "entryDx", "entryDy", "entryPerimeter", "whiteSpace"}

func skeleton(t *testing.T, data []byte, snapped map[string]bool) string {
	t.Helper()
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range d.Pages {
		for _, c := range p.Cells {
			st := c.Style()
			for _, k := range geometryKeys {
				st.Del(k)
			}
			e := c.Elem.Copy()
			node := e
			if c.Elem != c.Node {
				node = e.Find("mxCell")
			}
			node.Set("style", st.String())
			if snapped[c.ID] {
				node.Del("source")
				node.Del("target")
			}
			stripGeometry(node)
			b.WriteString(e.String())
			b.WriteString("\n")
		}
	}
	return b.String()
}

func stripGeometry(e *xmltree.Element) {
	for _, g := range e.FindAll("mxGeometry") {
		e.Remove(g)
	}
}

func firstDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) && i < len(lb); i++ {
		if la[i] != lb[i] {
			return "- " + la[i] + "\n+ " + lb[i]
		}
	}
	return "length differs"
}

func TestDeterministic(t *testing.T) {
	for _, in := range corpus(t)[:20] {
		a := tidyOrFail(t, in, Options{Level: Normal})
		b := tidyOrFail(t, in, Options{Level: Normal})
		if !bytes.Equal(a.Output, b.Output) {
			t.Errorf("%s: two runs differ", in.name)
		}
	}
}

// TestTidyingTwiceChangesNothing runs the tool on its own output: a second run
// must find nothing left to do.
func TestTidyingTwiceChangesNothing(t *testing.T) {
	for _, lv := range Levels {
		twice(t, lv)
	}
}

func twice(t *testing.T, lv Level) {
	var failed []string
	for _, in := range corpus(t) {
		first := tidyOrFail(t, in, Options{Level: lv})
		second := tidyOrFail(t, input{in.name, first.Output}, Options{Level: lv})
		if second.Report.Changed {
			var ops []string
			for _, d := range second.Report.Diagrams {
				for _, op := range d.Operations {
					ops = append(ops, op.Op)
				}
			}
			if second.Report.Snap.Snapped > 0 {
				ops = append(ops, "snap")
			}
			failed = append(failed, in.name+" ("+strings.Join(ops, ",")+")")
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		t.Errorf("%s: %d files changed again: %s", lv, len(failed), strings.Join(failed, " "))
	}
}

// TestCorpusSummary logs the total defects before and after per level, to show
// what each level achieves on the messified corpus. Run with -v.
func TestCorpusSummary(t *testing.T) {
	ins := corpus(t)
	for _, lv := range Levels {
		var before, after [5]int
		stepped := 0
		for _, in := range ins {
			res := tidyOrFail(t, in, Options{Level: lv})
			for _, d := range res.Report.Diagrams {
				b, a := d.Before, d.After
				before[0] += b.WiresThroughNodes
				before[1] += b.NodeOverlaps
				before[2] += b.LabelOverlaps
				before[3] += b.WireCrossings
				before[4] += b.WireOverlaps
				after[0] += a.WiresThroughNodes
				after[1] += a.NodeOverlaps
				after[2] += a.LabelOverlaps
				after[3] += a.WireCrossings
				after[4] += a.WireOverlaps
				if d.Applied != d.Level {
					stepped++
				}
			}
		}
		t.Logf("%s on %d files: through nodes %d-->%d, node overlaps %d-->%d, label overlaps %d-->%d, crossings %d-->%d, wire overlaps %d-->%d, stepped down %d",
			lv, len(ins), before[0], after[0], before[1], after[1], before[2], after[2], before[3], after[3], before[4], after[4], stepped)
	}
}

// TestWorseIsFalseForTheSameMetrics: keeping the original geometry after a
// step down never counts as worse, whatever its area and wire length.
func TestWorseIsFalseForTheSameMetrics(t *testing.T) {
	m := report.Metrics{LabelOverlaps: 2, Area: 12345, WireLength: 678}
	if worse(m, m) {
		t.Errorf("the same metrics %+v is worse than itself", m)
	}
}

// TestWorseToleratesSmallGrowthWithoutNewDefects: with the defects unchanged,
// area and wire length only count when they grew by more than a tenth.
func TestWorseToleratesSmallGrowthWithoutNewDefects(t *testing.T) {
	base := report.Metrics{LabelOverlaps: 2, Area: 10000, WireLength: 10000}
	for _, tc := range []struct {
		name  string
		after report.Metrics
		want  bool
	}{
		{"area up 5 percent", report.Metrics{LabelOverlaps: 2, Area: 10500, WireLength: 10000}, false},
		{"area up 20 percent", report.Metrics{LabelOverlaps: 2, Area: 12000, WireLength: 10000}, true},
		{"wire length up 5 percent", report.Metrics{LabelOverlaps: 2, Area: 10000, WireLength: 10500}, false},
		{"wire length up 20 percent", report.Metrics{LabelOverlaps: 2, Area: 10000, WireLength: 12000}, true},
	} {
		if got := worse(tc.after, base); got != tc.want {
			t.Errorf("%s: worse = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// labelOverlapsAfter tidies one corpus file at the default level and returns
// the label overlaps of the kept result.
func labelOverlapsAfter(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Tidy(data, Options{})
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	n := 0
	for _, d := range res.Report.Diagrams {
		n += d.After.LabelOverlaps
	}
	return n
}

// TestScatteredSequenceSettlesLabelsAfterSeqLayout: seqlayout rewrites the
// wires, so labels are settled on the geometry it keeps; a slide made before
// the layout can otherwise leave a label on another lifeline's line.
func TestScatteredSequenceSettlesLabelsAfterSeqLayout(t *testing.T) {
	if got := labelOverlapsAfter(t, "corpus/clean/sequence/07-checkout-scattered.drawio"); got > 1 {
		t.Errorf("%d label overlaps after, want at most 1", got)
	}
}

// TestSequenceCorpusDoesNotAddLabelOverlaps: every sequence corpus file keeps
// its label overlaps at or below the per-file ceiling in the table.
func TestSequenceCorpusDoesNotAddLabelOverlaps(t *testing.T) {
	limits := map[string]int{
		"01-uml-3x4.drawio":            0,
		"02-uml-4x7.drawio":            0,
		"03-uml-5x10.drawio":           0,
		"04-hand-3x5.drawio":           0,
		"05-hand-4x6.drawio":           0,
		"06-checkout.drawio":           0,
		"07-checkout-scattered.drawio": 0,
		"08-drawn.drawio":              0,
		"09-drawn-scattered.drawio":    0,
	}
	files, err := filepath.Glob("corpus/clean/sequence/*.drawio")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(limits) {
		t.Fatalf("%d sequence files, %d limits: add a limit for the new file", len(files), len(limits))
	}
	for _, f := range files {
		name := filepath.Base(f)
		want, ok := limits[name]
		if !ok {
			t.Fatalf("%s has no limit", name)
		}
		if got := labelOverlapsAfter(t, f); got > want {
			t.Errorf("%s: %d label overlaps after, want at most %d", name, got, want)
		}
	}
}

// selfCallDiagram is a sequence diagram with two lifelines and an activation
// bar whose self message carries a label wider than the gap the lifelines get
// by default, so a label centered on the loop would be struck by the line.
const selfCallDiagram = `<mxGraphModel><root>
<mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="c1" value="Customer" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="90" y="40" width="20" height="400" as="geometry"/></mxCell>
<mxCell id="c2" value="Web shop" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="190" y="40" width="100" height="400" as="geometry"/></mxCell>
<mxCell id="b1" value="" style="html=1;perimeter=orthogonalPerimeter;" vertex="1" parent="c1"><mxGeometry x="95" y="80" width="10" height="150" as="geometry"/></mxCell>
<mxCell id="e2" value="ask" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" source="c1" target="c2"><mxGeometry relative="1" as="geometry"><mxPoint x="100" y="120" as="sourcePoint"/><mxPoint x="240" y="120" as="targetPoint"/></mxGeometry></mxCell>
<mxCell id="e1" value="a self message label wider than the default gap" style="html=1;align=left;spacingLeft=2;endArrow=block;rounded=0;edgeStyle=orthogonalEdgeStyle;curved=0;" edge="1" parent="1" source="b1" target="b1"><mxGeometry relative="1" as="geometry"/></mxCell>
</root></mxGraphModel>`

// TestSeqLayoutPutsASelfMessageLabelBesideItsLoop: the label goes in the room
// between the loop and the next lifeline; centered on the loop edge it would
// cross the line it belongs to.
func TestSeqLayoutPutsASelfMessageLabelBesideItsLoop(t *testing.T) {
	res, err := Tidy([]byte(selfCallDiagram), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Report.Diagrams) != 1 {
		t.Fatalf("%d diagrams, want 1", len(res.Report.Diagrams))
	}
	d := res.Report.Diagrams[0]
	if d.After.LabelOverlaps != 0 {
		t.Errorf("%d label overlaps after, want 0", d.After.LabelOverlaps)
	}
	applied := false
	for _, op := range d.Operations {
		if op.Op == "seqlayout" {
			applied = true
		}
	}
	if !applied {
		t.Errorf("kept operations %v have no seqlayout entry", d.Operations)
	}
}

// barLabelDiagram is a sequence diagram with two lifelines and a wide
// activation bar on the right one. The message to the bar carries a label
// wider than the default gap, so it needs the room between the left lifeline's
// center line and the bar's edge. The bar is wide enough that leaving its
// inset out of spacing would make that room too narrow.
const barLabelDiagram = `<mxGraphModel><root>
<mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="c1" value="A" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="90" y="40" width="20" height="400" as="geometry"/></mxCell>
<mxCell id="c2" value="B" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="190" y="40" width="100" height="400" as="geometry"/></mxCell>
<mxCell id="b1" value="" style="html=1;perimeter=orthogonalPerimeter;" vertex="1" parent="c2"><mxGeometry x="10" y="80" width="80" height="200" as="geometry"/></mxCell>
<mxCell id="e2" value="ask" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" source="c1" target="c2"><mxGeometry relative="1" as="geometry"><mxPoint x="100" y="110" as="sourcePoint"/><mxPoint x="240" y="110" as="targetPoint"/></mxGeometry></mxCell>
<mxCell id="e1" value="an ordinary message wider than the gap" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" source="b1" target="c1"><mxGeometry relative="1" as="geometry"><mxPoint x="200" y="150" as="sourcePoint"/><mxPoint x="100" y="150" as="targetPoint"/></mxGeometry></mxCell>
</root></mxGraphModel>`

// labelStaysOnItsMessage reports whether a wire's first label in a draw.io file
// is still level with the wire's message row, rather than slid along a lifeline
// to find room.
func labelStaysOnItsMessage(t *testing.T, data []byte, id string) bool {
	t.Helper()
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 1 {
		t.Fatalf("%d pages, want 1", len(d.Pages))
	}
	td := segment.Split(view.Build(d.Pages[0]))
	for _, sd := range td.Diagrams {
		diagram := tidy.New(sd, text.Default())
		for _, w := range diagram.Wires {
			if w.V.Cell.ID != id {
				continue
			}
			boxes, pl := w.LabelBoxes(), w.Path()
			if len(boxes) == 0 || len(pl) == 0 {
				t.Fatalf("wire %s has no label or path", id)
			}
			return math.Abs(boxes[0].Center().Y-pl[0].Y) <= 10
		}
	}
	t.Fatalf("wire %s is gone", id)
	return false
}

// TestSeqLayoutLeavesRoomForALabelBesideABar: a label between a lifeline's
// center line and the bar its message ends on must clear both. Leaving the
// bar's inset out of spacing leaves no room beside the message, and the labels
// operation can then only clear it by sliding it up the lifeline.
func TestSeqLayoutLeavesRoomForALabelBesideABar(t *testing.T) {
	res, err := Tidy([]byte(barLabelDiagram), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Report.Diagrams) != 1 {
		t.Fatalf("%d diagrams, want 1", len(res.Report.Diagrams))
	}
	d := res.Report.Diagrams[0]
	if d.After.LabelOverlaps != 0 {
		t.Errorf("%d label overlaps after, want 0", d.After.LabelOverlaps)
	}
	applied := false
	for _, op := range d.Operations {
		if op.Op == "seqlayout" {
			applied = true
		}
	}
	if !applied {
		t.Errorf("kept operations %v have no seqlayout entry", d.Operations)
	}
	if !labelStaysOnItsMessage(t, res.Output, "e1") {
		t.Error("the label left its message row instead of fitting between the line and the bar")
	}
}

// foundMessageDiagram is a sequence diagram with three lifelines, a frame and a
// found message from the frame's edge to the third lifeline. Its label is wide
// enough that, centered on the wire, it lands on the middle lifeline's line.
const foundMessageDiagram = `<mxGraphModel><root>
<mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="c1" value="A" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="80" y="40" width="20" height="320" as="geometry"/></mxCell>
<mxCell id="c2" value="B" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="520" y="40" width="100" height="320" as="geometry"/></mxCell>
<mxCell id="c3" value="C" style="shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;" vertex="1" parent="1"><mxGeometry x="1050" y="40" width="100" height="320" as="geometry"/></mxCell>
<mxCell id="f" value="found order" style="shape=umlFrame;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="40" y="60" width="1200" height="190" as="geometry"/></mxCell>
<mxCell id="m1" value="one" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" source="c1" target="c2"><mxGeometry relative="1" as="geometry"><mxPoint x="100" y="140" as="sourcePoint"/><mxPoint x="560" y="145" as="targetPoint"/></mxGeometry></mxCell>
<mxCell id="m2" value="two" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" source="c2" target="c3"><mxGeometry relative="1" as="geometry"><mxPoint x="570" y="190" as="sourcePoint"/><mxPoint x="1090" y="198" as="targetPoint"/></mxGeometry></mxCell>
<mxCell id="found" value="found: an order" style="html=1;verticalAlign=bottom;endArrow=block;curved=0;" edge="1" parent="1" target="c3"><mxGeometry x="0" relative="1" as="geometry"><mxPoint x="40" y="90" as="sourcePoint"/></mxGeometry></mxCell>
</root></mxGraphModel>`

// foundLabelPosition reads the label position of the found message in a
// draw.io file, from -1 at the source to 1 at the target.
func foundLabelPosition(t *testing.T, data []byte) float64 {
	t.Helper()
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 1 {
		t.Fatalf("%d pages, want 1", len(d.Pages))
	}
	c := d.Pages[0].Cell("found")
	if c == nil {
		t.Fatal("the found message is gone")
	}
	x, _, _ := c.LabelPosition()
	return x
}

// TestSeqLayoutLeavesAFreeEndLabelWhereTheAuthorPutIt: spacing reserves no gap
// for a message with a free end, so placeMessageLabels must not move its label
// either. Labels are switched off to read the layout's own placement; the
// labels operation may still slide the label afterwards.
func TestSeqLayoutLeavesAFreeEndLabelWhereTheAuthorPutIt(t *testing.T) {
	want := foundLabelPosition(t, []byte(foundMessageDiagram))
	res, err := Tidy([]byte(foundMessageDiagram), Options{Ops: map[string]bool{"labels": false}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Report.Diagrams) != 1 {
		t.Fatalf("%d diagrams, want 1", len(res.Report.Diagrams))
	}
	d := res.Report.Diagrams[0]
	applied := false
	for _, op := range d.Operations {
		if op.Op == "seqlayout" {
			applied = true
		}
	}
	if !applied {
		t.Errorf("kept operations %v have no seqlayout entry", d.Operations)
	}
	if got := foundLabelPosition(t, res.Output); got != want {
		t.Errorf("found label at %s after seqlayout, want the author's %s", geom.Format(got), geom.Format(want))
	}

	// With labels settled, the label that the layout left on a line is moved
	// off it.
	res, err = Tidy([]byte(foundMessageDiagram), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Report.Diagrams[0].After.LabelOverlaps; got != 0 {
		t.Errorf("%d label overlaps after, want 0", got)
	}
}

// settleGuardDiagram is a diagram where a free text sits inside two shapes and
// one 40px step to the right would clear both but land on a UML lifeline's
// center line: fewer local hits, yet a measured label overlap.
func settleGuardDiagram() *tidy.Diagram {
	return &tidy.Diagram{Nodes: []*tidy.Node{
		{V: &view.Node{ID: "a"}, Box: geom.Rect{X: 0, Y: 0, W: 290, H: 300}},
		{V: &view.Node{ID: "b"}, Box: geom.Rect{X: 10, Y: 10, W: 280, H: 280}},
		{V: &view.Node{ID: "l", Shape: "umlLifeline"}, Box: geom.Rect{X: 305, Y: 0, W: 20, H: 400}, Container: true},
		{V: &view.Node{ID: "t"}, Box: geom.Rect{X: 250, Y: 140, W: 40, H: 20}, Text: true},
	}}
}

// TestSettleLabelsKeepsTheLayoutWhenLabelsMakesItWorse: Labels minimizes each
// label's own hits, so a move can raise the measured defects; settleLabels must
// then put the layout's geometry back and drop the labels entries.
func TestSettleLabelsKeepsTheLayoutWhenLabelsMakesItWorse(t *testing.T) {
	d := settleGuardDiagram()
	laid := d.Measure()
	if laid.LabelOverlaps != 0 {
		t.Fatalf("laid result has %d label overlaps, want 0", laid.LabelOverlaps)
	}
	after, log := settleLabels(d, laid, Safe, Options{}, "seqlayout")
	if after.LabelOverlaps != laid.LabelOverlaps || after.Score != laid.Score {
		t.Errorf("kept result %d label overlaps, score %.2f; want %d, %.2f",
			after.LabelOverlaps, after.Score, laid.LabelOverlaps, laid.Score)
	}
	if n := log.Counts["labels"]; n != 0 {
		t.Errorf("log kept %d labels entries, want none", n)
	}
	if box := d.Nodes[3].Box; box.X != 250 {
		t.Errorf("text at x %s, want 250", geom.Format(box.X))
	}
	// Without the guard the move happens, to show the case is not a no-op.
	bare := settleGuardDiagram()
	bare.Labels(&tidy.Log{})
	if got := bare.Measure().LabelOverlaps; got != 1 {
		t.Fatalf("Labels alone leaves %d label overlaps, want 1", got)
	}
}

// flowLayoutLabelDiagram is a flowchart whose flowlayout result has one wire
// label on a shape: the layout alone leaves it struck through, and settling
// labels afterwards clears it.
const flowLayoutLabelDiagram = `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="n1" value="Node 1" style="rounded=0;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="282" y="182" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="n2" value="Node 2" style="rounded=0;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="203" y="21" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="n3" value="Node 3" style="rounded=0;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="144" y="189" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="n4" value="Node 4" style="rounded=0;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="232" y="270" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="n5" value="Node 5" style="rounded=0;whiteSpace=wrap;html=1;" vertex="1" parent="1"><mxGeometry x="89" y="190" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="e1" edge="1" parent="1" source="n4" target="n1"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e1c" value="label 1" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e1"><mxGeometry x="0" y="2" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
<mxCell id="e2" edge="1" parent="1" source="n5" target="n1"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e2c" value="label 2" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e2"><mxGeometry x="0" y="-10" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
<mxCell id="e3" edge="1" parent="1" source="n4" target="n3"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e3c" value="label 3" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e3"><mxGeometry x="0" y="18" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
<mxCell id="e6" edge="1" parent="1" source="n4" target="n2"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e6c" value="label 6" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e6"><mxGeometry x="0" y="18" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
<mxCell id="e8" edge="1" parent="1" source="n4" target="n1"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e8c" value="label 8" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e8"><mxGeometry x="0" y="0" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
<mxCell id="e10" edge="1" parent="1" source="n2" target="n1"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="e10c" value="label 10" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="e10"><mxGeometry x="0" y="0" relative="1" as="geometry"><mxPoint as="offset"/></mxGeometry></mxCell>
</root></mxGraphModel>`

// TestFlowLayoutSettlesLabelsAfterAnAcceptedLayout: the flowlayout branch
// returns as soon as it keeps a layout, so the settle step after it must clear
// the labels that layout put on a shape; without it the kept result has the
// overlap and no later operation runs to fix it.
func TestFlowLayoutSettlesLabelsAfterAnAcceptedLayout(t *testing.T) {
	run := func(labels bool) report.Diagram {
		t.Helper()
		res, err := Tidy([]byte(flowLayoutLabelDiagram), Options{Kind: "flowchart", Ops: map[string]bool{"labels": labels}})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Report.Diagrams) != 1 {
			t.Fatalf("%d diagrams, want 1", len(res.Report.Diagrams))
		}
		return res.Report.Diagrams[0]
	}
	laid := run(false)
	if laid.After.LabelOverlaps != 1 {
		t.Fatalf("layout alone leaves %d label overlaps, want 1", laid.After.LabelOverlaps)
	}
	kept := run(true)
	if kept.After.LabelOverlaps != 0 {
		t.Errorf("kept result has %d label overlaps, want 0", kept.After.LabelOverlaps)
	}
	settled := false
	for _, op := range kept.Operations {
		if op.Op == "labels" {
			settled = true
		}
	}
	if !settled {
		t.Errorf("kept operations %v have no labels entry", kept.Operations)
	}
}
