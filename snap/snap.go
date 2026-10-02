// Package snap attaches wire ends that were dropped next to a shape instead of
// on it. The rules are in docs/design.md: an end inside a shape or close to its
// outline attaches; an end with two equally close candidates is left alone with
// a warning, because a wrong guess is worse than none.
package snap

import (
	"fmt"
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/issue"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/view"
)

// Params are the thresholds; see the snap-* settings.
type Params struct {
	Distance float64 // pixels
	Ratio    float64 // share of the shape's shorter side
	Margin   float64 // nearest must be this many times closer than the next
}

// Result lists what Run did on one page.
type Result struct {
	Snapped   int
	Ambiguous int
	Details   []report.Detail
	Issues    []issue.Issue
}

// Run snaps the free wire ends of a view's page, writing the attachments into
// the document. The view is stale afterward; build a new one.
func Run(v *view.View, p Params) Result {
	var res Result
	pairs := map[[2]string]bool{}
	for _, w := range v.Wires {
		if w.Src != nil && w.Dst != nil {
			pairs[[2]string{w.Src.ID, w.Dst.ID}] = true
		}
	}
	for _, w := range v.Wires {
		for _, source := range []bool{true, false} {
			n, d, amb := choose(v, w, source, p, pairs)
			end := "target"
			if source {
				end = "source"
			}
			switch {
			case amb != nil:
				res.Ambiguous++
				res.Issues = append(res.Issues, issue.New(issue.Warning, "snap.ambiguous", v.Page.Index,
					[]string{w.Cell.ID, amb[0].ID, amb[1].ID},
					"the %s end of wire %s is about as close to %s as to %s; left unattached",
					end, w.Cell.ID, describe(amb[0]), describe(amb[1])))
			case n != nil:
				w.Cell.SetTerminal(source, n.Cells[0].ID)
				if source {
					w.Src = n
				} else {
					w.Dst = n
				}
				if w.Src != nil && w.Dst != nil {
					pairs[[2]string{w.Src.ID, w.Dst.ID}] = true
				}
				res.Snapped++
				res.Details = append(res.Details, report.Detail{
					Op: "snap", Cells: []string{w.Cell.ID, n.Cells[0].ID},
					Msg: fmt.Sprintf("wire %s %s end --> %s (%s px)", w.Cell.ID, end, describe(n), geom.Format(d)),
				})
			}
		}
	}
	return res
}

// choose picks the node a free end should attach to. It returns the node and
// its distance, or the two tied candidates when the choice is ambiguous, or
// nothing.
func choose(v *view.View, w *view.Wire, source bool, p Params, pairs map[[2]string]bool) (*view.Node, float64, []*view.Node) {
	ref, other := w.DstRef, w.Src
	if source {
		ref, other = w.SrcRef, w.Dst
	}
	if ref != "" && v.Page.Cell(ref) != nil {
		return nil, 0, nil
	}
	pt, ok := w.Cell.TerminalPoint(source)
	if !ok {
		return nil, 0, nil
	}
	// A snap never makes a wire loop back to its own shape. Duplicating an
	// existing wire is legal (parallel wires exist), so it only breaks ties.
	duplicate := func(n *view.Node) bool {
		if other == nil {
			return false
		}
		key := [2]string{n.ID, other.ID}
		if !source {
			key = [2]string{other.ID, n.ID}
		}
		return pairs[key]
	}
	plausible := func(n *view.Node) bool { return n != other }
	type cand struct {
		n *view.Node
		d float64
	}
	var inside, near []cand
	for _, n := range v.Leaves() {
		if !plausible(n) {
			continue
		}
		d := n.Box.Dist(pt)
		if d == 0 {
			inside = append(inside, cand{n, 0})
			continue
		}
		if d <= threshold(n.Box, p) {
			near = append(near, cand{n, d})
		}
	}
	if len(inside) > 0 {
		sort.SliceStable(inside, func(i, j int) bool { return area(inside[i].n) < area(inside[j].n) })
		best := inside[0].n
		for _, c := range inside[1:] {
			if !c.n.Box.ContainsRect(best.Box) && duplicate(best) == duplicate(c.n) {
				return nil, 0, []*view.Node{best, c.n}
			}
		}
		if duplicate(best) {
			for _, c := range inside[1:] {
				if !duplicate(c.n) && !c.n.Box.ContainsRect(best.Box) {
					best = c.n
					break
				}
			}
		}
		return best, 0, nil
	}
	if len(near) > 0 {
		sort.SliceStable(near, func(i, j int) bool { return near[i].d < near[j].d })
		if len(near) > 1 && near[1].d < p.Margin*near[0].d {
			a, b := near[0], near[1]
			switch {
			case duplicate(a.n) && !duplicate(b.n):
				return b.n, b.d, nil
			case duplicate(b.n) && !duplicate(a.n):
				return a.n, a.d, nil
			}
			return nil, 0, []*view.Node{a.n, b.n}
		}
		return near[0].n, near[0].d, nil
	}
	// No shape qualifies: an end close to a container's outline belongs to the
	// container. An end merely somewhere inside one is left free, since arrows
	// pointing into empty lane space are usually annotations.
	for _, n := range v.Containers() {
		if !plausible(n) {
			continue
		}
		if d := n.Box.DistToOutline(pt); d <= threshold(n.Box, p) {
			return n, d, nil
		}
	}
	return nil, 0, nil
}

func threshold(b geom.Rect, p Params) float64 {
	return math.Min(p.Distance, p.Ratio*math.Min(b.W, b.H))
}

func area(n *view.Node) float64 { return n.Box.W * n.Box.H }

func describe(n *view.Node) string {
	if n.Text != "" {
		t := n.Text
		if r := []rune(t); len(r) > 30 {
			t = string(r[:30]) + "..."
		}
		return fmt.Sprintf("%s %q", n.ID, t)
	}
	return n.ID
}
