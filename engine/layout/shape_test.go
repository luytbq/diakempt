package layout

import (
	"testing"

	"github.com/luytbq/diakempt/engine/schema"
)

func TestEveryElementTypeHasGeometryDeclaration(t *testing.T) {
	for _, set := range []map[string]bool{schema.NodeTypes, schema.AttachTypes} {
		for kind := range set {
			if _, ok := Geometry[kind]; !ok {
				t.Errorf("type %s has no declaration in Geometry", kind)
			}
		}
	}
}

func TestOutlinePointLiesOnShape(t *testing.T) {
	for _, tc := range []struct {
		shape Shape
		side  byte
		t     float64
		want  [2]float64
	}{
		{ShapeRect, 'L', 0.25, [2]float64{0, 0.25}},
		{ShapeDiamond, 'L', 0.25, [2]float64{0.25, 0.25}},
		{ShapeDiamond, 'B', 0.75, [2]float64{0.75, 0.75}},
		{ShapeEllipse, 'R', 0.75, [2]float64{0.93, 0.75}},
		{ShapeEllipse, 'L', 0.5, [2]float64{0, 0.5}},
	} {
		if got := tc.shape.outline(tc.side, tc.t); got != tc.want {
			t.Errorf("%s side %c at %v: %v, want %v", tc.shape, tc.side, tc.t, got, tc.want)
		}
	}
}
