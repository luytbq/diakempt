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

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
)

// Spacing, in pixels. The vertical gap leaves room for the trunks of
// generalization arrows between ranks.
const (
	gapX      = 60
	gapY      = 90
	trunkStep = 10 // between the trunks of different parents in one gap
	sweeps    = 6  // ordering passes
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
	place(layers, links, rank)
	keepCorner(d, nodes)
	route(d, layers, links, rank)
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
	ranked := map[*tidy.Node]bool{}
	for _, rel := range []relation{generalization, whole} {
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
	used := map[int]bool{}
	for _, r := range rank {
		used[r] = true
	}
	var levels []int
	for r := range used {
		levels = append(levels, r)
	}
	sort.Ints(levels)
	dense := map[int]int{}
	for i, r := range levels {
		dense[r] = i
	}
	for n, r := range rank {
		rank[n] = dense[r]
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
func place(layers [][]*tidy.Node, links []*link, rank map[*tidy.Node]int) {
	y := 0.0
	for _, l := range layers {
		x := 0.0
		h := 0.0
		for _, n := range l {
			n.Box.X, n.Box.Y = x, y
			x += n.Box.W + gapX
			h = math.Max(h, n.Box.H)
		}
		y += h + gapY
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
// where it was.
func keepCorner(d *tidy.Diagram, nodes []*tidy.Node) {
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

// route draws generalizations between adjacent ranks as trunks, one per
// parent: from the top of each child up to a shared level in the gap, across
// to the parent's center, and up into its bottom. Every other wire is routed
// around the classes afterwards, so it sees the trunks.
func route(d *tidy.Diagram, layers [][]*tidy.Node, links []*link, rank map[*tidy.Node]int) {
	rankTop := make([]float64, len(layers))
	for r, l := range layers {
		rankTop[r] = math.Inf(1)
		for _, n := range l {
			rankTop[r] = math.Min(rankTop[r], n.Box.Y)
		}
	}
	trunks := map[*tidy.Node]int{} // parent --> trunk index in its gap
	perGap := map[int]int{}
	exits := map[*tidy.Node]int{} // child --> generalizations leaving it so far
	children := map[*tidy.Node]int{}
	for _, l := range links {
		if l.rel == generalization && rank[l.down] == rank[l.up]+1 {
			children[l.down]++
		}
	}
	var rest []*link
	for _, l := range links {
		if l.rel != generalization || rank[l.down] != rank[l.up]+1 {
			rest = append(rest, l)
			continue
		}
		p, c := l.up, l.down
		k, ok := trunks[p]
		if !ok {
			k = perGap[rank[c]]
			perGap[rank[c]]++
			trunks[p] = k
		}
		trunkY := rankTop[rank[c]] - gapY/2 - float64(k)*trunkStep
		// A child with two parents leaves from two points on its top side.
		fx := 0.5
		if n := children[c]; n > 1 {
			fx = float64(exits[c]+1) / float64(n+1)
		}
		exits[c]++
		from := geom.Point{X: c.Box.X + fx*c.Box.W, Y: c.Box.Y}
		to := geom.Point{X: p.Box.Center().X, Y: p.Box.Bottom()}
		w := l.w
		srcIsChild := w.Src == c
		pts := []geom.Point{{X: from.X, Y: trunkY}, {X: to.X, Y: trunkY}}
		if math.Abs(from.X-to.X) < 0.5 {
			pts = nil
		}
		if !srcIsChild {
			for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
				pts[i], pts[j] = pts[j], pts[i]
			}
		}
		w.Points = pts
		childEnd, parentEnd := "exit", "entry"
		if !srcIsChild {
			childEnd, parentEnd = "entry", "exit"
		}
		anchor(w, childEnd, fx, 0)
		anchor(w, parentEnd, 0.5, 1)
		w.Routed = true
	}
	for _, l := range rest {
		d.RouteWire(l.w)
	}
}

func anchor(w *tidy.Wire, prefix string, fx, fy float64) {
	w.Style.Set(prefix+"X", geom.Format(fx))
	w.Style.Set(prefix+"Y", geom.Format(fy))
	w.Style.Set(prefix+"Dx", "0")
	w.Style.Set(prefix+"Dy", "0")
}
