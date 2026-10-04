package diakempt

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/internal/messify"
)

// showcaseDir receives a before and after file per example on every test run,
// for people to open in draw.io and judge the results by eye. It is not
// committed.
const showcaseDir = "showcase"

type example struct {
	name   string
	source string
	// Messified examples damage the source first: seed, largest move in pixels,
	// share of shapes moved.
	seed          int64
	jitter, share float64
	level         Level
}

var examples = []example{
	{"01-flowchart-merge", "corpus/clean/flowcast/flowchart/05-merge-node.drawio", 5, 70, 0.8, Normal},
	{"02-flowchart-labels", "corpus/clean/flowcast/flowchart/11-labels-crowded.drawio", 2, 50, 0.6, Normal},
	{"03-swimlane-routing", "corpus/clean/flowcast/swimlane/16-route-general.drawio", 3, 50, 0.7, Normal},
	{"04-swimlane-nested-merge", "corpus/clean/flowcast/swimlane/42-place-merge-nested.drawio", 2, 40, 0.6, Normal},
	{"05-swimlane-fan-in", "corpus/clean/flowcast/swimlane/28-tracks-fan-in.drawio", 2, 40, 0.5, Normal},
	{"06-swimlane-many-arrows", "corpus/clean/flowcast/swimlane/79-many-arrows-one-node.drawio", 2, 40, 0.6, Normal},
	{"07-snap-rules", "corpus/cases/snap.drawio", 0, 0, 0, Normal},
	{"08-nested-containers", "corpus/cases/nested.drawio", 0, 0, 0, Normal},
	{"09-class-diagram", "corpus/clean/class/02-classes-5.drawio", 0, 0, 0, Normal},
	{"10-hand-sequence", "corpus/clean/sequence/04-hand-3x5.drawio", 0, 0, 0, Normal},
	{"11-architecture", "corpus/clean/architecture/02-tiers-4x3.drawio", 3, 40, 0.5, Normal},
	{"12-state-machine", "corpus/clean/state/02-states-6.drawio", 4, 40, 0.6, Normal},
	{"13-hand-flowchart", "corpus/clean/handflow/01-basic.drawio", 0, 0, 0, Normal},
	{"14-hand-flowchart-library", "corpus/clean/handflow/02-library.drawio", 0, 0, 0, Normal},
	{"15-hand-flowchart-big-font", "corpus/clean/handflow/03-big-font.drawio", 0, 0, 0, Normal},
	{"16-architecture-aggressive", "corpus/clean/architecture/02-tiers-4x3.drawio", 3, 40, 0.5, Aggressive},
	{"17-state-machine-aggressive", "corpus/clean/state/02-states-6.drawio", 4, 40, 0.6, Aggressive},
	{"18-normalize-aggressive", "corpus/cases/normalize.drawio", 0, 0, 0, Aggressive},
	{"19-network-aggressive", "corpus/clean/network/02-lan-8.drawio", 2, 30, 0.6, Aggressive},
	{"20-class-bank", "corpus/clean/class/03-bank.drawio", 0, 0, 0, Normal},
	{"21-class-bank-scattered", "corpus/clean/class/04-bank-scattered.drawio", 0, 0, 0, Normal},
	{"22-class-bank-messified", "corpus/clean/class/03-bank.drawio", 6, 60, 0.7, Normal},
	{"23-sequence-checkout", "corpus/clean/sequence/06-checkout.drawio", 0, 0, 0, Normal},
	{"24-sequence-checkout-scattered", "corpus/clean/sequence/07-checkout-scattered.drawio", 0, 0, 0, Normal},
	{"25-sequence-uml", "corpus/clean/sequence/03-uml-5x10.drawio", 0, 0, 0, Normal},
}

// TestShowcase writes showcase/NAME.before.drawio and NAME.after.drawio for
// each example, plus the full report in showcase/report.txt.
func TestShowcase(t *testing.T) {
	if err := os.MkdirAll(showcaseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var rep strings.Builder
	for _, ex := range examples {
		data, err := os.ReadFile(ex.source)
		if err != nil {
			t.Fatal(err)
		}
		if ex.jitter > 0 {
			d, _, err := doc.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			rng := rand.New(rand.NewSource(ex.seed))
			messify.Strip(d)
			messify.Jitter(d, rng, ex.share, ex.jitter)
			messify.Detach(d, rng, messify.DetachOptions{Share: 0.2, MaxInside: 6, MaxOutside: 6})
			data = d.Bytes()
		}
		res, err := Tidy(data, Options{Level: ex.level})
		if err != nil {
			t.Fatalf("%s: %v", ex.name, err)
		}
		before := filepath.Join(showcaseDir, ex.name+".before.drawio")
		after := filepath.Join(showcaseDir, ex.name+".after.drawio")
		if err := os.WriteFile(before, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(after, res.Output, 0o644); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&rep, "source: %s\n", ex.source)
		rep.WriteString(res.Report.Text(ex.name, true))
		rep.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(showcaseDir, "report.txt"), []byte(rep.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
