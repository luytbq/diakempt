// Package detect decides what kind of diagram a segment is, by rule-based
// scoring that can explain itself.
//
// Misclassification costs are asymmetric (docs/design.md): calling an
// architecture diagram a flowchart leads to a full relayout that destroys the
// user's arrangement, while missing a flowchart only costs the general tidy-up.
// So a kind that leads to relayout needs a high score and a clear margin over
// the runner-up; anything less is unknown.
package detect

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/view"
)

// Kinds detected.
const (
	Flowchart = "flowchart"
	Swimlane  = "swimlane"
	Sequence  = "sequence"
	Class     = "class"
	State     = "state"
	ER        = "er"
	Unknown   = "unknown"
)

// Thresholds. High confidence needs both a score and a lead over the next
// kind; only high confidence assigns a kind.
const (
	highScore  = 0.8
	highMargin = 0.25
	medScore   = 0.6
)

// Result is the verdict on one diagram.
type Result struct {
	Kind       string
	Confidence string // high, medium, low
	// Best is the best-scoring kind even when it was not confident enough to
	// be assigned; Scores holds every kind's score.
	Best    string
	Scores  map[string]float64
	Signals []string
}

// Detect scores a diagram against every kind.
func Detect(d *segment.Diagram) Result {
	f := measure(d)
	scores := map[string]float64{
		Flowchart: f.flowScore(false),
		Swimlane:  f.flowScore(true),
		Sequence:  f.sequenceScore(),
		Class:     f.classScore(),
		State:     f.stateScore(),
		ER:        f.erScore(),
	}
	kinds := []string{Flowchart, Swimlane, Sequence, Class, State, ER}
	sort.SliceStable(kinds, func(i, j int) bool { return scores[kinds[i]] > scores[kinds[j]] })
	best, second := kinds[0], scores[kinds[1]]
	r := Result{Best: best, Scores: scores, Signals: f.signals}
	top := scores[best]
	switch {
	case top >= highScore && top-second >= highMargin:
		r.Kind, r.Confidence = best, "high"
	case top >= medScore:
		r.Kind, r.Confidence = Unknown, "medium"
	default:
		r.Kind, r.Confidence = Unknown, "low"
	}
	if r.Kind == Unknown && top >= medScore {
		r.Signals = append(r.Signals, fmt.Sprintf("closest kind %s at %.2f, not confident enough", best, top))
	}
	return r
}

// features are the facts scoring reads.
type features struct {
	leaves, wires  int
	attached       int // wires with both ends on shapes
	directed       int // attached wires with one arrowhead
	undirected     int // attached wires with no arrowhead
	bidirectional  int
	forwardShare   float64 // share of directed wires going the dominant way
	decisions      int
	terminals      int
	flowLibrary    int
	sources, sinks int
	lanes          int
	foreign        []string // shapes or arrows of other notations
	classes        int      // UML class shapes
	umlWires       int      // wires with UML relation ends
	tables         int      // ER tables
	erWires        int      // wires with ER relation ends
	starts, ends   int      // state machine start and final states
	lifelines      int
	handLifelines  int
	messages       int
	nonLaneGroups  int
	signals        []string
}

func (f *features) signal(format string, args ...any) {
	f.signals = append(f.signals, fmt.Sprintf(format, args...))
}

func measure(d *segment.Diagram) *features {
	f := &features{wires: len(d.Wires)}
	in, out := map[*view.Node]int{}, map[*view.Node]int{}
	for _, n := range d.Nodes {
		if !n.Container && !n.TextOnly {
			f.leaves++
		}
		if why := foreignShape(n); why != "" {
			f.foreign = append(f.foreign, why)
		}
		if IsClass(n) {
			f.classes++
		}
		if n.Shape == "table" {
			f.tables++
		}
		switch {
		case n.Shape == "umlLifeline":
			f.lifelines++
		case n.Shape == "startState":
			f.starts++
		case n.Shape == "endState":
			f.ends++
		case strings.HasPrefix(n.Shape, "mxgraph.flowchart."):
			f.flowLibrary++
		}
	}
	var down, up, right, left int
	for _, w := range d.Wires {
		if why := foreignWire(w); why != "" {
			f.foreign = append(f.foreign, why)
			if strings.HasPrefix(why, "UML") {
				f.umlWires++
			}
			if why == "ER relation" {
				f.erWires++
			}
		}
		if w.Src == nil || w.Dst == nil || w.Src.Container || w.Dst.Container {
			continue
		}
		f.attached++
		end := w.Style.Value("endArrow", "classic") != "none"
		start := w.Style.Value("startArrow", "none") != "none"
		switch {
		case end && start:
			f.bidirectional++
		case !end && !start:
			f.undirected++
		default:
			f.directed++
			a, b := w.Src.Box.Center(), w.Dst.Box.Center()
			if start {
				a, b = b, a
			}
			switch {
			case b.Y-a.Y > 5:
				down++
			case a.Y-b.Y > 5:
				up++
			}
			switch {
			case b.X-a.X > 5:
				right++
			case a.X-b.X > 5:
				left++
			}
			out[w.Src]++
			in[w.Dst]++
		}
	}
	if f.directed > 0 {
		f.forwardShare = math.Max(float64(down), float64(right)) / float64(f.directed)
	}
	for _, n := range d.Nodes {
		if n.Container || n.TextOnly {
			continue
		}
		switch {
		case n.Shape == "rhombus" || n.Shape == "mxgraph.flowchart.decision":
			if out[n] >= 2 {
				f.decisions++
			}
		case n.Shape == "ellipse" || n.Shape == "doubleEllipse" || n.Shape == "mxgraph.flowchart.terminator" ||
			n.Shape == "mxgraph.flowchart.start_1" || n.Shape == "mxgraph.flowchart.start_2":
			if in[n] == 0 || out[n] == 0 {
				f.terminals++
			}
		}
		if in[n] == 0 && out[n] > 0 {
			f.sources++
		}
		if out[n] == 0 && in[n] > 0 {
			f.sinks++
		}
	}
	f.lanes, f.nonLaneGroups = lanes(d)
	f.handLifelines, f.messages = sequenceShape(d)
	if f.lifelines > 0 {
		f.messages = umlMessages(d)
	}
	f.signal("%d shapes, %d wires (%d directed, %d undirected, %d two-way)", f.leaves, f.wires, f.directed, f.undirected, f.bidirectional)
	if f.directed > 0 {
		f.signal("%.0f%% of directed wires go the same way", 100*f.forwardShare)
	}
	if f.decisions > 0 {
		f.signal("%s", plural(f.decisions, "decision"))
	}
	if f.terminals > 0 {
		f.signal("%d start or end shapes", f.terminals)
	}
	if f.lanes > 0 {
		f.signal("%s", plural(f.lanes, "lane"))
	}
	if f.nonLaneGroups > 0 {
		f.signal("%s that %s not lanes", plural(f.nonLaneGroups, "container"), map[bool]string{true: "is", false: "are"}[f.nonLaneGroups == 1])
	}
	if f.lifelines+f.handLifelines > 0 {
		f.signal("%d lifelines, %d messages", f.lifelines+f.handLifelines, f.messages)
	}
	if f.classes > 0 {
		f.signal("%s, %d UML relations", plural(f.classes, "class"), f.umlWires)
	}
	if f.tables > 0 {
		f.signal("%s, %d ER relations", plural(f.tables, "table"), f.erWires)
	}
	if f.starts+f.ends > 0 {
		f.signal("%d initial and %d final states", f.starts, f.ends)
	}
	if len(f.foreign) > 0 {
		f.signal("other notation: %s", strings.Join(dedupe(f.foreign), ", "))
	}
	return f
}

// flowScore scores a flowchart; with lanes set, a swimlane activity diagram.
func (f *features) flowScore(withLanes bool) float64 {
	if f.leaves < 3 || f.directed < 2 {
		return 0
	}
	if withLanes != (f.lanes >= 1) {
		return 0
	}
	directedShare := float64(f.directed) / float64(f.attached)
	shapes := 0.0
	if f.decisions > 0 || f.terminals > 0 || f.flowLibrary > 0 {
		shapes = 1
	}
	ends := 0.0
	if f.sources > 0 && f.sinks > 0 {
		ends = 1
	}
	s := 0.35*directedShare + 0.35*f.forwardShare + 0.15*shapes + 0.15*ends
	if len(f.foreign) > 0 || f.lifelines > 0 || f.handLifelines > 0 {
		s *= 0.3
	}
	if f.nonLaneGroups > 0 {
		s *= 0.5
	}
	return s
}

func (f *features) sequenceScore() float64 {
	lines := f.lifelines + f.handLifelines
	if lines < 2 || f.messages < 1 {
		return 0
	}
	others := f.wires - f.handLifelines
	share := 1.0
	if others > 0 {
		share = math.Min(1, float64(f.messages)/float64(others))
	}
	return 0.55 + 0.45*share
}

// classScore scores a UML class diagram: most shapes are classes, and the
// wires between them use UML relation ends.
func (f *features) classScore() float64 {
	if f.classes < 2 || f.leaves == 0 {
		return 0
	}
	share := float64(f.classes) / float64(f.leaves)
	rel := 0.5
	if f.umlWires > 0 {
		rel = 1
	}
	s := 0.6*share + 0.4*rel
	if f.lifelines > 0 || f.handLifelines > 0 || f.lanes > 0 {
		s *= 0.5
	}
	return s
}

// stateScore scores a UML state machine: the editor's initial or final state
// dots, and transitions that are mostly one-way arrows. Shapes of other
// notations, lanes and lifelines push it down.
func (f *features) stateScore() float64 {
	if f.leaves < 3 || f.directed < 2 || f.starts+f.ends == 0 {
		return 0
	}
	directedShare := float64(f.directed) / float64(f.attached)
	both := 0.5
	if f.starts > 0 && f.ends > 0 {
		both = 1
	}
	s := 0.5 + 0.25*directedShare + 0.25*both
	for _, why := range f.foreign {
		if why != "state machine" {
			s *= 0.5
			break
		}
	}
	if f.lanes > 0 || f.lifelines > 0 || f.handLifelines > 0 || f.nonLaneGroups > 0 {
		s *= 0.5
	}
	return s
}

// erScore scores an entity relationship diagram: most shapes are the
// editor's tables, and the wires between them use ER relation ends.
func (f *features) erScore() float64 {
	if f.tables < 2 || f.leaves == 0 {
		return 0
	}
	share := float64(f.tables) / float64(f.leaves)
	rel := 0.5
	if f.erWires > 0 {
		rel = 1
	}
	s := 0.6*share + 0.4*rel
	if f.classes > 0 || f.lifelines > 0 || f.handLifelines > 0 || f.lanes > 0 {
		s *= 0.5
	}
	return s
}

// IsClass reports the editor's UML class shape: a swimlane stacking its rows
// vertically.
func IsClass(n *view.Node) bool {
	st := n.Style
	return n.Shape == "swimlane" && st.Value("childLayout", "") == "stackLayout" && st.Value("horizontalStack", "1") == "0"
}

// foreignShape names shapes of notations that are not flows.
func foreignShape(n *view.Node) string {
	st := n.Style
	switch {
	case n.Shape == "table" || n.Shape == "tableRow":
		return "ER table"
	case n.Shape == "startState" || n.Shape == "endState":
		return "state machine"
	case n.Shape == "swimlane" && st.Value("childLayout", "") == "stackLayout" && st.Value("horizontalStack", "1") == "0":
		return "UML class"
	case strings.HasPrefix(n.Shape, "mxgraph.cisco") || strings.HasPrefix(n.Shape, "mxgraph.aws") ||
		strings.HasPrefix(n.Shape, "mxgraph.azure") || strings.HasPrefix(n.Shape, "mxgraph.gcp") ||
		strings.HasPrefix(n.Shape, "mxgraph.network") || n.Shape == "cloud":
		return "network or cloud icon"
	case st.Value("treeFolding", "0") == "1":
		return "mind map"
	case n.Shape == "umlFrame":
		return "UML"
	}
	return ""
}

// foreignWire names wire styles of notations that are not flows.
func foreignWire(w *view.Wire) string {
	st := w.Style
	for _, k := range []string{"endArrow", "startArrow"} {
		switch v := st.Value(k, ""); {
		case strings.HasPrefix(v, "ER"):
			return "ER relation"
		case strings.HasPrefix(v, "diamond"):
			return "UML association"
		case v == "block" && st.Value("endFill", "1") == "0":
			return "UML inheritance"
		}
	}
	if st.Value("edgeStyle", "") == "entityRelationEdgeStyle" {
		return "entity or mind map link"
	}
	return ""
}

// lanes counts swimlane containers holding flow shapes and arranged as bands:
// lanes of a pool, or sibling swimlanes of equal height or width side by side.
// It also counts other containers holding shapes.
func lanes(d *segment.Diagram) (lanes, others int) {
	var swim []*view.Node
	for _, n := range d.Nodes {
		if !n.Container || len(n.Children) == 0 {
			continue
		}
		hasLeaf := false
		hasLane := false
		for _, c := range n.Children {
			if c.Container && c.Shape == "swimlane" {
				hasLane = true
			} else if !c.Container {
				hasLeaf = true
			}
		}
		switch {
		case n.Shape == "swimlane" && hasLeaf:
			swim = append(swim, n)
		case n.Shape == "swimlane" && hasLane:
			// a pool
		case foreignShape(n) != "":
		default:
			others++
		}
	}
	if len(swim) < 2 {
		return len(swim), others
	}
	if len(swim) >= 2 && swim[0].Parent != nil && swim[0].Parent.Shape == "swimlane" {
		return len(swim), others
	}
	// Bands: siblings lined up edge to edge on one axis with equal extent on the
	// other.
	banded := 0
	for _, a := range swim {
		for _, b := range swim {
			if a == b || a.Parent != b.Parent {
				continue
			}
			if (near(a.Box.Y, b.Box.Y) && near(a.Box.H, b.Box.H)) || (near(a.Box.X, b.Box.X) && near(a.Box.W, b.Box.W)) {
				banded++
				break
			}
		}
	}
	return banded, others
}

func near(a, b float64) bool { return math.Abs(a-b) <= 2 }

// sequenceShape finds hand-drawn lifelines (segment.HandLifeline) and messages (horizontal arrows whose ends
// touch two of them).
func sequenceShape(d *segment.Diagram) (lifelines, messages int) {
	var lines []geom.Segment
	for _, w := range d.Wires {
		var src, dst *geom.Rect
		if w.Src != nil {
			src = &w.Src.Box
		}
		if w.Dst != nil {
			dst = &w.Dst.Box
		}
		x, top, bottom, ok := segment.HandLifeline(w.Style, w.Path, len(w.Points), src, dst)
		if !ok {
			continue
		}
		lines = append(lines, geom.Segment{A: geom.Point{X: x, Y: top}, B: geom.Point{X: x, Y: bottom}})
	}
	if len(lines) < 2 {
		return len(lines), 0
	}
	on := func(p geom.Point) int {
		for i, l := range lines {
			lo, hi := math.Min(l.A.Y, l.B.Y), math.Max(l.A.Y, l.B.Y)
			if math.Abs(p.X-l.A.X) <= 4 && p.Y >= lo-4 && p.Y <= hi+4 {
				return i
			}
		}
		return -1
	}
	for _, w := range d.Wires {
		pl := w.Path
		if len(pl) < 2 {
			continue
		}
		a, b := pl[0], pl[len(pl)-1]
		if math.Abs(a.Y-b.Y) > 4 {
			continue
		}
		if i, j := on(a), on(b); i >= 0 && j >= 0 && i != j {
			messages++
		}
	}
	return len(lines), messages
}

// umlMessages counts wires between UML lifelines, attached to the lifeline
// itself or to an activation bar on it. A self call counts too.
func umlMessages(d *segment.Diagram) int {
	n := 0
	for _, w := range d.Wires {
		if Lifeline(w.Src) != nil && Lifeline(w.Dst) != nil {
			n++
		}
	}
	return n
}

// Lifeline returns the UML lifeline a node is or sits on, or nil.
func Lifeline(n *view.Node) *view.Node {
	for k := 0; n != nil && k < 64; k++ {
		if n.Shape == "umlLifeline" {
			return n
		}
		n = n.Parent
	}
	return nil
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range s {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
