package layout

import "testing"

// TestCheckAssignsCodeToEachErrorKind hand-builds a broken geometry with every
// kind of error and checks that each kind yields the right code.
func TestCheckAssignsCodeToEachErrorKind(t *testing.T) {
	r := Result{
		LaneX: []float64{0},
		LaneW: []float64{400},
		Items: []PlacedItem{
			{ID: "A", X: 10, Y: 10, W: 100, H: 50},
			{ID: "B", X: 50, Y: 30, W: 100, H: 50},
			{ID: "C", X: 350, Y: 10, W: 100, H: 50},
			{ID: "D", X: 10, Y: 200, W: 100, H: 50},
			{ID: "F", X: 200, Y: 200, W: 100, H: 50},
		},
		Edges: []PlacedEdge{
			{ID: "E1", Src: "D", Dst: "F", Pts: [][2]float64{{110, 225}, {150, 300}}},
			{ID: "E2", Src: "A", Dst: "F", Pts: [][2]float64{{60, 60}, {60, 120}, {250, 120}, {250, 200}}},
			{ID: "E3", Src: "D", Dst: "C", Pts: [][2]float64{{110, 120}, {300, 120}}},
			{ID: "E4", Src: "D", Dst: "A", Pts: [][2]float64{{60, 150}, {60, 250}},
				Label: &[4]float64{0, 0, 40, 40}},
		},
	}
	want := map[string]bool{
		"check.node-overlap":      false,
		"check.outside-lane":      false,
		"check.oblique-segment":   false,
		"check.edge-crosses-node": false,
		"check.edge-overlap":      false,
		"check.label-over-node":   false,
	}
	for _, f := range Check(r) {
		if f.Code == "" {
			t.Errorf("finding without a code: %s", f.Msg)
		}
		if _, ok := want[f.Code]; ok {
			want[f.Code] = true
		}
	}
	for code, seen := range want {
		if !seen {
			t.Errorf("missing finding %s", code)
		}
	}
}
