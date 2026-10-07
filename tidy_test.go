package diakempt

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/internal/messify"
	"github.com/luytbq/diakempt/internal/xmltree"
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
