package tidy

import "github.com/luytbq/diakempt/doc"

// WireStyle turns each straight wire that still crosses a shape or runs on
// top of another wire into an orthogonal one with a right-angle route,
// keeping the change only if the diagram scores better (docs/adr/0005).
// Curved and orthogonal wires keep their style.
func (d *Diagram) WireStyle(log *Log) {
	for _, w := range d.Wires {
		if !straight(w.Style) || !d.broken(w) {
			continue
		}
		before := d.Measure().Score
		snap := d.Save()
		w.Style.Set("edgeStyle", "orthogonalEdgeStyle")
		if !d.routeOrthogonal(w) || d.Measure().Score >= before-1e-9 {
			d.Restore(snap)
			continue
		}
		w.Routed = true
		log.add("wirestyle", []string{w.V.Cell.ID}, "made wire %s orthogonal", w.V.Cell.ID)
	}
}

// straight reports a wire drawn as straight segments: no edge style, and not
// curved.
func straight(st doc.Style) bool {
	switch st.Value("edgeStyle", "none") {
	case "none", "":
	default:
		return false
	}
	return st.Value("curved", "0") != "1"
}
