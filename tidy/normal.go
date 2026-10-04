package tidy

import (
	"math"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
)

// siblings groups solid nodes and containers by parent, in document order.
func (d *Diagram) siblings() [][]*Node {
	groups := map[*Node][]*Node{}
	var order []*Node
	for _, n := range d.Nodes {
		if n.Text || n.Overlay {
			continue
		}
		if _, ok := groups[n.Parent]; !ok {
			order = append(order, n.Parent)
		}
		groups[n.Parent] = append(groups[n.Parent], n)
	}
	out := make([][]*Node, 0, len(order))
	for _, p := range order {
		out = append(out, groups[p])
	}
	return out
}

// clusters splits nodes into runs whose centers on an axis lie within tol of
// the run's first member, sorted along the axis.
func clusters(ns []*Node, a axis, tol float64) [][]*Node {
	s := append([]*Node(nil), ns...)
	sort.SliceStable(s, func(i, j int) bool { return center(s[i].Box, a) < center(s[j].Box, a) })
	var out [][]*Node
	for i := 0; i < len(s); {
		j := i + 1
		for j < len(s) && center(s[j].Box, a)-center(s[i].Box, a) <= tol {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// Align lines up siblings whose centers are within tol on an axis: each run
// moves to the center of its largest member, which is usually the shape the
// others were placed against. Wires between shapes that end up level lose
// waypoints that only made a detour.
func (d *Diagram) Align(tol float64, log *Log) {
	for _, ax := range []axis{axisX, axisY} {
		for _, sib := range d.siblings() {
			var leaves []*Node
			for _, n := range sib {
				if !n.Container {
					leaves = append(leaves, n)
				}
			}
			for _, run := range clusters(leaves, ax, tol) {
				if len(run) < 2 {
					continue
				}
				anchor := run[0]
				for _, n := range run[1:] {
					if n.Box.W*n.Box.H > anchor.Box.W*anchor.Box.H {
						anchor = n
					}
				}
				target := center(anchor.Box, ax)
				for _, n := range run {
					delta := target - center(n.Box, ax)
					if math.Abs(delta) < 0.01 {
						continue
					}
					d.Move(n, vec(ax, delta))
					log.add("align", []string{n.V.ID, anchor.V.ID}, "lined %s up with %s", n.V.ID, anchor.V.ID)
				}
			}
		}
	}
	d.straighten(log)
}

// straighten drops the waypoints of wires that can now run as a straight line
// between level shapes without crossing anything.
func (d *Diagram) straighten(log *Log) {
	for _, w := range d.Wires {
		if len(w.Points) == 0 || w.Src == nil || w.Dst == nil {
			continue
		}
		a, b := w.Src.Box, w.Dst.Box
		levelX := math.Abs(a.Center().X-b.Center().X) < 0.5
		levelY := math.Abs(a.Center().Y-b.Center().Y) < 0.5
		if !levelX && !levelY {
			continue
		}
		snap := d.Save()
		before := d.Measure().Score
		w.Points = nil
		for _, k := range []string{"exitX", "exitY", "exitDx", "exitDy", "entryX", "entryY", "entryDx", "entryDy", "exitPerimeter", "entryPerimeter"} {
			w.Style.Del(k)
		}
		if d.broken(w) || d.Measure().Score > before {
			d.Restore(snap)
			continue
		}
		log.add("align", []string{w.V.Cell.ID}, "straightened wire %s", w.V.Cell.ID)
	}
}

// insideRoom is how much of a shape's box its label can use: a diamond holds
// text in its middle half, an ellipse in about seven tenths.
func insideRoom(shape string) float64 {
	switch shape {
	case "rhombus", "mxgraph.flowchart.decision":
		return 0.5
	case "ellipse", "doubleEllipse", "cloud":
		return 0.7
	}
	return 1
}

// labelInside reports whether a shape draws its label inside its box, which is
// the only case where its size should follow its text.
func labelInside(n *Node) bool {
	st := n.V.Style
	if st.Value("verticalLabelPosition", "middle") != "middle" || st.Value("labelPosition", "center") != "center" {
		return false
	}
	switch {
	case n.V.Shape == "image" || n.V.Shape == "umlActor" || st.Value("image", "") != "":
		return false
	case strings.HasPrefix(n.V.Shape, "mxgraph.") && !strings.HasPrefix(n.V.Shape, "mxgraph.flowchart."):
		return false
	}
	return true
}

// textPadding is the room kept around a label inside its shape.
const textPadding = 8

// resizeSlack is how far measured text may overflow a shape before it grows:
// the width table only approximates the fonts draw.io renders with.
const resizeSlack = 6

// Resize grows shapes whose text does not fit inside them, about their center.
// Shapes are never shrunk: a box drawn larger than its text was drawn so on
// purpose.
func (d *Diagram) Resize(log *Log) {
	for _, n := range d.solidNodes() {
		if n.V.Text == "" || !labelInside(n) {
			continue
		}
		tw, th := d.measure(n.V.Text, n.V.Style)
		if n.V.Style.Value("whiteSpace", "") == "wrap" && n.V.Style.Value("html", "0") == "1" {
			// Wrapped text only needs its longest word on one line; height then
			// follows the width the shape has.
			lines := d.wrapTo(n.V.Text, n.V.Style, n.Box.W*insideRoom(n.V.Shape)-2*textPadding)
			tw, th = d.measure(strings.Join(lines, "\n"), n.V.Style)
		}
		room := insideRoom(n.V.Shape)
		needW := tw/room + 2*textPadding
		needH := th/room + 2*textPadding
		nb := n.Box
		// Measuring is approximate, so only a clear overflow counts.
		if needW > nb.W+resizeSlack {
			nb.X -= (needW - nb.W) / 2
			nb.W = needW
		}
		if needH > nb.H+resizeSlack {
			nb.Y -= (needH - nb.H) / 2
			nb.H = needH
		}
		if nb != n.Box {
			log.add("resize", []string{n.V.ID}, "grew %s to %sx%s to fit its text", n.V.ID, geom.Format(nb.W), geom.Format(nb.H))
			n.Box = nb
		}
	}
}

// wrapTo wraps text at a width in the style's font size.
func (d *Diagram) wrapTo(t string, st doc.Style, width float64) []string {
	size := geom.ParseNumber(st.Value("fontSize", "12"))
	if size <= 0 {
		size = 12
	}
	scale := size / 12
	if width <= 0 {
		return splitLines(t)
	}
	return d.tm.Wrap(splitLines(t), width/scale)
}

// styleKey is a node's style without the keys that do not change how the
// shape looks, so nodes drawn alike compare equal.
func styleKey(n *Node) string {
	st := doc.ParseStyle(n.V.Style.String())
	for _, k := range []string{"whiteSpace", "html", "points", "fontStyle"} {
		st.Del(k)
	}
	return n.V.Shape + "|" + st.String()
}

// SameSize gives sibling shapes of the same style and similar size (within a
// third of each other) the size of the largest, about their centers.
func (d *Diagram) SameSize(log *Log) {
	for _, sib := range d.siblings() {
		groups := map[string][]*Node{}
		var keys []string
		for _, n := range sib {
			if n.Container || !labelInside(n) {
				continue
			}
			k := styleKey(n)
			if _, ok := groups[k]; !ok {
				keys = append(keys, k)
			}
			groups[k] = append(groups[k], n)
		}
		for _, k := range keys {
			g := groups[k]
			if len(g) < 2 {
				continue
			}
			maxW, maxH := 0.0, 0.0
			for _, n := range g {
				maxW, maxH = math.Max(maxW, n.Box.W), math.Max(maxH, n.Box.H)
			}
			for _, n := range g {
				if n.Box.W < 0.67*maxW || n.Box.H < 0.67*maxH {
					continue
				}
				if math.Abs(n.Box.W-maxW) < 0.5 && math.Abs(n.Box.H-maxH) < 0.5 {
					continue
				}
				c := n.Box.Center()
				n.Box = geom.Rect{X: c.X - maxW/2, Y: c.Y - maxH/2, W: maxW, H: maxH}
				log.add("samesize", []string{n.V.ID}, "sized %s like its peers (%sx%s)", n.V.ID, geom.Format(maxW), geom.Format(maxH))
			}
		}
	}
}

// Spacing evens out the gaps along each aligned row or column of three or more
// siblings, when the gaps are already similar (each within half of their
// median): those were meant to be equal.
//
// Gaps within tol of their median count as even already, so spacing does not
// fight the grid, which moves shapes by up to half a grid step.
func (d *Diagram) Spacing(tol float64, log *Log) {
	for _, ax := range []axis{axisX, axisY} {
		other := axisY
		if ax == axisY {
			other = axisX
		}
		for _, sib := range d.siblings() {
			var leaves []*Node
			for _, n := range sib {
				if !n.Container {
					leaves = append(leaves, n)
				}
			}
			for _, line := range clusters(leaves, other, 0.5) {
				if len(line) < 3 {
					continue
				}
				sort.SliceStable(line, func(i, j int) bool { return center(line[i].Box, ax) < center(line[j].Box, ax) })
				gaps := make([]float64, len(line)-1)
				for i := range gaps {
					gaps[i] = start(line[i+1].Box, ax) - end(line[i].Box, ax)
				}
				med := median(gaps)
				if med <= 0 {
					continue
				}
				similar, even := true, true
				for _, g := range gaps {
					if math.Abs(g-med) > med/2 {
						similar = false
					}
					if math.Abs(g-med) > tol {
						even = false
					}
				}
				if !similar || even {
					continue
				}
				for i := range gaps {
					g := start(line[i+1].Box, ax) - end(line[i].Box, ax)
					if delta := med - g; math.Abs(delta) >= 0.5 {
						d.Shift(ax, center(line[i+1].Box, ax), delta)
						log.add("spacing", []string{line[i+1].V.ID}, "evened the gap before %s to %s px", line[i+1].V.ID, geom.Format(med))
					}
				}
			}
		}
	}
}

func start(r geom.Rect, a axis) float64 {
	if a == axisX {
		return r.X
	}
	return r.Y
}

func end(r geom.Rect, a axis) float64 {
	if a == axisX {
		return r.Right()
	}
	return r.Bottom()
}

func median(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	if len(s) == 0 {
		return 0
	}
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// Compact closes empty bands wider than four minimum gaps, across the whole
// diagram, down to two minimum gaps. A band is empty when no shape, container
// edge or wire point lies in it, and nothing else on the page either.
func (d *Diagram) Compact(gap float64, log *Log) {
	for _, ax := range []axis{axisX, axisY} {
		for iter := 0; iter < 50; iter++ {
			type span struct{ lo, hi float64 }
			var spans []span
			for _, n := range d.Nodes {
				if n.Container {
					// Only a container's edges block; its inside may be empty.
					spans = append(spans, span{start(n.Box, ax), start(n.Box, ax) + 1}, span{end(n.Box, ax) - 1, end(n.Box, ax)})
					continue
				}
				spans = append(spans, span{start(n.Box, ax), end(n.Box, ax)})
			}
			for _, o := range d.Others {
				spans = append(spans, span{start(o, ax), end(o, ax)})
			}
			for _, w := range d.Wires {
				for _, p := range w.Path() {
					v := p.X
					if ax == axisY {
						v = p.Y
					}
					spans = append(spans, span{v, v})
				}
			}
			sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
			found := false
			reach := math.Inf(-1)
			for i, s := range spans {
				if i > 0 && s.lo-reach > 4*gap {
					d.Shift(ax, s.lo, -(s.lo - reach - 2*gap))
					log.add("compact", nil, "closed an empty band of %s px", geom.Format(s.lo-reach))
					found = true
					break
				}
				reach = math.Max(reach, s.hi)
			}
			if !found {
				break
			}
		}
	}
}

// Grid moves each shape so its center sits on the grid. Centers that were
// equal stay equal, so alignment survives.
func (d *Diagram) Grid(size float64, log *Log) {
	if size <= 0 {
		return
	}
	for _, n := range d.solidNodes() {
		if n.Parent != nil && d.stacked(n.Parent) {
			continue
		}
		c := n.Box.Center()
		to := geom.Point{X: math.Round(c.X/size) * size, Y: math.Round(c.Y/size) * size}
		if by := to.Sub(c); math.Abs(by.X) >= 0.01 || math.Abs(by.Y) >= 0.01 {
			d.Move(n, by)
			log.add("grid", []string{n.V.ID}, "moved %s onto the grid", n.V.ID)
		}
	}
}
