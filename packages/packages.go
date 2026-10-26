// Package packages lays out package diagrams: packages nested in packages,
// such as the layers of an architecture holding the packages of each layer,
// joined by dependencies. The top-level packages keep the rows they were
// drawn in, so layers stay in the author's order; inside a package, its
// packages and classifiers go in rows ranked by the realizations between
// them (an interface above what realizes it), each
// row ordered to keep dependencies from crossing, and every package shrinks
// or grows to fit what it holds.
package packages

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
)

// Spacing, in pixels.
const (
	gapX    = 40 // between neighbors in a row
	gapY    = 60 // between rows, room for dependencies to turn
	pad     = 20 // inside a package, around what it holds
	sweeps  = 4  // ordering passes
	minHead = 30 // a package's title, at least
)

// Unsupported explains why a diagram cannot be laid out as a package diagram.
type Unsupported struct{ Reason string }

func (u *Unsupported) Error() string { return u.Reason }

// block is a shape and, for a package holding shapes, its layout.
type block struct {
	n      *tidy.Node // nil for the diagram itself
	kids   []*block
	rows   [][]*block
	head   float64 // room for the package's title
	minW   float64 // the title's width
	w, h   float64
	parent *block
}

// Layout places the packages of a working copy and routes the dependencies.
func Layout(d *tidy.Diagram) error {
	of := map[*tidy.Node]*block{}
	root := &block{}
	var texts []*tidy.Node
	for _, n := range d.Nodes {
		if n.Text || n.Overlay {
			texts = append(texts, n)
			continue
		}
		b := &block{n: n}
		of[n] = b
	}
	for _, n := range d.Nodes {
		b := of[n]
		if b == nil {
			continue
		}
		p := root
		if n.Parent != nil {
			if pb := of[n.Parent]; pb != nil {
				p = pb
			} else {
				return &Unsupported{fmt.Sprintf("shape %s sits in %s, which is not a package", n.V.ID, n.Parent.V.ID)}
			}
		}
		b.parent = p
		p.kids = append(p.kids, b)
	}
	if len(root.kids) == 0 {
		return &Unsupported{"no package"}
	}
	for _, w := range d.Wires {
		if w.Src == nil || w.Dst == nil {
			return &Unsupported{fmt.Sprintf("dependency %s has a free end", w.V.Cell.ID)}
		}
		if of[w.Src] == nil || of[w.Dst] == nil {
			return &Unsupported{fmt.Sprintf("dependency %s ends on a text", w.V.Cell.ID)}
		}
	}
	// Rows: the diagram keeps the rows it was drawn in; a package ranks what
	// it holds by the relations between them, or keeps their rows too.
	var rowsOf func(b *block)
	rowsOf = func(b *block) {
		for _, k := range b.kids {
			rowsOf(k)
		}
		if len(b.kids) == 0 {
			return
		}
		var rank map[*block]int
		if b != root {
			rank = ranked(b.kids, d.Wires, of)
		}
		if rank == nil {
			rank = drawnRows(b.kids)
		}
		top := 0
		for _, r := range rank {
			top = max(top, r)
		}
		b.rows = make([][]*block, top+1)
		for _, k := range b.kids {
			b.rows[rank[k]] = append(b.rows[rank[k]], k)
		}
		for _, row := range b.rows {
			sort.SliceStable(row, func(i, j int) bool { return row[i].n.Box.X < row[j].n.Box.X })
		}
	}
	rowsOf(root)
	for _, b := range of {
		if len(b.kids) > 0 {
			b.head, b.minW = head(d, b.n)
		}
	}
	origin := geom.Point{X: math.Inf(1), Y: math.Inf(1)}
	for _, n := range d.Nodes {
		origin.X, origin.Y = math.Min(origin.X, n.Box.X), math.Min(origin.Y, n.Box.Y)
	}
	orig := map[*tidy.Node]geom.Rect{}
	for _, n := range d.Nodes {
		orig[n] = n.Box
	}
	for s := 0; s <= sweeps; s++ {
		size(root)
		stretch(root)
		place(root, origin)
		if s < sweeps {
			reorder(root, d.Wires, of)
		}
	}
	// Texts keep their place relative to the shape they belong to.
	for _, t := range texts {
		host := t.Parent
		if host == nil {
			var best *tidy.Node
			bd := math.Inf(1)
			for n, b := range orig {
				if of[n] == nil {
					continue
				}
				if dd := dist(t.Orig, b); dd < bd {
					best, bd = n, dd
				}
			}
			host = best
		}
		if host != nil {
			o := orig[host]
			t.Box = t.Box.Move(geom.Point{X: host.Box.X - o.X, Y: host.Box.Y - o.Y})
		}
	}
	for _, w := range d.Wires {
		w.Points = nil
		d.RouteWire(w)
	}
	return nil
}

// ranked ranks the shapes of one package by the realizations and
// generalizations between them, what is realized above what realizes it.
// Dependencies do not rank: packages of one layer that use each other stay
// side by side. It returns nil when no such relation joins them.
func ranked(kids []*block, wires []*tidy.Wire, of map[*tidy.Node]*block) map[*block]int {
	in := map[*block]bool{}
	for _, k := range kids {
		in[k] = true
	}
	below := map[*block][]*block{}
	any := false
	reaches := func(from, to *block) bool {
		seen := map[*block]bool{}
		stack := []*block{from}
		for len(stack) > 0 {
			b := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if b == to {
				return true
			}
			if !seen[b] {
				seen[b] = true
				stack = append(stack, below[b]...)
			}
		}
		return false
	}
	for _, w := range wires {
		up, down := of[w.Src], of[w.Dst]
		if !in[up] || !in[down] || up == down || !realizes(w) {
			continue
		}
		up, down = down, up
		if reaches(down, up) {
			continue
		}
		below[up] = append(below[up], down)
		any = true
	}
	if !any {
		return nil
	}
	rank := map[*block]int{}
	var visit func(b *block, r int)
	visit = func(b *block, r int) {
		if old, ok := rank[b]; ok && old >= r {
			return
		}
		rank[b] = r
		for _, c := range below[b] {
			visit(c, r+1)
		}
	}
	above := map[*block]int{}
	for _, cs := range below {
		for _, c := range cs {
			above[c]++
		}
	}
	for _, k := range kids {
		if above[k] == 0 {
			visit(k, 0)
		}
	}
	return rank
}

// realizes reports a realization or generalization: a hollow triangle at the
// target, which is drawn above.
func realizes(w *tidy.Wire) bool {
	return w.Style.Value("endArrow", "classic") == "block" && w.Style.Value("endFill", "1") == "0"
}

// drawnRows numbers the rows shapes were drawn in: a shape starting below
// the middle of the row so far opens a new one.
func drawnRows(kids []*block) map[*block]int {
	sorted := append([]*block(nil), kids...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].n.Box.Y < sorted[j].n.Box.Y })
	rank := map[*block]int{}
	r, mid := 0, math.Inf(-1)
	for i, b := range sorted {
		if i > 0 && b.n.Box.Y > mid {
			r++
		}
		if i == 0 || b.n.Box.Y > mid {
			mid = b.n.Box.Center().Y
		}
		rank[b] = r
	}
	return rank
}

// head is the room a package needs above what it holds for its title, as
// the folder shape draws it below its tab, and the title's width.
func head(d *tidy.Diagram, n *tidy.Node) (float64, float64) {
	st := n.V.Style
	w, h := d.TextSize(n.V.Text, st)
	w += 2*pad + geom.ParseNumber(st.Value("spacingLeft", "0"))
	if st.Value("verticalAlign", "middle") != "top" {
		return minHead, w
	}
	top := geom.ParseNumber(st.Value("spacingTop", "0"))
	return math.Max(minHead, top+h+pad/2), w
}

// size works out every block's size from what it holds, bottom up. A leaf
// keeps its own.
func size(b *block) {
	for _, k := range b.kids {
		size(k)
	}
	if len(b.kids) == 0 {
		b.w, b.h = b.n.Box.W, b.n.Box.H
		return
	}
	cw, ch := 0.0, 0.0
	for i, row := range b.rows {
		rw, rh := rowSize(row)
		cw = math.Max(cw, rw)
		ch += rh
		if i > 0 {
			ch += gapY
		}
	}
	if b.n == nil {
		b.w, b.h = cw, ch
		return
	}
	b.w = math.Max(cw+2*pad, b.minW)
	b.h = b.head + ch + pad
}

func rowSize(row []*block) (w, h float64) {
	for i, k := range row {
		w += k.w
		if i > 0 {
			w += gapX
		}
		h = math.Max(h, k.h)
	}
	return w, h
}

// stretch gives the packages standing alone in their row of the diagram,
// such as the layers of an architecture, the width of the widest of them.
func stretch(root *block) {
	widest := 0.0
	var alone []*block
	for _, row := range root.rows {
		if len(row) == 1 && len(row[0].kids) > 0 {
			alone = append(alone, row[0])
			widest = math.Max(widest, row[0].w)
		}
	}
	if len(alone) < 2 {
		return
	}
	for _, b := range alone {
		b.w = widest
	}
	root.w = math.Max(root.w, widest)
}

// place sets every block's box from its size, its rows centered in it.
func place(b *block, at geom.Point) {
	if b.n != nil {
		b.n.Box = geom.Rect{X: at.X, Y: at.Y, W: b.w, H: b.h}
	}
	if len(b.kids) == 0 {
		return
	}
	inner := geom.Rect{X: at.X, Y: at.Y, W: b.w, H: b.h}
	if b.n != nil {
		inner = geom.Rect{X: at.X + pad, Y: at.Y + b.head, W: b.w - 2*pad, H: b.h - b.head - pad}
	}
	y := inner.Y
	for _, row := range b.rows {
		rw, rh := rowSize(row)
		x := inner.X + (inner.W-rw)/2
		for _, k := range row {
			place(k, geom.Point{X: x, Y: y + (rh-k.h)/2})
			x += k.w + gapX
		}
		y += rh + gapY
	}
}

// reorder sorts each row by the average x of the shapes its members depend
// on or are depended on by, inside them included.
func reorder(b *block, wires []*tidy.Wire, of map[*tidy.Node]*block) {
	for _, k := range b.kids {
		reorder(k, wires, of)
	}
	inside := func(x, in *block) bool {
		for ; x != nil; x = x.parent {
			if x == in {
				return true
			}
		}
		return false
	}
	for _, row := range b.rows {
		key := map[*block]float64{}
		for _, k := range row {
			sum, cnt := 0.0, 0
			for _, w := range wires {
				s, t := of[w.Src], of[w.Dst]
				var o *block
				switch {
				case inside(s, k) && !inside(t, k):
					o = t
				case inside(t, k) && !inside(s, k):
					o = s
				default:
					continue
				}
				sum += o.n.Box.Center().X
				cnt++
			}
			if cnt > 0 {
				key[k] = sum / float64(cnt)
			} else {
				key[k] = k.n.Box.Center().X
			}
		}
		sort.SliceStable(row, func(i, j int) bool { return key[row[i]] < key[row[j]] })
	}
}

func dist(a, b geom.Rect) float64 {
	dx := math.Max(0, math.Max(a.X-b.Right(), b.X-a.Right()))
	dy := math.Max(0, math.Max(a.Y-b.Bottom(), b.Y-a.Bottom()))
	return math.Hypot(dx, dy)
}
