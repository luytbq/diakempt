package layout

import (
	"reflect"
	"testing"
)

// jogResult is one lane with a source S at the top right, a blocker B, and a
// target T below it, joined by a wire that detours through a gutter track when
// it could go straight down under S.
func jogResult() Result {
	return Result{
		PoolW: 600, PoolH: 400, PoolHeader: 30, LaneHeader: 30,
		LaneX: []float64{0}, LaneW: []float64{600},
		Items: []PlacedItem{
			{ID: "S", X: 400, Y: 80, W: 100, H: 40},
			{ID: "B", X: 100, Y: 160, W: 100, H: 40},
			{ID: "T", X: 100, Y: 300, W: 100, H: 40},
		},
		Edges: []PlacedEdge{{ID: "E", Src: "S", Dst: "T",
			Pts: [][2]float64{{450, 120}, {450, 140}, {300, 140}, {300, 260}, {150, 260}, {150, 300}}}},
	}
}

// wiresOnly runs just the straightening move, so a test can pin it down without
// the item moves also rearranging the diagram.
func wiresOnly(r Result) Result {
	cfg := DefaultConfig()
	cfg.Optimize = 5
	o := newOptimizer(r, cfg)
	for i := range o.r.Edges {
		for o.straighten(i, 1) {
		}
	}
	return o.r
}

func TestOptimizeStraightensGutterDetour(t *testing.T) {
	got := wiresOnly(jogResult()).Edges[0].Pts
	want := [][2]float64{{450, 120}, {450, 260}, {150, 260}, {150, 300}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("path after optimize %v, want %v", got, want)
	}
}

func TestOptimizeOffChangesNothing(t *testing.T) {
	r := jogResult()
	cfg := DefaultConfig()
	cfg.Optimize = 0
	if got := optimize(r, cfg, nil); !reflect.DeepEqual(got, r) {
		t.Errorf("optimize = 0 still changed the result")
	}
}

func TestOptimizeNeverRoutesThroughAnItem(t *testing.T) {
	r := jogResult()
	// An item right under the source blocks the straight path down.
	r.Items = append(r.Items, PlacedItem{ID: "X", X: 420, Y: 200, W: 60, H: 30})
	got := wiresOnly(r).Edges[0].Pts
	if !reflect.DeepEqual(got, r.Edges[0].Pts) {
		t.Errorf("wire pulled through an item: %v", got)
	}
}

func TestOptimizeReanchorsLabelInPlace(t *testing.T) {
	r := jogResult()
	lb := [4]float64{160, 262, 200, 280}
	r.Edges[0].Label = &lb
	r.Edges[0].LabelOff = [2]float64{30, 11}
	e := wiresOnly(r).Edges[0]
	if *e.Label != lb {
		t.Fatalf("label box moved: %v", *e.Label)
	}
	// The label centre (180, 271) projects onto the horizontal run y = 260 at x = 180.
	if e.LabelOff != [2]float64{0, 11} {
		t.Errorf("label offset %v, want [0 11]", e.LabelOff)
	}
}

// TestOptimizeRejectsStraighteningThatAddsACrossing: removing two bends is worth
// less than one new crossing, so the detour stays.
func TestOptimizeRejectsStraighteningThatAddsACrossing(t *testing.T) {
	r := jogResult()
	r.Edges = append(r.Edges, PlacedEdge{ID: "F", Src: "P", Dst: "Q",
		Pts: [][2]float64{{420, 200}, {520, 200}}})
	got := wiresOnly(r).Edges[0].Pts
	if !reflect.DeepEqual(got, r.Edges[0].Pts) {
		t.Errorf("straightened across another wire: %v", got)
	}
}

// rowResult is one lane with two nodes on the same row joined by a straight
// wire, the target far to the right, and a note attached to the source at the
// attach gap.
func rowResult() Result {
	return Result{
		PoolW: 1000, PoolH: 300, PoolHeader: 30, LaneHeader: 30,
		LaneX: []float64{0}, LaneW: []float64{1000}, LaneMinW: []float64{120},
		Items: []PlacedItem{
			{ID: "A", X: 100, Y: 100, W: 100, H: 40},
			{ID: "B", X: 800, Y: 100, W: 100, H: 40},
			{ID: "N", Attach: "A", X: 140, Y: 180, W: 80, H: 30},
		},
		Edges: []PlacedEdge{{ID: "E", Src: "A", Dst: "B", Pts: [][2]float64{{200, 120}, {800, 120}}}},
	}
}

func TestOptimizePullsNodeInAndNarrowsLane(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Optimize = 10
	r := optimize(rowResult(), cfg, nil)
	p := r.Edges[0].Pts
	if l := p[1][0] - p[0][0]; l != float64(cfg.MinGutter) {
		t.Errorf("wire is %v px long, want the minimum gap %d", l, cfg.MinGutter)
	}
	if r.LaneW[0] >= 1000 || r.PoolW != r.LaneW[0] {
		t.Errorf("lane %v, pool %v: lane not narrowed or pool out of step", r.LaneW[0], r.PoolW)
	}
	for _, it := range r.Items {
		if it.X < r.LaneX[0] || it.X+it.W > r.LaneX[0]+r.LaneW[0] {
			t.Errorf("%s left its lane", it.ID)
		}
	}
}

func TestOptimizeKeepsNotesAtAttachGap(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Optimize = 10
	r := rowResult()
	r.Items[2] = PlacedItem{ID: "N", Attach: "A", X: 240, Y: 100, W: 80, H: 30}
	r.Items[1].Y = 200
	r.Edges[0].Pts = [][2]float64{{150, 140}, {150, 220}, {800, 220}}
	out := optimize(r, cfg, nil)
	a, n := out.Items[0], out.Items[2]
	if gap := n.X - (a.X + a.W); gap != float64(cfg.AttachGap) {
		t.Errorf("note sits %v px from its node, want --attach-gap %d", gap, cfg.AttachGap)
	}
}

func TestOptimizeNeverNarrowsLaneBelowMinimum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Optimize = 10
	r := rowResult()
	r.LaneMinW = []float64{950}
	out := optimize(r, cfg, nil)
	if out.LaneW[0] < 950 {
		t.Errorf("lane narrowed to %v, below its minimum 950", out.LaneW[0])
	}
}
