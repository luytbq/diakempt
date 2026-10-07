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
	place(layers, links, rank)
	transpose(nodes)
	keepCorner(d, nodes)
	for _, l := range links {
		if view.IsEntityRelation(l.w.Style) {
			l.w.Points = nil
			continue
		}
		d.RouteWire(l.w)
	}
	return nil
}

// transpose swaps the axes of the nodes' boxes, current and original.
func transpose(nodes []*tidy.Node) {
	flip := func(r geom.Rect) geom.Rect { return geom.Rect{X: r.Y, Y: r.X, W: r.H, H: r.W} }
	for _, n := range nodes {
		n.Box, n.Orig = flip(n.Box), flip(n.Orig)
	}
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
