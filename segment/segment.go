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

// Split segments a view.
func Split(v *view.View) Page {
	idx := map[*view.Node]int{}
	for i, n := range v.Nodes {
		idx[n] = i
	}
	uf := newUnion(len(v.Nodes))
	for _, n := range v.Nodes {
		if top := topContainer(n); top != n {
			uf.join(idx[n], idx[top])
		}
	}
	wired := map[*view.Node]bool{}
	for _, w := range v.Wires {
		if w.Src != nil {
			wired[w.Src] = true
		}
		if w.Dst != nil {
			wired[w.Dst] = true
		}
		if w.Src != nil && w.Dst != nil {
			uf.join(idx[w.Src], idx[w.Dst])
		}
	}
	// Free texts join the nearest wired node within reach.
	for _, n := range v.Nodes {
		if !n.TextOnly || wired[n] || n.Parent != nil {
			continue
		}
		var best *view.Node
		bd := math.Inf(1)
		for _, m := range v.Nodes {
			if m == n || m.TextOnly || m.Container || !wired[m] {
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
	groups := map[int][]*view.Node{}
	for _, n := range v.Nodes {
		r := uf.find(idx[n])
		groups[r] = append(groups[r], n)
	}
	var out Page
	byRoot := map[int]*Diagram{}
	roots := make([]int, 0, len(groups))
	for r := range groups {
		roots = append(roots, r)
	}
	sort.Ints(roots)
	for _, r := range roots {
		nodes := groups[r]
		hasWire := false
		hasContainer := false
		for _, n := range nodes {
			hasWire = hasWire || wired[n]
			hasContainer = hasContainer || n.Container
		}
		if !hasWire && !hasContainer {
			out.Decoration = append(out.Decoration, nodes...)
			continue
		}
		d := &Diagram{Page: v.Page.Index, Nodes: nodes}
		byRoot[r] = d
		out.Diagrams = append(out.Diagrams, d)
	}
	for _, w := range v.Wires {
		n := w.Src
		if n == nil {
			n = w.Dst
		}
		if n == nil {
			out.LooseWires = append(out.LooseWires, w)
			continue
		}
		if d := byRoot[uf.find(idx[n])]; d != nil {
			d.Wires = append(d.Wires, w)
		}
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
