package tidy

import "github.com/luytbq/diakempt/geom"

// End names one end of a wire.
type End int

const (
	Source End = iota
	Target
)

// prefix is the start of the style keys draw.io keeps an end's port in.
func (e End) prefix() string {
	if e == Source {
		return "exit"
	}
	return "entry"
}

// A Port is where a wire end meets its node: a point given as fractions of
// the node's width and height, plus an offset in pixels. Given tells draw.io
// to take the point as it is instead of projecting it onto the outline;
// draw.io applies the offset only to a point taken as given.
type Port struct {
	X, Y  float64
	Off   geom.Point
	Given bool
}

// SetPort fixes an end of the wire at a port of its node.
func (w *Wire) SetPort(e End, p Port) {
	k := e.prefix()
	w.Style.Set(k+"X", geom.Format(p.X))
	w.Style.Set(k+"Y", geom.Format(p.Y))
	w.Style.Set(k+"Dx", geom.Format(p.Off.X))
	w.Style.Set(k+"Dy", geom.Format(p.Off.Y))
	if p.Given {
		w.Style.Set(k+"Perimeter", "0")
	} else {
		w.Style.Del(k + "Perimeter")
	}
}

// ClearPorts lets both ends float, so draw.io joins the wire to each node
// where it meets the outline.
func (w *Wire) ClearPorts() {
	for _, e := range []End{Source, Target} {
		for _, k := range []string{"X", "Y", "Dx", "Dy", "Perimeter"} {
			w.Style.Del(e.prefix() + k)
		}
	}
}

// TransposePorts swaps the axes of both ports, for a layout computed with
// the diagram turned on its side.
func (w *Wire) TransposePorts() {
	for _, e := range []End{Source, Target} {
		for _, k := range [][2]string{{"X", "Y"}, {"Dx", "Dy"}} {
			a, aok := w.Style.Get(e.prefix() + k[0])
			b, bok := w.Style.Get(e.prefix() + k[1])
			if aok && bok {
				w.Style.Set(e.prefix()+k[0], b)
				w.Style.Set(e.prefix()+k[1], a)
			}
		}
	}
}
