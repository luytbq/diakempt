package schema

import "github.com/luytbq/diakempt/engine/model"

// Element is what the Flow Table format knows about one element type: how it
// relates to the flow and which graph rules apply to it. validate reads this
// declaration and never compares type names, so adding an element type means
// adding an entry here (and its geometry in layout.Geometry).
type Element struct {
	// AlwaysInFlow: the element is a step of the flow: edges connect to it, it
	// has its own row and column, and other elements can attach to it.
	AlwaysInFlow bool
	// MayAttach: the element may carry attach and then stand beside another
	// element instead of taking part in the flow.
	MayAttach bool
	// InFlowUnattached: without attach, the element is a step of the flow.
	InFlowUnattached bool
	// MinOutEdges is the fewest outgoing edges the element must have; fewer is
	// an error.
	MinOutEdges int
	// WarnNoOutEdge: an element with no outgoing edge gets a warning, since the
	// flow stops there without saying so.
	WarnNoOutEdge bool
	// WarnInEdge and WarnOutEdge: incoming or outgoing edges are suspicious.
	WarnInEdge, WarnOutEdge bool
	// LabelBranches: every outgoing edge should carry a label, since each one
	// is a different answer.
	LabelBranches bool
}

// Elements declares every element type of the activity-swimlane format.
var Elements = map[string]Element{
	"start":     {AlwaysInFlow: true, WarnInEdge: true},
	"end":       {AlwaysInFlow: true, WarnOutEdge: true},
	"task":      {AlwaysInFlow: true, WarnNoOutEdge: true},
	"condition": {AlwaysInFlow: true, MinOutEdges: 2, WarnNoOutEdge: true, LabelBranches: true},
	"external":  {AlwaysInFlow: true},
	"db":        {MayAttach: true, InFlowUnattached: true},
	"text":      {MayAttach: true},
}

func elementSet(keep func(Element) bool) map[string]bool {
	out := map[string]bool{}
	for name, e := range Elements {
		if keep(e) {
			out[name] = true
		}
	}
	return out
}

// IsRestMarker recognizes the text row that marks the rest of the table. That
// row is not an element: it needs no parent and is not drawn.
func IsRestMarker(r model.Row) bool {
	return r.Type == "text" && r.Text() == RestMarker && r.Meta["attach"] == ""
}
