package num

import "testing"

// The values below sit exactly on halfway points, where naive rounding goes
// wrong:
// 0.125 and 2.675 round down, 0.135 and 0.005 round up, depending on the true
// binary value.
func TestFmtRoundsToTwoDecimals(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{100, "100"},
		{100.5, "100.5"},
		{0.125, "0.12"},
		{0.135, "0.14"},
		{0.005, "0.01"},
		{0.015, "0.01"},
		{611.4, "611.4"},
		{63.99, "63.99"},
		{-11.5, "-11.5"},
		{-0.001, "0"},
		{1e-09, "0"},
		{2.675, "2.67"},
		{1.005, "1"},
	}
	for _, c := range cases {
		if got := Fmt(c.in); got != c.want {
			t.Errorf("Fmt(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRndRoundsUpToNearestEven(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{{0, 0}, {1, 2}, {2, 2}, {2.1, 4}, {49.2, 50}, {50, 50}, {-1, 0}}
	for _, c := range cases {
		if got := Rnd(c.in); got != c.want {
			t.Errorf("Rnd(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
