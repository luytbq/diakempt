package layout

import "testing"

// Orient must transform everything that carries coordinates: wire points,
// element boxes, label boxes, anchor points and label offsets. Missing one still
// yields a drawable diagram, but part of it ends up in the wrong place.
func TestOrientTransformsEverythingWithCoordinates(t *testing.T) {
	label := [4]float64{10, 100, 40, 120}
	base := Result{
		PoolW: 200, PoolH: 400, PoolHeader: 30, LaneHeader: 30,
		Items: []PlacedItem{{ID: "a", X: 10, Y: 100, W: 30, H: 20}},
		Edges: []PlacedEdge{{ID: "e", Pts: [][2]float64{{20, 120}, {20, 200}},
			ExitFrac: [2]float64{0.5, 1}, EntryFrac: [2]float64{0.5, 0},
			Label: &label, LabelOff: [2]float64{5, -7}}},
	}
	cases := map[string]struct {
		item  [4]float64 // x, y, w, h
		pt0   [2]float64
		exit  [2]float64
		off   [2]float64
		poolW float64
	}{
		DirBT: {[4]float64{10, 340, 30, 20}, [2]float64{20, 340}, [2]float64{0.5, 0}, [2]float64{5, 7}, 200},
		DirLR: {[4]float64{100, 10, 20, 30}, [2]float64{120, 20}, [2]float64{1, 0.5}, [2]float64{-7, 5}, 400},
		DirRL: {[4]float64{340, 10, 20, 30}, [2]float64{340, 20}, [2]float64{0, 0.5}, [2]float64{7, 5}, 400},
	}
	for dir, want := range cases {
		r := base
		r.Dir = dir
		got := orientResult(r)
		it := got.Items[0]
		if [4]float64{it.X, it.Y, it.W, it.H} != want.item {
			t.Errorf("%s: element %v, want %v", dir, [4]float64{it.X, it.Y, it.W, it.H}, want.item)
		}
		e := got.Edges[0]
		if e.Pts[0] != want.pt0 {
			t.Errorf("%s: first point %v, want %v", dir, e.Pts[0], want.pt0)
		}
		if e.ExitFrac != want.exit {
			t.Errorf("%s: exit anchor %v, want %v", dir, e.ExitFrac, want.exit)
		}
		if e.LabelOff != want.off {
			t.Errorf("%s: label offset %v, want %v", dir, e.LabelOff, want.off)
		}
		if got.PoolW != want.poolW {
			t.Errorf("%s: pool width %v, want %v", dir, got.PoolW, want.poolW)
		}
		// The label box must move with the wire, not keep its old coordinates.
		if *e.Label == label {
			t.Errorf("%s: label box was not axis-swapped", dir)
		}
		b := *e.Label
		if b[0] > b[2] || b[1] > b[3] {
			t.Errorf("%s: label box inverted %v", dir, b)
		}
		if base.Edges[0].Pts[0] != [2]float64{20, 120} || *base.Edges[0].Label != label {
			t.Errorf("%s: Orient modified the original result", dir)
		}
	}
	if r := orientResult(base); r.Edges[0].Pts[0] != [2]float64{20, 120} {
		t.Error("an empty direction must return the result unchanged")
	}
}

func TestValidateConfigRejectsBothBounds(t *testing.T) {
	c := DefaultConfig()
	if err := ValidateConfig(c); err != nil {
		t.Fatalf("the default config must be valid: %v", err)
	}
	for _, f := range Fields() {
		for _, v := range []int{f.Lo - 1, f.Hi + 1} {
			bad := DefaultConfig()
			*f.Get(&bad) = v
			if err := ValidateConfig(bad); err == nil {
				t.Errorf("%s = %d is outside the range %d..%d but was not rejected", f.Name, v, f.Lo, f.Hi)
			}
		}
	}
}

// Fields returns a copy: a caller modifying the list must not corrupt the
// process's default config.
func TestFieldsReturnsCopy(t *testing.T) {
	fs := Fields()
	if len(fs) == 0 {
		t.Fatal("no fields")
	}
	fs[0].Default, fs[0].Name = -1, "broken"
	again := Fields()
	if again[0].Name == "broken" || again[0].Default == -1 {
		t.Error("Fields returned the internal slice itself")
	}
	if *again[0].Get(&Config{}) != 0 {
		t.Error("Get must point into the Config passed in")
	}
}
