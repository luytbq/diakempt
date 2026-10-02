package layout

import (
	"strings"

	"github.com/luytbq/diakempt/engine/unistr"
)

func splitStyle(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = unistr.Strip(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// gk is the grid key of a column: lane first, then column within the lane.
// Compared lexicographically.
type gk struct{ lane, col int }

func (a gk) less(b gk) bool { return a.lane < b.lane || (a.lane == b.lane && a.col < b.col) }

// cell is a cell of the grid.
type cell struct{ lane, col, row int }

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// fmax and fmin return the first argument when the two are equal. math.Max and
// math.Min cannot be used, because math.Max(-0, 0) always returns +0: the result
// would then depend on the sign of zero rather than only on the value, and the
// engine needs deterministic results.
func fmax(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

func fmin(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}
