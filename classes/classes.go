// Package classes lays out UML class diagrams the way they are usually read:
// superclasses and interfaces above the classes that extend or implement
// them, a whole above its parts, siblings side by side under their parent, and
// the generalization arrows of one parent joining into a shared trunk that
// enters it from below. Other relations are routed around the classes.
//
// Classes keep their size: it comes from their attribute and method rows.
package classes

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
)

// Spacing, in pixels. The vertical gap leaves room for the trunks of
// generalization arrows between ranks.
const (
	gapX   = 60
	gapY   = 90
	sweeps = 6 // ordering passes
)

// Unsupported explains why a diagram cannot be laid out as a class diagram.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

// relation is how a wire constrains the layout.
type relation int

const (
	// free relations (association, dependency) do not decide ranks.
	free relation = iota
	// generalization and realization: the target is the parent, drawn above.
	generalization
	// composition and aggregation: the diamond end is the whole, drawn above.
	whole
	// dependency (a dashed open arrow): the client, at the tail, is drawn
	// above the supplier it uses.
	uses
)

type link struct {
	w        *tidy.Wire
	rel      relation
	up, down *tidy.Node // for ranked relations: the one drawn above, below
}

// Layout places the classes and routes the relations of a working copy.
func Layout(d *tidy.Diagram) error {
	nodes, links, err := collect(d, "classes", classify)
	if err != nil {
		return err
	}
	rank := ranks(nodes, links)
	layers := order(nodes, links, rank)
	place(layers, links, rank, gaps(layers, links, rank))
	keepCorner(d, nodes)
	for _, l := range route(layers, links, rank) {
		d.RouteWire(l.w)
	}
	return nil
}

// collect returns the shapes to place and the relations between them, or why
// the diagram cannot be laid out. noun names the shapes in the reasons.
func collect(d *tidy.Diagram, noun string, classify func(*tidy.Wire) *link) ([]*tidy.Node, []*link, error) {
	var nodes []*tidy.Node
	for _, n := range d.Nodes {
		if n.Container {
			return nil, nil, &Unsupported{noun + " inside a container"}
		}
		if !n.Text {
			nodes = append(nodes, n)
		}
	}
	if len(nodes) < 2 {
		return nil, nil, &Unsupported{"fewer than two " + noun}
	}
	var links []*link
	for _, w := range d.Wires {
		if w.Src == nil || w.Dst == nil {
			return nil, nil, &Unsupported{fmt.Sprintf("wire %s has a free end", w.V.Cell.ID)}
		}
		if w.Src.Text || w.Dst.Text {
			return nil, nil, &Unsupported{fmt.Sprintf("wire %s ends on a text", w.V.Cell.ID)}
		}
		links = append(links, classify(w))
	}
	return nodes, links, nil
}

// classify reads a wire's relation from its arrowheads, as the editor's UML
// shapes draw them.
func classify(w *tidy.Wire) *link {
	l := &link{w: w}
	end := w.Style.Value("endArrow", "classic")
	start := w.Style.Value("startArrow", "none")
	hollow := func(fill string) bool { return w.Style.Value(fill, "1") == "0" }
	switch {
	case end == "block" && hollow("endFill"):
		l.rel, l.up, l.down = generalization, w.Dst, w.Src
	case start == "block" && hollow("startFill"):
		l.rel, l.up, l.down = generalization, w.Src, w.Dst
	case strings.HasPrefix(start, "diamond"):
		l.rel, l.up, l.down = whole, w.Src, w.Dst
	case strings.HasPrefix(end, "diamond"):
		l.rel, l.up, l.down = whole, w.Dst, w.Src
	case w.Style.Value("dashed", "0") == "1" && start == "none" && (end == "open" || end == "classic"):
		l.rel, l.up, l.down = uses, w.Src, w.Dst
	}
	if l.up == l.down {
		l.rel = free
	}
	return l
}

// ranks assigns each class a rank from the top: generalizations first, then
// whole-part relations where they do not contradict them, each class one
// rank below the lowest class it hangs from. Classes with no ranked relation
// sit in the rank of their median neighbor.
func ranks(nodes []*tidy.Node, links []*link) map[*tidy.Node]int {
	below := map[*tidy.Node][]*tidy.Node{}
	reaches := func(from, to *tidy.Node) bool {
		seen := map[*tidy.Node]bool{}
		stack := []*tidy.Node{from}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n == to {
				return true
			}
			if seen[n] {
				continue
			}
			seen[n] = true
			stack = append(stack, below[n]...)
		}
		return false
	}
	if !anyRanked(links) {
		return drawnRanks(nodes)
	}
	ranked := map[*tidy.Node]bool{}
	for _, rel := range []relation{generalization, whole, uses} {
		for _, l := range links {
			if l.rel != rel {
				continue
			}
			if reaches(l.down, l.up) {
				// It would close a cycle; it stays unranked.
				l.rel = free
				continue
			}
			below[l.up] = append(below[l.up], l.down)
			ranked[l.up], ranked[l.down] = true, true
		}
	}
	rank := map[*tidy.Node]int{}
	var visit func(n *tidy.Node, r int)
	visit = func(n *tidy.Node, r int) {
		if old, ok := rank[n]; ok && old >= r {
			return
		}
		rank[n] = r
		for _, m := range below[n] {
			visit(m, r+1)
		}
	}
	above := map[*tidy.Node]int{}
	for _, ms := range below {
		for _, m := range ms {
			above[m]++
		}
	}
	for _, n := range nodes {
		if ranked[n] && above[n] == 0 {
			visit(n, 0)
		}
	}
	for _, n := range nodes {
		if ranked[n] {
			continue
		}
		var rs []int
		for _, l := range links {
			if o := other(l.w, n); o != nil && ranked[o] {
				rs = append(rs, rank[o])
			}
		}
		if len(rs) > 0 {
			sort.Ints(rs)
			rank[n] = rs[len(rs)/2]
		} else {
			rank[n] = 0
		}
	}
	// Close gaps left by empty ranks.
	return dense(rank)
}

// sideRoom is the extra room two neighbors in a rank need for the labels of
// the wires between them, which run side to side: role names and
// multiplicities near each end, and the wire's own label. A label at a share
// of the wire's length from its nearer end needs the wire long enough to keep
// it off the class there, and the labels together need it long enough to
// stand side by side.
func sideRoom(a, b *tidy.Node, links []*link) float64 {
	need := 0.0
	for _, l := range links {
		if !(l.w.Src == a && l.w.Dst == b) && !(l.w.Src == b && l.w.Dst == a) {
			continue
		}
		sum := float64(labelPad)
		for _, lb := range l.w.LabelSpans() {
			sum += lb.W + labelPad
			share := (1 - math.Abs(lb.Pos)) / 2
			if share > 0.01 {
				need = math.Max(need, math.Min((lb.W/2+labelPad)/share, maxSideRoom))
			}
		}
		need = math.Max(need, sum)
	}
	return math.Max(0, need-gapX)
}

// labelPad keeps labels apart and off classes; maxSideRoom bounds the room a
// label very close to an end can ask for.
const (
	labelPad    = 6
	maxSideRoom = 400
)

func anyRanked(links []*link) bool {
	for _, l := range links {
		if l.rel != free {
			return true
		}
	}
	return false
}

// analysisRanks are the rows of an analysis class diagram, by stereotype:
// boundaries on top, controls under them, entities at the bottom.
var analysisRanks = map[string]int{"boundary": 0, "control": 1, "entity": 2}

// drawnRanks ranks classes joined only by associations and dependencies,
// which say nothing about what goes above what. When every class carries an
// analysis stereotype and they are not all the same, those decide; otherwise
// the rows the classes were drawn in do.
func drawnRanks(nodes []*tidy.Node) map[*tidy.Node]int {
	rank := map[*tidy.Node]int{}
	for _, n := range nodes {
		r, ok := analysisRanks[detect.Stereotype(n.V.Text)]
		if !ok {
			rank = nil
			break
		}
		rank[n] = r
	}
	kinds := map[int]bool{}
	for _, r := range rank {
		kinds[r] = true
	}
	if len(kinds) > 1 {
		return dense(rank)
	}
	rank = map[*tidy.Node]int{}
	sorted := append([]*tidy.Node(nil), nodes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Box.Y < sorted[j].Box.Y })
	r, bottom := 0, math.Inf(-1)
	for i, n := range sorted {
		// A class starting below the middle of the row so far opens a new row.
		if i > 0 && n.Box.Y > bottom {
			r++
		}
		if i == 0 || n.Box.Y > bottom {
			bottom = n.Box.Center().Y
		}
		rank[n] = r
	}
	return rank
}

// dense renumbers ranks 0, 1, 2, ... keeping their order.
func dense(rank map[*tidy.Node]int) map[*tidy.Node]int {
	used := map[int]bool{}
	for _, r := range rank {
		used[r] = true
	}
	var levels []int
	for r := range used {
		levels = append(levels, r)
	}
	sort.Ints(levels)
	to := map[int]int{}
	for i, r := range levels {
		to[r] = i
	}
	for n, r := range rank {
		rank[n] = to[r]
	}
	return rank
}

func other(w *tidy.Wire, n *tidy.Node) *tidy.Node {
	switch n {
	case w.Src:
		return w.Dst
	case w.Dst:
		return w.Src
	}
	return nil
}

// order lists each rank's classes left to right: first as drawn, then by the
// average position of their neighbors in the rank above and below, a few
// sweeps down and up, which untangles most crossings.
func order(nodes []*tidy.Node, links []*link, rank map[*tidy.Node]int) [][]*tidy.Node {
	max := 0
	for _, r := range rank {
		if r > max {
			max = r
		}
	}
	layers := make([][]*tidy.Node, max+1)
	for _, n := range nodes {
		layers[rank[n]] = append(layers[rank[n]], n)
	}
	for _, l := range layers {
		sort.SliceStable(l, func(i, j int) bool { return l[i].Box.Center().X < l[j].Box.Center().X })
	}
	pos := func() map[*tidy.Node]float64 {
		p := map[*tidy.Node]float64{}
		for _, l := range layers {
			for i, n := range l {
				p[n] = float64(i)
			}
		}
		return p
	}
	for s := 0; s < sweeps; s++ {
		down := s%2 == 0
		for k := range layers {
			r := k
			if !down {
				r = len(layers) - 1 - k
			}
			p := pos()
			key := map[*tidy.Node]float64{}
			for _, n := range layers[r] {
				sum, cnt := 0.0, 0
				for _, l := range links {
					o := other(l.w, n)
					if o == nil {
						continue
					}
					want := r - 1
					if !down {
						want = r + 1
					}
					if rank[o] == want {
						sum += p[o]
						cnt++
					}
				}
				if cnt > 0 {
					key[n] = sum / float64(cnt)
				} else {
					key[n] = p[n]
				}
			}
			l := layers[r]
			sort.SliceStable(l, func(i, j int) bool { return key[l[i]] < key[l[j]] })
		}
	}
	return layers
}

// place sets coordinates: ranks stacked top to bottom with their tops
// aligned, and each class centered under the classes it hangs from as far as
// its neighbors allow.
func place(layers [][]*tidy.Node, links []*link, rank map[*tidy.Node]int, gaps []float64) {
	y := 0.0
	for r, l := range layers {
		x := 0.0
		h := 0.0
		for i, n := range l {
			n.Box.X, n.Box.Y = x, y
			x += n.Box.W + gapX
			if i+1 < len(l) {
				x += sideRoom(n, l[i+1], links)
			}
			h = math.Max(h, n.Box.H)
		}
		gap := float64(gapY)
		if r < len(gaps) {
			gap = gaps[r]
		}
		y += h + gap
	}
	parents := func(n *tidy.Node) []*tidy.Node {
		var out []*tidy.Node
		for _, l := range links {
			if o := other(l.w, n); o != nil && rank[o] == rank[n]-1 {
				out = append(out, o)
			}
		}
		return out
	}
	for r := 1; r < len(layers); r++ {
		l := layers[r]
		desired := make([]float64, len(l))
		for i, n := range l {
			ps := parents(n)
			if len(ps) == 0 {
				desired[i] = math.NaN()
				continue
			}
			sum := 0.0
			for _, p := range ps {
				sum += p.Box.Center().X
			}
			desired[i] = sum / float64(len(ps))
		}
		// Left to right, each class goes where it wants or just right of its
		// neighbor; then the rank shifts back by the average overshoot.
		prev := math.Inf(-1)
		over, cnt := 0.0, 0
		for i, n := range l {
			x := prev
			if !math.IsNaN(desired[i]) {
				x = math.Max(prev, desired[i]-n.Box.W/2)
			} else if math.IsInf(prev, -1) {
				x = n.Box.X
			}
			n.Box.X = x
			prev = x + n.Box.W + gapX
			if i+1 < len(l) {
				prev += sideRoom(n, l[i+1], links)
			}
			if !math.IsNaN(desired[i]) {
				over += n.Box.Center().X - desired[i]
				cnt++
			}
		}
		if cnt > 0 {
			shift := -over / float64(cnt)
			for _, n := range l {
				n.Box.X += shift
			}
		}
	}
}

// keepCorner moves the laid-out classes so the diagram's top-left corner is
// where it was, and returns how far it moved them.
func keepCorner(d *tidy.Diagram, nodes []*tidy.Node) geom.Point {
	var was, now geom.Rect
	for _, n := range nodes {
		was = was.Union(n.Orig)
		now = now.Union(n.Box)
	}
	by := geom.Point{X: was.X - now.X, Y: was.Y - now.Y}
	for _, n := range nodes {
		n.Box = n.Box.Move(by)
	}
	for _, n := range d.Nodes {
		if n.Text {
			// Free notes keep their place relative to the nearest class.
			n.Box = n.Box.Move(nearestShift(n, nodes))
		}
	}
	return by
}

func nearestShift(t *tidy.Node, nodes []*tidy.Node) geom.Point {
	var best *tidy.Node
	bd := math.Inf(1)
	for _, n := range nodes {
		if d := t.Orig.Center().Dist(n.Orig.Center()); d < bd {
			best, bd = n, d
		}
	}
	if best == nil {
		return geom.Point{}
	}
	return geom.Point{X: best.Box.X - best.Orig.X, Y: best.Box.Y - best.Orig.Y}
}

// Channel spacing, in pixels: the room above and below the levels in a gap,
// and between two levels.
const (
	channelPad  = 20
	channelStep = 12
)

// channel reports whether a link runs through the gap between two adjacent
// ranks on a level of its own, and trunk whether it joins a trunk there.
func channel(l *link, rank map[*tidy.Node]int) bool {
	a, b := rank[l.w.Src], rank[l.w.Dst]
	return (a-b == 1 || b-a == 1) && !trunk(l, rank)
}

func trunk(l *link, rank map[*tidy.Node]int) bool {
	return l.rel == generalization && rank[l.down] == rank[l.up]+1
}

// gaps returns the height of each gap between ranks: enough for its trunks
// and channels.
func gaps(layers [][]*tidy.Node, links []*link, rank map[*tidy.Node]int) []float64 {
	if len(layers) < 2 {
		return nil
	}
	levels := make([]int, len(layers)-1)
	parents := map[*tidy.Node]bool{}
	for _, l := range links {
		switch {
		case trunk(l, rank):
			if !parents[l.up] {
				parents[l.up] = true
				levels[rank[l.up]]++
			}
		case channel(l, rank):
			levels[min(rank[l.w.Src], rank[l.w.Dst])]++
		}
	}
	out := make([]float64, len(levels))
	for i, n := range levels {
		out[i] = math.Max(gapY, 2*channelPad+float64(max(n-1, 0))*channelStep)
	}
	return out
}

// route draws the wires between adjacent ranks through the gap between
// them. Generalizations become trunks, one per parent: from the top of each
// child up to a shared level, across to the parent, and up into its bottom.
// Every other wire between adjacent ranks gets a level of its own, leaving
// the upper class from its bottom and entering the lower one from its top.
// Levels are ordered and ports spread along each side to avoid crossings.
// It returns the wires within a rank or across several, for the caller to
// route around the classes afterwards, so they see the rest.
func route(layers [][]*tidy.Node, links []*link, rank map[*tidy.Node]int) []*link {
	n := len(layers)
	top, bottom := make([]float64, n), make([]float64, n)
	for r, l := range layers {
		top[r], bottom[r] = math.Inf(1), math.Inf(-1)
		for _, c := range l {
			top[r] = math.Min(top[r], c.Box.Y)
			bottom[r] = math.Max(bottom[r], c.Box.Bottom())
		}
	}
	// A run is what crosses a gap: a trunk (one parent and its children) or
	// one channel wire.
	type run struct {
		links      []*link
		upper      *tidy.Node
		lowers     []*tidy.Node
		gap        int
		level      float64
		upX        float64 // where it meets the upper class
		downX      []float64
		upperFrac  float64
		lowerFracs []float64
	}
	var runs []*run
	byParent := map[*tidy.Node]*run{}
	var rest []*link
	for _, l := range links {
		switch {
		case l.w.OnPart():
			rest = append(rest, l)
		case trunk(l, rank):
			r := byParent[l.up]
			if r == nil {
				r = &run{upper: l.up, gap: rank[l.up]}
				byParent[l.up] = r
				runs = append(runs, r)
			}
			r.links = append(r.links, l)
			r.lowers = append(r.lowers, l.down)
		case channel(l, rank):
			up, down := l.w.Src, l.w.Dst
			if rank[up] > rank[down] {
				up, down = down, up
			}
			runs = append(runs, &run{links: []*link{l}, upper: up, lowers: []*tidy.Node{down}, gap: rank[up]})
		default:
			rest = append(rest, l)
		}
	}
	// Ports: the ends on each side of a class, in the order of the classes
	// they lead to, spread evenly along it.
	type end struct {
		r   *run
		k   int // index into r.lowers, or -1 for the upper end
		key float64
	}
	sides := map[*tidy.Node][]*end{} // bottom sides
	tops := map[*tidy.Node][]*end{}
	for _, r := range runs {
		sum := 0.0
		for _, c := range r.lowers {
			sum += c.Box.Center().X
		}
		sides[r.upper] = append(sides[r.upper], &end{r: r, k: -1, key: sum / float64(len(r.lowers))})
		r.lowerFracs = make([]float64, len(r.lowers))
		r.downX = make([]float64, len(r.lowers))
		for k, c := range r.lowers {
			tops[c] = append(tops[c], &end{r: r, k: k, key: r.upper.Box.Center().X})
		}
	}
	spread := func(m map[*tidy.Node][]*end, set func(e *end, frac, x float64)) {
		for c, es := range m {
			sort.SliceStable(es, func(i, j int) bool { return es[i].key < es[j].key })
			for i, e := range es {
				f := float64(i+1) / float64(len(es)+1)
				set(e, f, c.Box.X+f*c.Box.W)
			}
		}
	}
	spread(sides, func(e *end, f, x float64) { e.r.upperFrac, e.r.upX = f, x })
	spread(tops, func(e *end, f, x float64) { e.r.lowerFracs[e.k], e.r.downX[e.k] = f, x })
	// Levels: in each gap, a run whose upper end lies inside another run's
	// span goes below it, where its drop does not cross that run's level.
	gapRuns := map[int][]*run{}
	for _, r := range runs {
		gapRuns[r.gap] = append(gapRuns[r.gap], r)
	}
	span := func(r *run) (float64, float64) {
		lo, hi := r.upX, r.upX
		for _, x := range r.downX {
			lo, hi = math.Min(lo, x), math.Max(hi, x)
		}
		return lo, hi
	}
	cost := func(a, b *run) int {
		// Crossings when a's level is above b's: b's drops from the upper
		// rank cross a's level, and a's drops into the lower rank cross
		// b's level.
		c := 0
		alo, ahi := span(a)
		blo, bhi := span(b)
		if b.upX > alo && b.upX < ahi {
			c++
		}
		for _, x := range a.downX {
			if x > blo && x < bhi {
				c++
			}
		}
		return c
	}
	for g, rs := range gapRuns {
		wins := map[*run]int{}
		for i, a := range rs {
			for _, b := range rs[i+1:] {
				switch ab, ba := cost(a, b), cost(b, a); {
				case ab < ba:
					wins[a]++
				case ba < ab:
					wins[b]++
				}
			}
		}
		sort.SliceStable(rs, func(i, j int) bool { return wins[rs[i]] > wins[rs[j]] })
		h := top[g+1] - bottom[g]
		step := 0.0
		if len(rs) > 1 {
			step = math.Min(channelStep*2, (h-2*channelPad)/float64(len(rs)-1))
		}
		first := bottom[g] + (h-step*float64(len(rs)-1))/2
		for i, r := range rs {
			r.level = first + float64(i)*step
		}
	}
	for _, r := range runs {
		for k, l := range r.links {
			pts := []geom.Point{{X: r.upX, Y: r.level}, {X: r.downX[k], Y: r.level}}
			if math.Abs(r.upX-r.downX[k]) < 0.5 {
				pts = nil
			}
			w := l.w
			upperEnd, lowerEnd := "exit", "entry"
			if w.Src != r.upper {
				upperEnd, lowerEnd = "entry", "exit"
				for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
					pts[i], pts[j] = pts[j], pts[i]
				}
			}
			w.Points = pts
			anchor(w, upperEnd, r.upperFrac, 1)
			anchor(w, lowerEnd, r.lowerFracs[k], 0)
			w.Routed = true
		}
	}
	return rest
}

func anchor(w *tidy.Wire, prefix string, fx, fy float64) {
	w.Style.Set(prefix+"X", geom.Format(fx))
	w.Style.Set(prefix+"Y", geom.Format(fy))
	w.Style.Set(prefix+"Dx", "0")
	w.Style.Set(prefix+"Dy", "0")
}
