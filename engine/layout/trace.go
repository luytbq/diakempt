package layout

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Trace receives one line per processing step when a caller wants a detailed
// log. The core never prints: the CLI prints these lines under --verbose. A nil
// Trace costs nothing, since every step checks it before building its line.
type Trace func(format string, a ...any)

func (t Trace) on() bool { return t != nil }

// Log sends one line to t; a nil Trace ignores it.
func (t Trace) Log(format string, a ...any) {
	if t != nil {
		t(format, a...)
	}
}

// Since formats the time a step took, for the end of a trace line.
func Since(t0 time.Time) string {
	return fmt.Sprintf("(%.1f ms)", float64(time.Since(t0).Microseconds())/1000)
}

// tracePlace logs the grid after placement: its size, then every item's cell.
func (l *Layout) tracePlace(t0 time.Time) {
	if !l.Trace.on() {
		return
	}
	var cols []string
	for lane, ln := range l.Lanes {
		cols = append(cols, fmt.Sprintf("%s=%d", ln.ID, len(l.Cols[lane])))
	}
	l.Trace.Log("place: %d rows, columns per lane %s, %d items %s",
		l.NRows, strings.Join(cols, " "), len(l.ItemOrder), Since(t0))
	for _, it := range l.ItemOrder {
		what := it.Type
		if it.Attach != "" {
			what += " attached to " + it.Attach
		}
		l.Trace.Log("place:   %s %s: lane %s, row %d, col %d", it.ID, what, l.Lanes[it.Lane].ID, it.Row, it.Col)
	}
}

// traceRoute logs how many edges took each wire style and how crowded the
// busiest gutters and channels are, then every edge's style and exit side.
func (l *Layout) traceRoute(t0 time.Time) {
	if !l.Trace.on() {
		return
	}
	cases := map[byte]int{}
	for _, e := range l.Edges {
		cases[e.Case]++
	}
	busiest, tracks := "", 0
	var keys []Res
	for res := range l.NTracks {
		keys = append(keys, res)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].label() < keys[j].label() })
	for _, res := range keys {
		if n := l.NTracks[res]; n > tracks {
			busiest, tracks = res.label(), n
		}
	}
	l.Trace.Log("route: %d edges: %d straight down (A), %d straight across (B), %d L-shaped (C), %d general (D); %d wire segments; busiest is %s with %d tracks %s",
		len(l.Edges), cases['A'], cases['B'], cases['C'], cases['D'], len(l.Segs), busiest, tracks, Since(t0))
	for _, e := range l.Edges {
		back := ""
		if e.Back {
			back = ", back edge"
		}
		l.Trace.Log("route:   %s %s -> %s: style %c, exits %c%s", e.ID, e.Src, e.Dst, e.Case, e.ExitSide, back)
	}
}

func (l *Layout) traceGeometry(pass int, reserved int, t0 time.Time) {
	l.Trace.Log("geometry: pass %d: pool %.2fx%.2f px, %d gutter or channel reservations for labels %s",
		pass, l.PoolW, l.PoolH, reserved, Since(t0))
}

func (l *Layout) traceLabels(before int, t0 time.Time) {
	if !l.Trace.on() {
		return
	}
	n := 0
	for _, e := range l.Edges {
		if e.Label != nil {
			n++
		}
	}
	l.Trace.Log("labels: placed %d labels, %d could not find a fully free spot %s", n, len(l.Warnings)-before, Since(t0))
}
