// Package schema declares the valid metadata keys for each row type.
//
// The Flow Table keeps exactly five columns, so the metadata column carries both
// required references such as from, to, attach and optional keys such as style.
// The explicitness therefore cannot live in the table; it lives here. See
// docs/adr/0001.
package schema

// Row types, derived from Elements. NodeTypes are always in the flow.
// AttachTypes may carry attach and then stand beside the element they attach
// to; nothing requires them to.
var (
	NodeTypes   = elementSet(func(e Element) bool { return e.AlwaysInFlow })
	AttachTypes = elementSet(func(e Element) bool { return e.MayAttach })
	AllTypes    = union(NodeTypes, AttachTypes, set("lane", "edge"))
)

// ReservedIDs are ids draw.io uses itself, which must not be reused.
var ReservedIDs = set("0", "1", "pool")

// RestMarker is the content of the text row that marks the rest of the table.
const RestMarker = "Rest of the table"

// Ref is a metadata key that points to the id of another row.
type Ref struct {
	Key      string
	Required bool
	// Note is the sentence tail used when the reference points to a type it
	// cannot accept. It lives here because the message is part of the interface:
	// users read it, and the conformance gate compares it verbatim.
	Note string
	// SameLane set means warn when the target is in another lane.
	SameLane bool
}

// TypeSpec is the metadata schema of one row type.
type TypeSpec struct {
	// Keys are the allowed keys, in declaration order.
	Keys        []string
	StyleValues []string
	// Refs are in exactly the order they must be checked, because the order of
	// findings is part of the interface.
	Refs []Ref
}

const edgeNote = "edges only connect start/end/task/condition/external or a db without attach"
const attachNote = "can only attach to start/end/task/condition/external or a db without attach"

// defaultSpec applies to every node type: only style is accepted, and style only accepts highlight.
var defaultSpec = TypeSpec{Keys: []string{"style"}, StyleValues: []string{"highlight"}}

// attachSpec applies to the element types that may attach to another element.
var attachSpec = TypeSpec{
	Keys:        []string{"attach", "style"},
	StyleValues: []string{"highlight"},
	Refs:        []Ref{{Key: "attach", Note: attachNote, SameLane: true}},
}

var swimlane = map[string]TypeSpec{
	"lane": {Keys: nil, StyleValues: []string{"highlight"}},
	"edge": {
		Keys:        []string{"from", "to", "style", "back"},
		StyleValues: []string{"highlight", "dashed", "bold", "noarrow"},
		Refs: []Ref{
			{Key: "from", Required: true, Note: edgeNote},
			{Key: "to", Required: true, Note: edgeNote},
		},
	},
}

// InFlow reports whether a row of this type and metadata takes part in the flow:
// it can be the end of an edge, has its own row, column and outgoing edges, and
// other elements can attach to it. A node always does; a db does when it does
// not attach to anything.
func InFlow(elementType string, meta map[string]string) bool {
	e := Elements[elementType]
	return e.AlwaysInFlow || (e.InFlowUnattached && meta["attach"] == "")
}

// For returns the metadata schema of a row type.
func For(rowType string) TypeSpec {
	if s, ok := swimlane[rowType]; ok {
		return s
	}
	if e, ok := Elements[rowType]; ok && e.MayAttach {
		return attachSpec
	}
	return defaultSpec
}

func (s TypeSpec) AllowsKey(k string) bool   { return contains(s.Keys, k) }
func (s TypeSpec) AllowsStyle(v string) bool { return contains(s.StyleValues, v) }

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func union(ms ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		for k := range m {
			out[k] = true
		}
	}
	return out
}
