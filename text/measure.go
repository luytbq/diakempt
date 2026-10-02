package text

import (
	"math"
	"strings"
)

// breakChars are the characters after which a long word may be broken; a break
// is also allowed after "::".
const breakChars = "/._,(){}=&?-"

const (
	DefaultSize  = 12.0
	DefaultLineH = 15.0
)

// Measure wraps lines and measures text boxes.
type Measure struct {
	m     *Metrics
	Size  float64
	LineH float64
	hard  bool
}

func NewMeasure(m *Metrics) *Measure {
	return &Measure{m: m, Size: DefaultSize, LineH: DefaultLineH}
}

func (t *Measure) W(s string) float64 { return t.m.Width(s, t.Size) }

// Hard reports whether the most recent wrap had to cut in the middle of a word.
//
// This is a scratch flag, reset per line inside Wrap. Reading it once after
// wrapping several lines reflects the last line, not the whole batch.
func (t *Measure) Hard() bool { return t.hard }

// Box returns the width of the longest line and the total height.
func (t *Measure) Box(lines []string) (w, h float64) {
	for _, l := range lines {
		if x := t.W(l); x > w {
			w = x
		}
	}
	return w, float64(float64(len(lines)) * t.LineH)
}

// breakAfter reports whether a break is allowed right before position i. Call with 1 <= i <= len(r).
func breakAfter(r []rune, i int) bool {
	if strings.ContainsRune(breakChars, r[i-1]) {
		return true
	}
	return i >= 2 && r[i-1] == ':' && r[i-2] == ':'
}

// breakSplit cuts a word at every allowed break point, dropping empty pieces.
func breakSplit(word string) []string {
	r := []rune(word)
	var out []string
	start := 0
	for i := 1; i <= len(r); i++ {
		if breakAfter(r, i) {
			if i > start {
				out = append(out, string(r[start:i]))
			}
			start = i
		}
	}
	if start < len(r) {
		out = append(out, string(r[start:]))
	}
	return out
}

// splitLong cuts a word wider than maxw, preferring allowed break points; when
// none is left it hard-cuts mid-word and sets the hard flag.
func (t *Measure) splitLong(word string, maxw float64) []string {
	chunks := breakSplit(word)
	var out []string
	cur := ""
	for _, c := range chunks {
		if t.W(cur+c) <= maxw {
			cur += c
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		cur = ""
		for t.W(c) > maxw {
			t.hard = true
			rc := []rune(c)
			k := len(rc)
			for k > 1 && t.W(string(rc[:k])) > maxw {
				k--
			}
			out = append(out, string(rc[:k]))
			c = string(rc[k:])
		}
		cur = c
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (t *Measure) wrapLine(line string, maxw float64) []string {
	line = strings.TrimSpace(line)
	if t.W(line) <= maxw {
		return []string{line}
	}
	var out []string
	cur := ""
	for _, word := range strings.Fields(line) {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if t.W(cand) <= maxw {
			cur = cand
			continue
		}
		if cur != "" {
			out = append(out, cur)
		}
		if t.W(word) <= maxw {
			cur = word
			continue
		}
		parts := t.splitLong(word, maxw)
		out = append(out, parts[:len(parts)-1]...)
		cur = parts[len(parts)-1]
	}
	return append(out, cur)
}

// Wrap wraps lines at width maxw, then narrows to the smallest width that keeps
// the same line count, so lines end up about equally long instead of one full
// line and one stub.
func (t *Measure) Wrap(lines []string, maxw float64) []string {
	var out []string
	for _, line := range lines {
		t.hard = false
		first := t.wrapLine(line, maxw)
		if len(first) == 1 || t.hard {
			out = append(out, first...)
			continue
		}
		lo, hi := 1, int(math.Ceil(maxw))
		for lo < hi {
			mid := (lo + hi) / 2
			t.hard = false
			if len(t.wrapLine(line, float64(mid))) <= len(first) && !t.hard {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		out = append(out, t.wrapLine(line, float64(hi))...)
	}
	return out
}
