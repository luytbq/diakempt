// Package report holds what a run tells its caller, and renders it as text.
// The core returns a File as data; the CLI prints it as text or JSON.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/luytbq/diakempt/issue"
)

// File is the report for one input file.
type File struct {
	Pages      int       `json:"pages"`
	Diagrams   []Diagram `json:"diagrams"`
	Decoration int       `json:"decoration"`
	Snap       Snap      `json:"snap"`
	// Normalized lists the structure rewrites done at the aggressive level.
	Normalized []Detail       `json:"normalized,omitempty"`
	Issues     []issue.Issue  `json:"issues,omitempty"`
	Changed    bool           `json:"changed"`
}

// Snap summarizes wire ends attached across the whole file.
type Snap struct {
	Snapped   int      `json:"snapped"`
	Ambiguous int      `json:"ambiguous"`
	Details   []Detail `json:"details,omitempty"`
}

// Diagram is the report for one diagram.
type Diagram struct {
	// ID is page, index on the page and a readable name: Page-1 #2 "Login".
	ID         string   `json:"id"`
	Page       int      `json:"page"`
	Index      int      `json:"index"`
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	Confidence string   `json:"confidence"`
	Signals    []string `json:"signals,omitempty"`
	// Forced is true when the kind came from the caller, not from detection.
	Forced bool `json:"forced,omitempty"`
	// Level is the level asked for; Applied the one whose result was kept, or
	// "none" when the original geometry was kept.
	Level      string     `json:"level"`
	Applied    string     `json:"applied"`
	StepDowns  []string   `json:"step_downs,omitempty"`
	Operations []OpCount  `json:"operations,omitempty"`
	Details    []Detail   `json:"details,omitempty"`
	Before     Metrics    `json:"before"`
	After      Metrics    `json:"after"`
}

// OpCount is how many changes one operation made.
type OpCount struct {
	Op    string `json:"op"`
	Count int    `json:"count"`
}

// Detail is one change, for --verbose.
type Detail struct {
	Op    string   `json:"op"`
	Cells []string `json:"cells"`
	Msg   string   `json:"message"`
}

// Metrics counts layout defects. Score weighs them into one number; lower is
// better.
type Metrics struct {
	WiresThroughNodes int     `json:"wires_through_nodes"`
	NodeOverlaps      int     `json:"node_overlaps"`
	LabelOverlaps     int     `json:"label_overlaps"`
	WireCrossings     int     `json:"wire_crossings"`
	WireOverlaps      int     `json:"wire_overlaps"`
	Area              float64 `json:"area"`
	WireLength        float64 `json:"wire_length"`
	Score             float64 `json:"score"`
}

// Text renders the report for people. name is the input file name.
func (f File) Text(name string, verbose bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s, %s", name, plural(f.Pages, "page"), plural(len(f.Diagrams), "diagram"))
	if f.Decoration > 0 {
		fmt.Fprintf(&b, ", %s", plural(f.Decoration, "decoration element"))
	}
	b.WriteString("\n")
	if f.Snap.Snapped > 0 || f.Snap.Ambiguous > 0 {
		fmt.Fprintf(&b, "  snapped %s", plural(f.Snap.Snapped, "wire end"))
		if f.Snap.Ambiguous > 0 {
			fmt.Fprintf(&b, ", skipped %d ambiguous", f.Snap.Ambiguous)
		}
		b.WriteString("\n")
		if verbose {
			writeDetails(&b, "    ", f.Snap.Details)
		}
	}
	for _, d := range f.Diagrams {
		fmt.Fprintf(&b, "  %s: %s", d.ID, d.Kind)
		if d.Forced {
			b.WriteString(" (forced)")
		} else if d.Confidence != "" {
			fmt.Fprintf(&b, " (%s confidence)", d.Confidence)
		}
		fmt.Fprintf(&b, ", level %s", d.Level)
		if d.Applied != d.Level {
			fmt.Fprintf(&b, ", applied %s", d.Applied)
		}
		b.WriteString("\n")
		if verbose && len(d.Signals) > 0 {
			fmt.Fprintf(&b, "    signals: %s\n", strings.Join(d.Signals, ", "))
		}
		for _, s := range d.StepDowns {
			fmt.Fprintf(&b, "    stepped down: %s\n", s)
		}
		if len(d.Operations) > 0 {
			parts := make([]string, len(d.Operations))
			for i, op := range d.Operations {
				parts[i] = fmt.Sprintf("%s %d", op.Op, op.Count)
			}
			fmt.Fprintf(&b, "    changes: %s\n", strings.Join(parts, ", "))
		} else {
			b.WriteString("    changes: none\n")
		}
		fmt.Fprintf(&b, "    before: %s\n    after:  %s\n", d.Before.text(), d.After.text())
		if verbose {
			writeDetails(&b, "    ", d.Details)
		}
	}
	if verbose {
		for _, n := range f.Normalized {
			fmt.Fprintf(&b, "  normalized: %s\n", n.Msg)
		}
	}
	issues := append([]issue.Issue(nil), f.Issues...)
	sort.SliceStable(issues, func(i, j int) bool { return sevRank(issues[i].Severity) > sevRank(issues[j].Severity) })
	for _, is := range issues {
		if is.Severity == issue.Info && !verbose {
			continue
		}
		fmt.Fprintf(&b, "  %s: %s [%s]\n", is.Severity, is.Msg, is.Code)
	}
	return b.String()
}

func (m Metrics) text() string {
	return fmt.Sprintf("%d wires through nodes, %d node overlaps, %d label overlaps, %d wire crossings, %d wire overlaps",
		m.WiresThroughNodes, m.NodeOverlaps, m.LabelOverlaps, m.WireCrossings, m.WireOverlaps)
}

func writeDetails(b *strings.Builder, indent string, ds []Detail) {
	for _, d := range ds {
		fmt.Fprintf(b, "%s%s: %s\n", indent, d.Op, d.Msg)
	}
}

func sevRank(s issue.Severity) int {
	switch s {
	case issue.Error:
		return 2
	case issue.Warning:
		return 1
	}
	return 0
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// Warnings counts the warnings and errors in the report.
func (f File) Warnings() int {
	n := 0
	for _, is := range f.Issues {
		if is.Severity != issue.Info {
			n++
		}
	}
	return n
}

// Worse reports whether any diagram scored worse after than before.
func (f File) Worse() bool {
	for _, d := range f.Diagrams {
		if d.After.Score > d.Before.Score+1e-9 {
			return true
		}
	}
	return false
}
