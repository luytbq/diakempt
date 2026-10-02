// Package validate checks a parsed table against the metadata schema and the
// specific rules of the activity-swimlane diagram.
package validate

import (
	"fmt"
	"github.com/luytbq/diakempt/engine/unistr"
	"strings"

	"github.com/luytbq/diakempt/engine/model"
	"github.com/luytbq/diakempt/engine/schema"
)

// Messages write quotes literally instead of using %q. %q escapes special
// characters, so a metadata key with a backslash would print differently from
// what the user wrote in the table.

// Validate returns every finding, in a fixed order.
//
// The order is part of the interface: callers print findings in exactly this
// order, and the conformance gate compares both order and content.
func Validate(rows []model.Row) []model.Issue {
	v := &validator{rows: rows, byID: map[string]int{}}
	v.checkIDs()
	v.checkRows()
	v.checkGraph()
	return v.issues
}

type validator struct {
	rows   []model.Row
	issues []model.Issue
	// byID holds the index of the first row with that id. Later rows with a
	// duplicate id are skipped by the graph checks.
	byID  map[string]int
	lanes map[string]bool
}

func (v *validator) err(r model.Row, code, msg string) {
	v.issues = append(v.issues, model.Issue{Code: code, Level: model.LevelError, Loc: r.Loc, ID: r.ID, Msg: msg})
}

func (v *validator) warn(r model.Row, code, msg string) {
	v.issues = append(v.issues, model.Issue{Code: code, Level: model.LevelWarning, Loc: r.Loc, ID: r.ID, Msg: msg})
}

func (v *validator) row(id string) (model.Row, bool) {
	i, ok := v.byID[id]
	if !ok {
		return model.Row{}, false
	}
	return v.rows[i], true
}

func (v *validator) isNode(id string) bool {
	r, ok := v.row(id)
	return ok && schema.InFlow(r.Type, r.Meta)
}

func (v *validator) checkIDs() {
	for i, r := range v.rows {
		if r.ID == "" {
			v.err(r, "table.empty_id", "empty id")
			continue
		}
		if schema.ReservedIDs[r.ID] {
			v.err(r, "table.reserved_id", fmt.Sprintf("id \"%s\" collides with a draw.io reserved id", r.ID))
		}
		if j, dup := v.byID[r.ID]; dup {
			v.err(r, "ref.duplicate_id", "id duplicates "+v.rows[j].Loc.String())
		} else {
			v.byID[r.ID] = i
		}
		if !schema.AllTypes[r.Type] {
			v.err(r, "table.bad_type", fmt.Sprintf("invalid type: \"%s\"", r.Type))
		}
	}
	v.lanes = map[string]bool{}
	for _, r := range v.rows {
		if r.Type == "lane" {
			v.lanes[r.ID] = true
		}
	}
}

func (v *validator) checkRows() {
	for _, r := range v.rows {
		if !schema.AllTypes[r.Type] {
			continue
		}
		switch {
		case r.Type == "lane" || r.Type == "edge":
			if r.Parent != "" {
				v.err(r, "table.parent_not_empty", fmt.Sprintf("%s must leave parent empty", r.Type))
			}
		case !schema.IsRestMarker(r):
			// A table without lanes is a flowchart: every element shares one area
			// and leaves parent empty.
			if r.Parent == "" && len(v.lanes) == 0 {
				break
			}
			if r.Parent == "" {
				v.err(r, "ref.missing_parent", "missing parent (id of the lane containing the element)")
			} else if !v.lanes[r.Parent] {
				v.err(r, "ref.parent_not_lane", fmt.Sprintf("parent \"%s\" is not the id of a lane", r.Parent))
			}
		}

		spec := schema.For(r.Type)
		for _, k := range r.MetaKeys {
			if !spec.AllowsKey(k) {
				v.warn(r, "schema.unknown_key",
					fmt.Sprintf("metadata \"%s\" does not apply to type %s, ignored", k, r.Type))
			}
		}
		for _, s := range styleList(r) {
			if !spec.AllowsStyle(s) {
				v.warn(r, "schema.bad_style",
					fmt.Sprintf("style \"%s\" does not apply to type %s, ignored", s, r.Type))
			}
		}
		if b, ok := r.Meta["back"]; ok && b != "true" && b != "false" {
			v.warn(r, "schema.bad_back", fmt.Sprintf("back=\"%s\" is invalid, only true/false are accepted", b))
		}
		v.checkRefs(r, spec)
	}
}

func (v *validator) checkRefs(r model.Row, spec schema.TypeSpec) {
	for _, ref := range spec.Refs {
		val := r.Meta[ref.Key]
		if val == "" {
			if ref.Required {
				v.err(r, "schema.missing_key", fmt.Sprintf("%s is missing %s", r.Type, ref.Key))
			}
			continue
		}
		target, ok := v.row(val)
		if !ok {
			v.err(r, "ref.dangling", fmt.Sprintf("%s=%s points to an id that does not exist", ref.Key, val))
			continue
		}
		if !schema.InFlow(target.Type, target.Meta) {
			v.err(r, "ref.bad_target",
				fmt.Sprintf("%s=%s is %s; %s", ref.Key, val, target.Type, ref.Note))
			continue
		}
		if ref.SameLane && target.Parent != r.Parent {
			v.warn(r, "ref.other_lane",
				fmt.Sprintf("is in lane %s but attaches to %s in lane %s; will be drawn in lane %s",
					r.Parent, val, target.Parent, target.Parent))
		}
	}
}

// styleList returns the style values in the order written in the cell, without duplicates.
func styleList(r model.Row) []string {
	var out []string
	for _, s := range strings.Split(r.Meta["style"], ",") {
		s = unistr.Strip(s)
		if s == "" {
			continue
		}
		dup := false
		for _, x := range out {
			if x == s {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}

func (v *validator) checkGraph() {
	var edges []model.Row
	outs := map[string][]model.Row{}
	ins := map[string][]model.Row{}
	for _, r := range v.rows {
		if r.Type != "edge" || !v.isNode(r.Meta["from"]) || !v.isNode(r.Meta["to"]) {
			continue
		}
		edges = append(edges, r)
		outs[r.Meta["from"]] = append(outs[r.Meta["from"]], r)
		ins[r.Meta["to"]] = append(ins[r.Meta["to"]], r)
	}
	// For a duplicate id, the position is taken from the last row with that id.
	pos := map[string]int{}
	for i, r := range v.rows {
		if r.ID != "" {
			pos[r.ID] = i
		}
	}

	for i, r := range v.rows {
		if !schema.InFlow(r.Type, r.Meta) {
			continue
		}
		if j, ok := v.byID[r.ID]; !ok || j != i {
			continue
		}
		es := outs[r.ID]
		el := schema.Elements[r.Type]
		if len(es) < el.MinOutEdges {
			v.err(r, "graph.condition_branches",
				fmt.Sprintf("%s has only %d outgoing edges, needs at least %d", r.Type, len(es), el.MinOutEdges))
		}
		if el.WarnNoOutEdge && len(es) == 0 {
			v.warn(r, "graph.no_out_edge", fmt.Sprintf("%s has no outgoing edge", r.Type))
		}
		if el.WarnInEdge && len(ins[r.ID]) > 0 {
			v.warn(r, "graph.start_has_in", r.Type+" has incoming edges: "+ids(ins[r.ID]))
		}
		if el.WarnOutEdge && len(es) > 0 {
			v.warn(r, "graph.end_has_out", r.Type+" has outgoing edges: "+ids(es))
		}
		if el.LabelBranches {
			for _, e := range es {
				if e.Text() == "" {
					v.warn(e, "graph.branch_no_label",
						fmt.Sprintf("outgoing edge of %s %s has no label", r.Type, r.ID))
				}
			}
		}
		v.checkOutEdgeOrder(r, es, pos)
	}

	for _, e := range edges {
		if e.Meta["back"] == "true" {
			continue
		}
		if pos[e.Meta["to"]] < pos[e.Meta["from"]] {
			v.warn(e, "order.missing_back",
				fmt.Sprintf("%s comes before its source %s but the edge does not say back=true",
					e.Meta["to"], e.Meta["from"]))
		}
	}
}

// checkOutEdgeOrder enforces the reading rule of the format: the outgoing edges
// of an element must come right after that element, with only the db and text
// attached to it allowed in between.
func (v *validator) checkOutEdgeOrder(r model.Row, es []model.Row, pos map[string]int) {
	if len(es) == 0 {
		return
	}
	p := pos[r.ID] + 1
	for p < len(v.rows) && schema.AttachTypes[v.rows[p].Type] && v.rows[p].Meta["attach"] == r.ID {
		p++
	}
	block := map[string]bool{}
	for k := p; k < len(v.rows) && k < p+len(es); k++ {
		block[v.rows[k].ID] = true
	}
	var misplaced []string
	for _, e := range es {
		if !block[e.ID] {
			misplaced = append(misplaced, e.ID)
		}
	}
	if len(misplaced) == 0 && len(block) == len(es) {
		return
	}
	v.err(r, "order.out_edges_not_adjacent",
		fmt.Sprintf("outgoing edges must come right after %s (and the db/text attached to it); out of place: %s",
			r.ID, strings.Join(misplaced, ", ")))
}

func ids(rows []model.Row) string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return strings.Join(out, ", ")
}
