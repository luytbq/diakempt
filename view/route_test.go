package view

import (
	"testing"

	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
)

// TestRouteEntity checks ER relations against what draw.io's EntityRelation
// edge style draws: out of facing sides at the row's center, a 30px run, then
// straight across; waypoints are ignored.
func TestRouteEntity(t *testing.T) {
	st := doc.ParseStyle("edgeStyle=entityRelationEdgeStyle;endArrow=ERmany;startArrow=ERmandOne;")
	parent := geom.Rect{X: 0, Y: 0, W: 200, H: 120}
	child := geom.Rect{X: 300, Y: 100, W: 200, H: 150}
	row := geom.Rect{X: 300, Y: 200, W: 200, H: 30}
	cases := []struct {
		name string
		e    Ends
		want geom.Polyline
	}{
		{"apart", Ends{Src: &parent, Dst: &child},
			geom.Polyline{{X: 200, Y: 60}, {X: 230, Y: 60}, {X: 270, Y: 175}, {X: 300, Y: 175}}},
		{"to a row", Ends{Src: &parent, Dst: &child, DstPart: &row},
			geom.Polyline{{X: 200, Y: 60}, {X: 230, Y: 60}, {X: 270, Y: 215}, {X: 300, Y: 215}}},
		{"stacked", Ends{Src: &parent, Dst: &geom.Rect{X: 50, Y: 200, W: 200, H: 100}},
			geom.Polyline{{X: 200, Y: 60}, {X: 280, Y: 60}, {X: 280, Y: 250}, {X: 250, Y: 250}}},
	}
	for _, c := range cases {
		got := Route(c.e, []geom.Point{{X: 900, Y: 900}}, st)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i].Dist(c.want[i]) > 0.01 {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}
