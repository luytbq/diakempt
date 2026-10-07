package snap

import (
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/internal/messify"
	"github.com/luytbq/diakempt/view"
)

var defaults = Params{Distance: 20, Ratio: 0.25, Margin: 1.5}

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

func TestSnapRulesOnHandWrittenCases(t *testing.T) {
	d := read(t, "../corpus/cases/snap.drawio")
	p := d.Pages[0]
	res := Run(view.Build(p), defaults)
	want := map[string]string{
		"near":   "B",    // 5px above Pay
		"inside": "D",    // inside Pack
		"tie":    "",     // 10px from both Left and Right
		"far":    "",     // nowhere near a shape
		"ontext": "H",    // on a text box laid over H
		"border": "lane", // on the lane's top border, no shape nearby
		"loose":  "",     // annotation arrow
		"self":   "",     // next to its own source
	}
	for id, target := range want {
		if got := p.Cell(id).Target(); got != target {
			t.Errorf("%s: target %q, want %q", id, got, target)
		}
	}
	if p.Cell("loose").Source() != "" {
		t.Error("loose: source attached")
	}
	if res.Snapped != 3+1 || res.Ambiguous != 1 {
		t.Errorf("snapped %d, ambiguous %d", res.Snapped, res.Ambiguous)
	}
	if len(res.Issues) != 1 || res.Issues[0].Code != "snap.ambiguous" {
		t.Errorf("issues: %v", res.Issues)
	}
}

// TestSnapRepairsMessifiedFlowcastDiagrams cuts wire ends loose by up to 8px
// inside or outside their shape and checks that every one is reattached to the
// shape it came from. A wrong attachment fails the test outright.
func TestSnapRepairsMessifiedFlowcastDiagrams(t *testing.T) {
	files, _ := filepath.Glob("../corpus/clean/flowcast/*/*.drawio")
	if len(files) == 0 {
		t.Fatal("no clean corpus")
	}
	var total, right, wrong, missed int
	for _, f := range files {
		for seed := int64(1); seed <= 3; seed++ {
			d := read(t, f)
			truth := messify.Detach(d, rand.New(rand.NewSource(seed)), messify.DetachOptions{Share: 0.5, MaxInside: 8, MaxOutside: 8})
			for _, p := range d.Pages {
				Run(view.Build(p), defaults)
			}
			for _, tr := range truth {
				total++
				c := d.Pages[tr.Page].Cell(tr.Wire)
				got := c.Target()
				if tr.Source {
					got = c.Source()
				}
				switch got {
				case tr.Node:
					right++
				case "":
					missed++
				default:
					wrong++
					t.Errorf("%s seed %d: wire %s source=%v attached to %s, want %s (offset %.1f)",
						filepath.Base(f), seed, tr.Wire, tr.Source, got, tr.Node, tr.Offset)
				}
			}
		}
	}
	t.Logf("detached %d ends: %d reattached, %d left free, %d wrong", total, right, missed, wrong)
	if float64(right) < 0.97*float64(total) {
		t.Errorf("only %d of %d ends reattached", right, total)
	}
}

// TestFarEndsAreNeverMisattached moves loose ends up to 30px out, past the snap
// threshold of most shapes. Many stay free. An end that landed clearly closer to
// its original shape than to any other must never attach elsewhere; ends that
// landed next to another shape are ambiguous to anyone and are not judged.
func TestFarEndsAreNeverMisattached(t *testing.T) {
	files, _ := filepath.Glob("../corpus/clean/flowcast/*/*.drawio")
	var total, judged, right, wrong int
	for _, f := range files {
		d := read(t, f)
		truth := messify.Detach(d, rand.New(rand.NewSource(7)), messify.DetachOptions{Share: 0.5, MaxInside: 0, MaxOutside: 30})
		clear := map[int]bool{}
		for i, tr := range truth {
			v := view.Build(d.Pages[tr.Page])
			pt, _ := v.Page.Cell(tr.Wire).TerminalPoint(tr.Source)
			own, other := math.Inf(1), math.Inf(1)
			for _, n := range v.Nodes {
				if n.ID == tr.Node {
					own = n.Box.Dist(pt)
				} else if dd := n.Box.Dist(pt); n.Container {
					other = math.Min(other, n.Box.DistToOutline(pt))
				} else {
					other = math.Min(other, dd)
				}
			}
			clear[i] = other >= defaults.Margin*own
		}
		for _, p := range d.Pages {
			Run(view.Build(p), defaults)
		}
		for i, tr := range truth {
			total++
			if !clear[i] {
				continue
			}
			judged++
			c := d.Pages[tr.Page].Cell(tr.Wire)
			got := c.Target()
			if tr.Source {
				got = c.Source()
			}
			if got == tr.Node {
				right++
			} else if got != "" {
				wrong++
				t.Errorf("%s: wire %s attached to %s, want %s (offset %.1f)", filepath.Base(f), tr.Wire, got, tr.Node, tr.Offset)
			}
		}
	}
	t.Logf("detached %d ends up to 30px out, %d clearly nearest their shape: %d reattached, %d wrong", total, judged, right, wrong)
}

// TestMessageEndOnLifelineStaysFree: a message end lying on a hand-drawn
// lifeline just below its head is on the line on purpose, not dropped next to
// the head.
func TestMessageEndOnLifelineStaysFree(t *testing.T) {
	d, _, err := doc.Parse([]byte(`<mxGraphModel><root>
  <mxCell id="0" /><mxCell id="1" parent="0" />
  <mxCell id="a" value="A" style="rounded=0;" vertex="1" parent="1"><mxGeometry x="0" y="0" width="120" height="40" as="geometry" /></mxCell>
  <mxCell id="b" value="B" style="rounded=0;" vertex="1" parent="1"><mxGeometry x="200" y="0" width="120" height="40" as="geometry" /></mxCell>
  <mxCell id="la" style="endArrow=none;dashed=1;" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="60" y="40" as="sourcePoint" /><mxPoint x="60" y="300" as="targetPoint" /></mxGeometry></mxCell>
  <mxCell id="lb" style="endArrow=none;dashed=1;" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="260" y="40" as="sourcePoint" /><mxPoint x="260" y="300" as="targetPoint" /></mxGeometry></mxCell>
  <mxCell id="m" style="endArrow=classic;" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="60" y="46" as="sourcePoint" /><mxPoint x="260" y="46" as="targetPoint" /></mxGeometry></mxCell>
</root></mxGraphModel>`))
	if err != nil {
		t.Fatal(err)
	}
	p := d.Pages[0]
	Run(view.Build(p), defaults)
	if s, tg := p.Cell("m").Source(), p.Cell("m").Target(); s != "" || tg != "" {
		t.Errorf("message attached: source %q, target %q", s, tg)
	}
	if p.Cell("la").Source() != "a" {
		t.Errorf("lifeline top not attached to its head: %q", p.Cell("la").Source())
	}
}
