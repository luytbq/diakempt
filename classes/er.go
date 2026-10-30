package classes

import (
	"strings"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/view"
)

// LayoutER places the tables of an entity relationship diagram in columns,
// read left to right: a table one column right of the tables on the "one"
// side of its relations, so foreign keys point left. Tables keep their size.
//
// draw.io routes ER relations from the table sides by itself and ignores
// waypoints, so those wires only lose any stale waypoints; other wires are
// routed around the tables.
//
// The placement is the class layout turned on its side: ranks become
// columns, and the order within a rank runs top to bottom.
func LayoutER(d *tidy.Diagram) error {
	nodes, links, err := collect(d, "tables", classifyER)
	if err != nil {
		return err
	}
	transpose(nodes)
	rank := ranks(nodes, links)
	layers := order(nodes, links, rank)
	// draw.io routes ER relations itself; the rest get the class diagram's
	// channels, turned on their side with everything else.
	var own []*link
	for _, l := range links {
		if view.IsEntityRelation(l.w.Style) {
			l.w.Points = nil
		} else {
			own = append(own, l)
		}
	}
	place(layers, own, rank, gaps(layers, own, rank))
	rest := route(layers, own, rank)
	transpose(nodes)
	inRest := map[*link]bool{}
	for _, l := range rest {
		inRest[l] = true
	}
	for _, l := range own {
		if !inRest[l] {
			transposeWire(l.w)
		}
	}
	keepCorner(d, nodes)
	for _, l := range rest {
		d.RouteWire(l.w)
	}
	return nil
}

// transpose swaps the axes of the nodes' boxes.
func transpose(nodes []*tidy.Node) {
	for _, n := range nodes {
		n.Box = geom.Rect{X: n.Box.Y, Y: n.Box.X, W: n.Box.H, H: n.Box.W}
	}
}

// transposeWire swaps the axes of a wire's waypoints and ports.
func transposeWire(w *tidy.Wire) {
	for i, p := range w.Points {
		w.Points[i] = geom.Point{X: p.Y, Y: p.X}
	}
	w.TransposePorts()
}

// classifyER reads a relation's cardinality from its ER arrowheads: the table
// at a "one" end is the parent, drawn left of the table at a "many" end.
// One-to-one and many-to-many relations do not decide columns.
func classifyER(w *tidy.Wire) *link {
	l := &link{w: w}
	src := erEnd(w.Style.Value("startArrow", "none"))
	dst := erEnd(w.Style.Value("endArrow", "classic"))
	switch {
	case src == one && dst == many:
		l.rel, l.up, l.down = whole, w.Src, w.Dst
	case src == many && dst == one:
		l.rel, l.up, l.down = whole, w.Dst, w.Src
	}
	if l.up == l.down {
		l.rel = free
	}
	return l
}

type cardinality int

const (
	unknown cardinality = iota
	one
	many
)

// erEnd reads the cardinality an ER arrowhead draws.
func erEnd(arrow string) cardinality {
	switch {
	case !strings.HasPrefix(arrow, "ER"):
		return unknown
	case strings.Contains(arrow, "Many"), arrow == "ERmany":
		return many
	case strings.Contains(arrow, "One"), arrow == "ERone":
		return one
	}
	return unknown
}
