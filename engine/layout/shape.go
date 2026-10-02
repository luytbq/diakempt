package layout

import (
	"math"

	"github.com/luytbq/diakempt/engine/num"
)

// Shape is the primitive shape of an element. Result and writers speak in
// primitive shapes, not semantic types, so writers need not know about kinds.
type Shape string

const (
	ShapeRect          Shape = "rect"
	ShapeDiamond       Shape = "diamond"
	ShapeEllipse       Shape = "ellipse"
	ShapeDoubleEllipse Shape = "double-ellipse"
	ShapeDashedEllipse Shape = "dashed-ellipse"
	ShapeCylinder      Shape = "cylinder"
	ShapeNote          Shape = "note"
)

// ElementGeometry is what the layout engine knows about one element type: its
// shape, its wiring rules, how its text wraps and how big its box gets. The
// engine and writers read only this declaration and never compare type names,
// so adding an element type means adding one entry to Geometry (and its table
// rules in schema.Elements).
//
// The outgoing wire rule is shared by all types, so it does not live here:
// outgoing wires leave only from the left, right or bottom side, never the top.
type ElementGeometry struct {
	Shape Shape
	// EntryTopOnly: incoming wires connect only at the top. The engine does not
	// place the element on the same row as its source and does not let wires
	// enter a lateral side horizontally.
	EntryTopOnly bool
	// StartsFlow: an element of this type with no source goes to the first row
	// rather than below everything placed so far.
	StartsFlow bool
	// Wrap is the width its text wraps at.
	Wrap func(cfg Config) float64
	// Size is the box, before rounding up to an even pixel, for text of the
	// given measured width and height.
	Size func(cfg Config, tw, th float64) (w, h float64)
}

// Geometry declares the geometry of each element type.
var Geometry = map[string]ElementGeometry{
	"task": {
		Shape: ShapeRect,
		// The wrap width leaves room for the left and right padding of the box.
		Wrap: func(cfg Config) float64 { return float64(cfg.TaskMaxW - 32) },
		Size: func(cfg Config, tw, th float64) (float64, float64) {
			return max(float64(cfg.TaskMinW), tw+32), max(50, th+20)
		},
	},
	"condition": {
		Shape: ShapeDiamond, EntryTopOnly: true,
		Wrap: func(cfg Config) float64 { return float64(cfg.CondWrap) },
		// Text fits inside the diamond when tw/W + th/H <= 1.
		Size: func(cfg Config, tw, th float64) (float64, float64) {
			return max(100, tw/0.6+10), max(60, th/0.4+10)
		},
	},
	"start":    {Shape: ShapeEllipse, StartsFlow: true, Wrap: termWrap, Size: ellipseSize(0)},
	"end":      {Shape: ShapeDoubleEllipse, Wrap: termWrap, Size: ellipseSize(12)},
	"external": {Shape: ShapeDashedEllipse, Wrap: termWrap, Size: ellipseSize(0)},
	"db": {
		Shape: ShapeCylinder,
		Wrap:  func(cfg Config) float64 { return float64(cfg.DBWrap) },
		Size: func(cfg Config, tw, th float64) (float64, float64) {
			return max(100, tw+24), max(60, th+34)
		},
	},
	"text": noteGeometry,
}

// noteGeometry is the geometry of a note, and of any type not declared.
var noteGeometry = ElementGeometry{
	Shape: ShapeNote,
	Wrap:  func(cfg Config) float64 { return float64(cfg.TextWrap) },
	Size:  func(cfg Config, tw, th float64) (float64, float64) { return tw + 20, th + 12 },
}

func termWrap(cfg Config) float64 { return float64(cfg.TermWrap) }

// ellipseSize is the size rule of the ellipse shapes. pad is extra room for an
// outer border, which the end shape has.
func ellipseSize(pad float64) func(Config, float64, float64) (float64, float64) {
	return func(_ Config, tw, th float64) (float64, float64) {
		return max(80, float64(tw*1.42)+24+pad), max(44, float64(th*1.42)+16+pad)
	}
}

// geometryOf returns the declaration of an element type. An undeclared type is
// drawn as a rectangle and sized like a note.
func geometryOf(elementType string) ElementGeometry {
	if g, ok := Geometry[elementType]; ok {
		return g
	}
	g := noteGeometry
	g.Shape = ShapeRect
	return g
}

// singlePort reports whether this shape has only one connection point, at the
// middle of each side. A slanted or curved outline has no straight stretch to
// split into ports like a rectangle's side, so several wires on the same side
// must converge on that side's vertex unless they are forced apart.
func (s Shape) singlePort() bool {
	switch s {
	case ShapeDiamond, ShapeEllipse, ShapeDoubleEllipse, ShapeDashedEllipse:
		return true
	}
	return false
}

// outline is the point on the shape's outline, on side side, at position t
// along that side, as fractions of the bounding box. For a rectangle the point
// lies right on the edge; for diamonds and ellipses the point moves inward from
// the bounding box until it meets the outline, so draw.io draws the wire end
// exactly where it was computed. Rounded to two digits as the writer outputs
// it, so that merge reading the file back sees ports matching the computed ones.
func (s Shape) outline(side byte, t float64) [2]float64 {
	inset := 0.0
	switch s {
	case ShapeDiamond:
		inset = math.Abs(t - 0.5)
	case ShapeEllipse, ShapeDoubleEllipse, ShapeDashedEllipse:
		d := float64(2*t) - 1
		inset = num.Round(0.5-float64(0.5*math.Sqrt(1-float64(d*d))), 2)
	}
	far := num.Round(1-inset, 2)
	switch side {
	case 'B':
		return [2]float64{t, far}
	case 'T':
		return [2]float64{t, inset}
	case 'R':
		return [2]float64{far, t}
	}
	return [2]float64{inset, t}
}
