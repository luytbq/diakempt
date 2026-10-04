package detect

import (
	"math/rand"
	"os"
	"path/filepath"
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
	"class":              Unknown,
	"er":                 Unknown,
	"state":              Unknown,
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
