// Package num holds the two number normalization rules shared by every layer.
//
// They determine every byte of the generated .drawio file, so changing them
// changes every golden.
package num

import (
	"math"
	"strconv"
	"strings"
)

// Fmt produces the number form used in .drawio files: two decimal places, then
// trailing zeros and a trailing dot are trimmed. A negative number that rounds
// to zero gives "0", not "-0".
func Fmt(v float64) string {
	// Coordinates read from an old .drawio file can be inf or nan.
	switch {
	case math.IsNaN(v):
		return "nan"
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	}
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "-0" {
		return "0"
	}
	return s
}

// Rnd rounds up to the nearest even integer. Every element width and height
// goes through here, so coordinates always land on an even grid.
func Rnd(v float64) int {
	return int(math.Ceil(v/2.0) * 2)
}

// Round rounds to d decimal places, based on the true binary value of x.
//
// math.Round(x*100)/100 is not used because it is wrong in two places. The
// multiplication rounds once more, so 2.675 (really 2.67499...) becomes 2.68.
// And math.Round pushes exact halves away from zero, so 0.125 becomes 0.13.
// Going through a correctly rounded decimal string gives 2.67 and 0.12 (exact
// halves to even), and Round and Fmt always agree with each other. Verified
// against conformance/round-vectors.json.
func Round(x float64, d int) float64 {
	v, err := strconv.ParseFloat(strconv.FormatFloat(x, 'f', d, 64), 64)
	if err != nil {
		panic(err)
	}
	return v
}
