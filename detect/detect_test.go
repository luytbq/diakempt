package detect

import (
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/internal/messify"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/snap"
	"github.com/luytbq/diakempt/view"
)

func kinds(t *testing.T, d *doc.Document) []Result {
	t.Helper()
	var out []Result
	for _, p := range d.Pages {
		snap.Run(view.Build(p), snap.Params{Distance: 20, Ratio: 0.25, Margin: 1.5})
		for _, sd := range segment.Split(view.Build(p)).Diagrams {
			out = append(out, Detect(sd))
		}
	}
	return out
}

func read(t *testing.T, path string) *doc.Document {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// want maps a corpus folder to the kind every diagram in it must get.
var want = map[string]string{
	"flowcast/flowchart": Flowchart,
	"flowcast/swimlane":  Swimlane,
	"sequence":           Sequence,
	"handflow":           Flowchart,
	"architecture":       Unknown,
	"network":            Unknown,
	"mindmap":            Unknown,
	"class":              Class,
	"er":                 ER,
	"state":              State,
	"usecase":            UseCase,
}

func TestDetectCleanCorpus(t *testing.T) {
	for dir, kind := range want {
		files, _ := filepath.Glob(filepath.Join("../corpus/clean", dir, "*.drawio"))
		if len(files) == 0 {
			t.Fatalf("no files in %s", dir)
		}
		right, judged := 0, 0
		for _, f := range files {
			rs := kinds(t, read(t, f))
			if len(rs) == 0 {
				t.Errorf("%s/%s: no diagram", dir, filepath.Base(f))
				continue
			}
			if !tooSmall(rs[0]) {
				judged++
			}
			if rs[0].Kind == kind {
				if !tooSmall(rs[0]) {
					right++
				}
				continue
			}
			// A missed flowchart only costs the general tidy-up; calling
			// something else a flowchart is what must never happen.
			if kind == Unknown || rs[0].Kind != Unknown {
				t.Errorf("%s/%s: got %s (%s), want %s; scores %v; %s", dir, filepath.Base(f), rs[0].Kind,
					rs[0].Confidence, kind, rs[0].Scores, strings.Join(rs[0].Signals, "; "))
			}
		}
		if kind == Unknown {
			continue
		}
		t.Logf("%s: %d of %d large enough to judge detected as %s", dir, right, judged, kind)
		if float64(right) < 0.9*float64(judged) {
			t.Errorf("%s: only %d of %d detected", dir, right, judged)
		}
	}
}

// TestDetectSurvivesMess checks that messified flows are still recognized.
func TestDetectSurvivesMess(t *testing.T) {
	files, _ := filepath.Glob("../corpus/clean/flowcast/*/*.drawio")
	right, total := 0, 0
	for _, f := range files {
		d := read(t, f)
		rng := rand.New(rand.NewSource(3))
		messify.Strip(d)
		messify.Jitter(d, rng, 0.5, 40)
		messify.Detach(d, rng, messify.DetachOptions{Share: 0.2, MaxInside: 6, MaxOutside: 6})
		kind := Flowchart
		if strings.Contains(f, "swimlane") {
			kind = Swimlane
		}
		for _, r := range kinds(t, d) {
			if tooSmall(r) {
				continue
			}
			total++
			if r.Kind == kind {
				right++
			} else if r.Kind != Unknown {
				t.Errorf("%s: got %s, want %s", filepath.Base(f), r.Kind, kind)
			}
		}
	}
	t.Logf("messified flows: %d of %d detected", right, total)
	if float64(right) < 0.85*float64(total) {
		t.Errorf("only %d of %d detected", right, total)
	}
}

// tooSmall reports diagrams with too little in them to call a flow either
// way: fewer than three shapes or two directed wires. Leaving them unknown is
// the cautious answer.
func tooSmall(r Result) bool {
	return r.Scores[Flowchart] == 0 && r.Scores[Swimlane] == 0 && r.Scores[Sequence] == 0
}

// TestActivityWithFinalStateShapes: an activity diagram in lanes whose final
// nodes use the state machine's endState shape is still a swimlane diagram.
func TestActivityWithFinalStateShapes(t *testing.T) {
	d, _, err := doc.Parse([]byte(`<mxGraphModel><root>
  <mxCell id="0" /><mxCell id="1" parent="0" />
  <mxCell id="l1" value="Customer" style="swimlane;startSize=30;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="240" height="600" as="geometry" /></mxCell>
  <mxCell id="l2" value="System" style="swimlane;startSize=30;" vertex="1" parent="1"><mxGeometry x="240" y="0" width="240" height="600" as="geometry" /></mxCell>
  <mxCell id="s" style="ellipse;fillColor=#000000;" vertex="1" parent="l1"><mxGeometry x="105" y="50" width="30" height="30" as="geometry" /></mxCell>
  <mxCell id="a" value="Scan code" style="rounded=1;" vertex="1" parent="l1"><mxGeometry x="60" y="120" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="b" value="Check" style="rounded=1;" vertex="1" parent="l2"><mxGeometry x="60" y="200" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="q" value="" style="rhombus;" vertex="1" parent="l2"><mxGeometry x="100" y="290" width="40" height="40" as="geometry" /></mxCell>
  <mxCell id="c" value="Reject" style="rounded=1;" vertex="1" parent="l2"><mxGeometry x="60" y="380" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="e1" style="ellipse;html=1;shape=endState;fillColor=#000000;" vertex="1" parent="l2"><mxGeometry x="105" y="480" width="30" height="30" as="geometry" /></mxCell>
  <mxCell id="d" value="Ride" style="rounded=1;" vertex="1" parent="l1"><mxGeometry x="60" y="380" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="e2" style="ellipse;html=1;shape=endState;fillColor=#000000;" vertex="1" parent="l1"><mxGeometry x="105" y="480" width="30" height="30" as="geometry" /></mxCell>
  <mxCell id="w1" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="s" target="a"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w2" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="a" target="b"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w3" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="b" target="q"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w4" value="[no]" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="q" target="c"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w5" value="[yes]" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="q" target="d"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w6" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="c" target="e1"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="w7" style="endArrow=open;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="d" target="e2"><mxGeometry relative="1" as="geometry" /></mxCell>
</root></mxGraphModel>`))
	if err != nil {
		t.Fatal(err)
	}
	rs := kinds(t, d)
	if len(rs) != 1 || rs[0].Kind != Swimlane {
		t.Fatalf("got %+v, want one swimlane diagram", rs)
	}
}

// TestERTablesDrawnAsStacks: tables drawn with the stacked shape the editor's
// ER library shares with UML classes, joined by ER relations, are an ER
// diagram.
func TestERTablesDrawnAsStacks(t *testing.T) {
	table := func(id string, x float64) string {
		return `<mxCell id="` + id + `" value="` + id + `" style="swimlane;fontStyle=0;childLayout=stackLayout;horizontal=1;startSize=26;horizontalStack=0;resizeParent=1;" vertex="1" parent="1"><mxGeometry x="` +
			strconv.FormatFloat(x, 'f', 0, 64) + `" y="0" width="160" height="86" as="geometry" /></mxCell>
  <mxCell id="` + id + `-id" value="id" style="text;" vertex="1" parent="` + id + `"><mxGeometry y="26" width="160" height="30" as="geometry" /></mxCell>
  <mxCell id="` + id + `-name" value="name" style="text;" vertex="1" parent="` + id + `"><mxGeometry y="56" width="160" height="30" as="geometry" /></mxCell>
`
	}
	d, _, err := doc.Parse([]byte(`<mxGraphModel><root>
  <mxCell id="0" /><mxCell id="1" parent="0" />
  ` + table("station", 0) + table("dock", 240) + table("bike", 480) + `
  <mxCell id="r1" style="startArrow=ERmandOne;endArrow=ERzeroToMany;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="station" target="dock"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="r2" style="startArrow=ERzeroToOne;endArrow=ERzeroToOne;edgeStyle=orthogonalEdgeStyle;" edge="1" parent="1" source="dock" target="bike"><mxGeometry relative="1" as="geometry" /></mxCell>
</root></mxGraphModel>`))
	if err != nil {
		t.Fatal(err)
	}
	rs := kinds(t, d)
	if len(rs) != 1 || rs[0].Kind != ER {
		t.Fatalf("got %+v, want one ER diagram", rs)
	}
}
