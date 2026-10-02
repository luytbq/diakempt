package geom

import "testing"

func TestSegmentCrossesRectOnlyThroughTheInside(t *testing.T) {
	r := Rect{X: 0, Y: 0, W: 100, H: 50}
	for _, tc := range []struct {
		s    Segment
		want bool
	}{
		{Segment{Point{-10, 25}, Point{110, 25}}, true},
		{Segment{Point{50, -10}, Point{50, 10}}, true},
		{Segment{Point{-10, 0}, Point{110, 0}}, false},   // along the top side
		{Segment{Point{50, -10}, Point{50, 0}}, false},   // ends on the outline
		{Segment{Point{-10, 60}, Point{110, 60}}, false}, // below
	} {
		if got := tc.s.CrossesRect(r, 1); got != tc.want {
			t.Errorf("%v: got %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestSegmentIntersectsIgnoresSharedEnds(t *testing.T) {
	a := Segment{Point{0, 0}, Point{10, 10}}
	if !a.Intersects(Segment{Point{0, 10}, Point{10, 0}}) {
		t.Error("an X must intersect")
	}
	if a.Intersects(Segment{Point{10, 10}, Point{20, 0}}) {
		t.Error("a shared end is not a crossing")
	}
}

func TestDistToOutline(t *testing.T) {
	r := Rect{X: 0, Y: 0, W: 100, H: 50}
	if d := r.DistToOutline(Point{50, 10}); d != 10 {
		t.Errorf("inside: %v", d)
	}
	if d := r.DistToOutline(Point{103, 54}); d != 5 {
		t.Errorf("outside corner: %v", d)
	}
	if d := r.Dist(Point{50, 10}); d != 0 {
		t.Errorf("Dist inside: %v", d)
	}
}

func TestFormatRoundsToTwoDecimals(t *testing.T) {
	for in, want := range map[float64]string{1: "1", 1.005: "1", 2.346: "2.35", -0.001: "0", 120.5: "120.5"} {
		if got := Format(in); got != want {
			t.Errorf("Format(%v) = %q, want %q", in, got, want)
		}
	}
}
