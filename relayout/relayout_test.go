package relayout

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/view"
)

// TestFlowcastOutputLaysOutTheSame relays out diagrams flowcast drew and
// compares every shape with where flowcast put it, relative to the diagram's
// corner. The engine is flowcast's, so a diagram read back faithfully lays out
// the same; a mismatch means the reading lost something the table said.
func TestFlowcastOutputLaysOutTheSame(t *testing.T) {
	files, _ := filepath.Glob("../corpus/clean/flowcast/*/*.drawio")
	same, total := 0, 0
	var differ []string
	for _, f := range files {
		data, _ := os.ReadFile(f)
		d, _, err := doc.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		for _, sd := range segment.Split(view.Build(d.Pages[0])).Diagrams {
			det := detect.Detect(sd)
			if det.Kind != detect.Flowchart && det.Kind != detect.Swimlane {
				continue
			}
			plan, err := Build(sd, det.Kind == detect.Swimlane)
			if err != nil {
				t.Errorf("%s: %v", filepath.Base(f), err)
				continue
			}
			r, err := plan.Lay(text.Default())
			if err != nil {
				t.Errorf("%s: %v", filepath.Base(f), err)
				continue
			}
			w := tidy.New(sd, text.Default())
			plan.Apply(w, r, sd)
			total++
			ok := true
			for _, n := range w.Nodes {
				if math.Abs(n.Box.X-n.Orig.X) > 1 || math.Abs(n.Box.Y-n.Orig.Y) > 1 ||
					math.Abs(n.Box.W-n.Orig.W) > 1 || math.Abs(n.Box.H-n.Orig.H) > 1 {
					if ok && testing.Verbose() {
						t.Logf("%s: %s %q at %+v, flowcast had %+v", filepath.Base(f), n.V.ID, n.V.Text, n.Box, n.Orig)
					}
					ok = false
				}
			}
			if ok {
				same++
			} else {
				differ = append(differ, filepath.Base(filepath.Dir(f))+"/"+filepath.Base(f))
			}
		}
	}
	t.Logf("%d of %d diagrams lay out exactly as flowcast drew them; differ: %s", same, total, strings.Join(differ, " "))
	if float64(same) < 0.79*float64(total) {
		t.Errorf("only %d of %d match", same, total)
	}
}

// TestLongLabelsGetRoom: a wire's label is never wrapped by draw.io, so the
// layout leaves room for it on one line, clear of the shapes beside it.
func TestLongLabelsGetRoom(t *testing.T) {
	d, _, err := doc.Parse([]byte(`<mxGraphModel><root>
  <mxCell id="0" /><mxCell id="1" parent="0" />
  <mxCell id="s" style="ellipse;fillColor=#000000;" vertex="1" parent="1"><mxGeometry x="60" y="0" width="30" height="30" as="geometry" /></mxCell>
  <mxCell id="a" value="Waiting" style="rounded=1;" vertex="1" parent="1"><mxGeometry x="20" y="80" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="b" value="Started" style="rounded=1;" vertex="1" parent="1"><mxGeometry x="20" y="200" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="c" value="Expired" style="rounded=1;" vertex="1" parent="1"><mxGeometry x="220" y="80" width="120" height="50" as="geometry" /></mxCell>
  <mxCell id="e" style="ellipse;shape=endState;fillColor=#000000;" vertex="1" parent="1"><mxGeometry x="60" y="320" width="30" height="30" as="geometry" /></mxCell>
  <mxCell id="t1" value="open" style="endArrow=open;html=1;" edge="1" parent="1" source="s" target="a"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="t2" value="complete()" style="endArrow=open;html=1;" edge="1" parent="1" source="a" target="b"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="t3" value="expire() [now - createdAt &gt;= 60 seconds and nobody took the bike]" style="endArrow=open;html=1;" edge="1" parent="1" source="a" target="c"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="t4" style="endArrow=open;html=1;" edge="1" parent="1" source="b" target="e"><mxGeometry relative="1" as="geometry" /></mxCell>
  <mxCell id="t5" style="endArrow=open;html=1;" edge="1" parent="1" source="c" target="e"><mxGeometry relative="1" as="geometry" /></mxCell>
</root></mxGraphModel>`))
	if err != nil {
		t.Fatal(err)
	}
	sd := segment.Split(view.Build(d.Pages[0])).Diagrams[0]
	w := tidy.New(sd, text.Default())
	if _, err := Run(w, sd, false, text.Default()); err != nil {
		t.Fatal(err)
	}
	if m := w.Measure(); m.LabelOverlaps > 0 {
		t.Errorf("%d label overlaps", m.LabelOverlaps)
	}
}
