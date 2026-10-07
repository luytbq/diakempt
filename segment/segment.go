// Package segment splits a page into diagrams and decoration.
//
// A diagram is a connected group of nodes and wires, joined with the containers
// the nodes sit in: everything inside one top-level container belongs to one
// diagram. A free text close to a node, with no wire, joins that node's diagram.
// What is left (lone shapes, titles, legends, wires with both ends free) is
// decoration.
package segment

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/view"
)

// Diagram is one connected group on a page.
type Diagram struct {
	Page  int
	Index int
	// Name is a readable name: the top container's title, else the text of the
	// first node in reading order.
	Name  string
	Nodes []*view.Node
	Wires []*view.Wire
}

// ID is the stable identifier shown in reports: Page-1 #2 "Login".
func (d *Diagram) ID() string {
	if d.Name == "" {
		return fmt.Sprintf("Page-%d #%d", d.Page+1, d.Index+1)
	}
	return fmt.Sprintf("Page-%d #%d %q", d.Page+1, d.Index+1, d.Name)
}

// Bounds returns the box around the diagram's nodes and wires.
func (d *Diagram) Bounds() geom.Rect {
	var b geom.Rect
	for _, n := range d.Nodes {
		b = b.Union(n.Box)
	}
	for _, w := range d.Wires {
		b = b.Union(w.Path.Bounds())
	}
	return b
}

// Page is a segmented page.
type Page struct {
	Diagrams   []*Diagram
	Decoration []*view.Node
	// LooseWires are wires attached to nothing.
	LooseWires []*view.Wire
}

// AttachDistance is how close, in pixels, a free text must be to a node to
// count as that node's note.
const AttachDistance = 40

// contactDistance is how close, in pixels, a free wire end must be to another
// wire or to a shape outline to count as touching it.
const contactDistance = 4

// Split segments a view. Nodes and wires are grouped together: an attached wire
// joins its nodes, a free wire end touching another wire or a shape joins that,
// and nodes in one top-level container belong together.
func Split(v *view.View) Page {
	nn := len(v.Nodes)
	idx := map[*view.Node]int{}
	for i, n := range v.Nodes {
		idx[n] = i
	}
	widx := map[*view.Wire]int{}
	for i, w := range v.Wires {
		widx[w] = nn + i
	}
	uf := newUnion(nn + len(v.Wires))
	for _, n := range v.Nodes {
		if top := topContainer(n); top != n {
			uf.join(idx[n], idx[top])
		}
	}
	wired := map[*view.Node]bool{}
	for _, w := range v.Wires {
		for _, n := range []*view.Node{w.Src, w.Dst} {
			if n != nil {
				wired[n] = true
				uf.join(widx[w], idx[n])
			}
		}
	}
	// Free wire ends touching another wire or a shape, as in hand-drawn
	// sequence diagrams where arrows run between dashed lines.
	for _, w := range v.Wires {
		for _, source := range []bool{true, false} {
			if (source && w.Src != nil) || (!source && w.Dst != nil) {
				continue
			}
			pt, ok := w.End(source)
			if !ok {
				continue
			}
			for _, o := range v.Wires {
				if o != w && nearPath(o.Path, pt) {
					uf.join(widx[w], widx[o])
				}
			}
			for _, n := range v.Nodes {
				if !n.Container && n.Box.Inset(-contactDistance).Contains(pt) {
					uf.join(widx[w], idx[n])
					wired[n] = true
				}
			}
		}
	}
	// A hand-drawn lifeline whose head was dragged off its top end still
	// belongs with the shape just above it.
	for _, w := range v.Wires {
		if w.Src != nil || w.Dst != nil {
			continue
		}
		x, top, _, ok := HandLifeline(w.Style, w.Path, len(w.Points), nil, nil)
		if !ok {
			continue
		}
		if n := headAbove(v.Nodes, x, top); n != nil {
			uf.join(widx[w], idx[n])
			wired[n] = true
		}
	}
	// Free texts and notes join the nearest wired node within reach.
	for _, n := range v.Nodes {
		if !(n.TextOnly || n.Shape == "note") || wired[n] || n.Parent != nil {
			continue
		}
		var best *view.Node
		bd := math.Inf(1)
		for _, m := range v.Nodes {
			if m == n || m.TextOnly || !(wired[m] || wiredInside(m, wired)) {
				continue
			}
			if d := gap(n.Box, m.Box); d < bd {
				best, bd = m, d
			}
		}
		if best != nil && bd <= AttachDistance {
			uf.join(idx[n], idx[best])
		}
	}
	// A UML frame (alt, loop, opt) belongs with the messages it encloses.
	for _, n := range v.Nodes {
		if n.Shape != "umlFrame" || wired[n] {
			continue
		}
		for _, w := range v.Wires {
			if crossesBox(w.Path, n.Box) {
				uf.join(idx[n], widx[w])
				break
			}
		}
	}
	type group struct {
		nodes []*view.Node
		wires []*view.Wire
	}
	groups := map[int]*group{}
	get := func(r int) *group {
		if groups[r] == nil {
			groups[r] = &group{}
		}
		return groups[r]
	}
	for _, n := range v.Nodes {
		g := get(uf.find(idx[n]))
		g.nodes = append(g.nodes, n)
	}
	for _, w := range v.Wires {
		g := get(uf.find(widx[w]))
		g.wires = append(g.wires, w)
	}
	roots := make([]int, 0, len(groups))
	for r := range groups {
		roots = append(roots, r)
	}
	sort.Ints(roots)
	var out Page
	for _, r := range roots {
		g := groups[r]
		hasContainer := false
		for _, n := range g.nodes {
			hasContainer = hasContainer || (n.Container && len(n.Children) > 0)
		}
		// A diagram has wires and something they connect, or a container
		// holding shapes. Anything smaller is decoration.
		if !(len(g.wires) > 0 && len(g.nodes)+len(g.wires) >= 2 && len(g.nodes) > 0) && !hasContainer {
			out.Decoration = append(out.Decoration, g.nodes...)
			out.LooseWires = append(out.LooseWires, g.wires...)
			continue
		}
		out.Diagrams = append(out.Diagrams, &Diagram{Page: v.Page.Index, Nodes: g.nodes, Wires: g.wires})
	}
	// Reading order: top to bottom in bands, then left to right.
	sort.SliceStable(out.Diagrams, func(i, j int) bool {
		a, b := out.Diagrams[i].Bounds(), out.Diagrams[j].Bounds()
		if math.Abs(a.Y-b.Y) > 40 {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	for i, d := range out.Diagrams {
		d.Index = i
		sortNodes(d.Nodes, idx)
		d.Name = name(d)
	}
	sortNodes(out.Decoration, idx)
	return out
}

// MinLifeline is the shortest line, in pixels, read as a hand-drawn lifeline.
const MinLifeline = 60

// HandLifeline reads a wire as a hand-drawn lifeline: a dashed line without
// arrowheads or waypoints running straight down at least MinLifeline, with
// free ends, or with its top end on the head shape it hangs from (src or dst,
// the boxes of the attached ends). It returns the line's x and the heights of
// its top and bottom.
func HandLifeline(st doc.Style, pl geom.Polyline, points int, src, dst *geom.Rect) (x, top, bottom float64, ok bool) {
	if len(pl) < 2 || points > 0 || st.Value("dashed", "0") != "1" ||
		st.Value("endArrow", "classic") != "none" || st.Value("startArrow", "none") != "none" {
		return 0, 0, 0, false
	}
	a, b := pl[0], pl[len(pl)-1]
	head := src
	if src != nil && dst != nil {
		return 0, 0, 0, false
	}
	if dst != nil {
		head, a, b = dst, b, a
	}
	if head != nil {
		// a is on the head's outline; the free end b must hang below it.
		x, top, bottom = b.X, head.Bottom(), b.Y
		if x < head.X || x > head.Right() {
			return 0, 0, 0, false
		}
	} else {
		if math.Abs(a.X-b.X) > 2 {
			return 0, 0, 0, false
		}
		x, top, bottom = a.X, math.Min(a.Y, b.Y), math.Max(a.Y, b.Y)
	}
	return x, top, bottom, bottom-top >= MinLifeline
}

// headAbove returns the shape a lifeline top at (x, top) hangs from: the
// closest shape spanning x whose bottom is at most AttachDistance above the
// top, or nil.
func headAbove(nodes []*view.Node, x, top float64) *view.Node {
	var best *view.Node
	bd := math.Inf(1)
	for _, n := range nodes {
		if n.Container || n.TextOnly || x < n.Box.X-contactDistance || x > n.Box.Right()+contactDistance {
			continue
		}
		d := top - n.Box.Bottom()
		if d < -contactDistance || d > AttachDistance || n.Box.Y > top {
			continue
		}
		if d < bd {
			best, bd = n, d
		}
	}
	return best
}

// wiredInside reports whether a container holds a wired shape, as a lifeline
// holds the activation bars its messages attach to.
func wiredInside(n *view.Node, wired map[*view.Node]bool) bool {
	for _, c := range n.Children {
		if wired[c] || wiredInside(c, wired) {
			return true
		}
	}
	return false
}

func crossesBox(pl geom.Polyline, b geom.Rect) bool {
	for _, s := range pl.Segments() {
		if s.CrossesRect(b, 0) || b.Contains(s.A) || b.Contains(s.B) {
			return true
		}
	}
	return false
}

func nearPath(pl geom.Polyline, p geom.Point) bool {
	for _, s := range pl.Segments() {
		if s.Dist(p) <= contactDistance {
			return true
		}
	}
	return false
}

func sortNodes(ns []*view.Node, idx map[*view.Node]int) {
	sort.SliceStable(ns, func(i, j int) bool { return idx[ns[i]] < idx[ns[j]] })
}

func topContainer(n *view.Node) *view.Node {
	top := n
	for p := n.Parent; p != nil; p = p.Parent {
		top = p
	}
	return top
}

func name(d *Diagram) string {
	for _, n := range d.Nodes {
		if n.Container && n.Parent == nil && n.Text != "" {
			return short(n.Text)
		}
	}
	var best *view.Node
	for _, n := range d.Nodes {
		if n.Container || n.Text == "" {
			continue
		}
		if best == nil || n.Box.Y < best.Box.Y-10 || (math.Abs(n.Box.Y-best.Box.Y) <= 10 && n.Box.X < best.Box.X) {
			best = n
		}
	}
	if best == nil {
		return ""
	}
	return short(best.Text)
}

func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 40 {
		return string(r[:40]) + "..."
	}
	return s
}

// gap returns the distance between two boxes, zero when they touch or overlap.
func gap(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}

type union struct{ parent []int }

func newUnion(n int) *union {
	u := &union{parent: make([]int, n)}
	for i := range u.parent {
		u.parent[i] = i
	}
	return u
}

func (u *union) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]]
		i = u.parent[i]
	}
	return i
}

func (u *union) join(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if ra > rb {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
}
