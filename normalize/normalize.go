// Package normalize rewrites a page's structure into a canonical form, so more
// diagrams can be recognized and laid out. Only the aggressive level runs it,
// because unlike every other step it changes more than geometry: it merges,
// moves and deletes cells. Every rewrite is reported.
package normalize

import (
	"fmt"
	"math"
	"strings"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/view"
)

// labelReach is how close, in pixels, a free text must be to a wire to be
// taken as that wire's label.
const labelReach = 12

// Run applies every rewrite to a page, in an order where each step sees the
// result of the one before: groups first so their contents are plain cells,
// then stacked shapes, wires connected to wires, and texts next to wires.
func Run(p *doc.Page) []report.Detail {
	var out []report.Detail
	out = append(out, dissolveGroups(p)...)
	out = append(out, mergeStacked(p)...)
	out = append(out, joinWires(p)...)
	out = append(out, adoptLabels(p)...)
	return out
}

func detail(cells []string, format string, args ...any) report.Detail {
	return report.Detail{Op: "normalize", Cells: cells, Msg: fmt.Sprintf(format, args...)}
}

// dissolveGroups moves the contents of convenience groups up to the group's
// parent, where they are drawn, and deletes the group. A group is a selection
// aid in draw.io, not a part of the picture.
func dissolveGroups(p *doc.Page) []report.Detail {
	var out []report.Detail
	for changed := true; changed; {
		changed = false
		for _, c := range p.Cells {
			if !c.IsVertex() || c.Style().Shape() != "group" || c.Label() != "" {
				continue
			}
			kids := p.Children(c.ID)
			parent := c.Parent()
			for _, k := range kids {
				k.SetParent(parent)
			}
			p.Remove(c)
			p.Reindex()
			out = append(out, detail([]string{c.ID}, "dissolved group %s (%d cells moved to its parent)", c.ID, len(kids)))
			changed = true
			break
		}
	}
	return out
}

// mergeStacked turns a text box laid over a shape into the shape's own label,
// and points wires attached to the text box at the shape.
func mergeStacked(p *doc.Page) []report.Detail {
	var out []report.Detail
	v := view.Build(p)
	for _, n := range v.Nodes {
		if len(n.Cells) < 2 {
			continue
		}
		host := n.Cells[0]
		var texts []string
		if t := host.Label(); strings.TrimSpace(t) != "" {
			texts = append(texts, t)
		}
		for _, m := range n.Cells[1:] {
			texts = append(texts, m.Label())
		}
		html := host.Style().Value("html", "0") == "1"
		sep := "\n"
		if html {
			sep = "<br>"
		}
		host.SetLabel(strings.Join(texts, sep))
		for _, m := range n.Cells[1:] {
			for _, w := range p.Cells {
				if w.Source() == m.ID {
					w.SetTerminal(true, host.ID)
				}
				if w.Target() == m.ID {
					w.SetTerminal(false, host.ID)
				}
			}
			p.Remove(m)
			out = append(out, detail([]string{m.ID, host.ID}, "merged text %s into the label of %s", m.ID, host.ID))
		}
	}
	p.Reindex()
	return out
}

// joinWires points a wire that ends on another wire at that wire's own end, the
// shape the joined line flows to (or comes from, for a source end).
func joinWires(p *doc.Page) []report.Detail {
	var out []report.Detail
	for _, w := range p.Cells {
		if !w.IsEdge() {
			continue
		}
		for _, source := range []bool{true, false} {
			ref := w.Target()
			if source {
				ref = w.Source()
			}
			o := p.Cell(ref)
			if o == nil || !o.IsEdge() {
				continue
			}
			to := o.Target()
			if source {
				to = o.Source()
			}
			if to == "" || to == w.ID {
				continue
			}
			w.SetTerminal(source, to)
			out = append(out, detail([]string{w.ID, o.ID, to}, "wire %s ended on wire %s; attached it to %s instead", w.ID, o.ID, to))
		}
	}
	return out
}

// adoptLabels makes a free text standing next to a wire without a label that
// wire's label, and deletes the text box.
func adoptLabels(p *doc.Page) []report.Detail {
	var out []report.Detail
	v := view.Build(p)
	used := map[*view.Wire]bool{}
	for _, n := range v.Nodes {
		if !n.TextOnly || n.Container || len(n.Cells) != 1 || n.Text == "" {
			continue
		}
		c := n.Cells[0]
		attached := false
		for _, w := range v.Wires {
			attached = attached || w.Src == n || w.Dst == n
		}
		if attached {
			continue
		}
		var best *view.Wire
		bd := math.Inf(1)
		for _, w := range v.Wires {
			if w.Text != "" || used[w] {
				continue
			}
			if d := distToPath(w.Path, n.Box); d < bd {
				best, bd = w, d
			}
		}
		if best == nil || bd > labelReach {
			continue
		}
		used[best] = true
		label := c.Label()
		if c.Style().Value("html", "0") == "1" && best.Style.Value("html", "0") != "1" {
			label = n.Text
		}
		best.Cell.SetLabel(label)
		p.Remove(c)
		out = append(out, detail([]string{c.ID, best.Cell.ID}, "made text %s the label of wire %s", c.ID, best.Cell.ID))
	}
	p.Reindex()
	return out
}

// distToPath returns how far a box is from a polyline.
func distToPath(pl geom.Polyline, b geom.Rect) float64 {
	best := math.Inf(1)
	for _, s := range pl.Segments() {
		for _, q := range []geom.Point{b.Center(), {X: b.X, Y: b.Y}, {X: b.Right(), Y: b.Y}, {X: b.X, Y: b.Bottom()}, {X: b.Right(), Y: b.Bottom()}} {
			best = math.Min(best, segDist(s, q))
		}
		if s.CrossesRect(b, 0) {
			return 0
		}
	}
	return best
}

func segDist(s geom.Segment, p geom.Point) float64 {
	dx, dy := s.B.X-s.A.X, s.B.Y-s.A.Y
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return p.Dist(s.A)
	}
	t := math.Max(0, math.Min(1, ((p.X-s.A.X)*dx+(p.Y-s.A.Y)*dy)/l2))
	return p.Dist(geom.Point{X: s.A.X + t*dx, Y: s.A.Y + t*dy})
}
