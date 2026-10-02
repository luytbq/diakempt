// Package model holds the shared data types of the input table.
//
// It is separate from the root package because every source adapter needs these
// types, while the root package needs the adapters. The root package re-exports
// them with type aliases.
package model

// Row is a table row split into cells, not yet interpreted with diagram semantics.
type Row struct {
	// Idx is the position among the rows read, not the line number in the file.
	// Broken rows are skipped, so the two numbers differ.
	Idx    int
	Loc    Location
	ID     string
	Type   string
	Parent string
	Lines  []string
	Meta   map[string]string
	// MetaKeys is the order of keys as written in the cell. Needed because Go maps
	// iterate randomly, while warnings about unknown keys must follow the order the
	// user wrote them in.
	MetaKeys []string
}

// Text joins the content lines and trims whitespace. An empty result means the
// cell has no real text.
func (r Row) Text() string { return trimSpace(joinLines(r.Lines)) }

// Table is the result of reading one input source.
type Table struct {
	Title string
	Rows  []Row
	// Issues here are only findings of the reading layer. Checks against diagram
	// semantics live in the validate layer.
	Issues []Issue
	Source string
	// Direction is the direction the source declares itself, like mermaid's
	// flowchart LR line. Empty means the source says nothing.
	Direction string
}
