package diakempt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/luytbq/diakempt/classes"
	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/issue"
	"github.com/luytbq/diakempt/normalize"
	"github.com/luytbq/diakempt/relayout"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/sequence"
	"github.com/luytbq/diakempt/snap"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/view"
)

// Result is the outcome of tidying one file.
type Result struct {
	// Output is the tidied file. When nothing changed it is the input written
	// back, equivalent to it.
	Output []byte
	Report report.File
}

// Tidy tidies one draw.io file. An error means the file could not be processed
// at all; it is an issue.Issue carrying a stable code.
func Tidy(data []byte, opt Options) (Result, error) {
	if err := opt.Validate(); err != nil {
		return Result{}, err
	}
	d, warns, err := doc.Parse(data)
	if err != nil {
		return Result{}, err
	}
	rep := report.File{Pages: len(d.Pages), Issues: warns}
	tm := text.Default()
	for _, p := range d.Pages {
		if p.Empty() {
			continue
		}
		// One pass can make work for another: moving shapes brings a free wire
		// end within snap reach, and a new attachment joins two diagrams. Passes
		// repeat until one changes nothing, so tidying the output again finds
		// nothing to do.
		var merged *pageReport
		for pass := 0; pass < maxPasses; pass++ {
			pr := tidyPage(p, tm, opt, pass == 0)
			if merged == nil {
				merged = pr
			} else {
				merged.absorb(pr)
			}
			if !pr.changed {
				break
			}
		}
		merged.into(&rep)
	}
	rep.Snap.Ambiguous = countCode(rep.Issues, "snap.ambiguous")
	return Result{Output: d.Bytes(), Report: rep}, nil
}

// maxPasses bounds the passes over one page.
const maxPasses = 3

// pageReport is what one or more passes over a page found and did.
type pageReport struct {
	snap       report.Snap
	normalized []report.Detail
	issues     []issue.Issue
	diagrams   []report.Diagram
	decoration int
	changed    bool
}

// tidyPage runs one pass over a page: snap, normalize at aggressive, then each
// diagram. Normalizing only happens on the first pass.
func tidyPage(p *doc.Page, tm *text.Measure, opt Options, first bool) *pageReport {
	lv := opt.level()
	pr := &pageReport{}
	if opt.enabled("snap", lv) {
		r := snap.Run(view.Build(p), snapParams(opt))
		pr.snap.Snapped, pr.snap.Details = r.Snapped, r.Details
		pr.issues = r.Issues
		pr.changed = r.Snapped > 0
	}
	if first && lv == Aggressive && opt.enabled("normalize", lv) {
		if ds := normalize.Run(p); len(ds) > 0 {
			pr.normalized = ds
			pr.changed = true
		}
	}
	seg := segment.Split(view.Build(p))
	pr.decoration = len(seg.Decoration) + len(seg.LooseWires)
	for _, sd := range seg.Diagrams {
		dr, changed := tidyDiagram(sd, others(seg, sd), tm, lv, opt)
		pr.diagrams = append(pr.diagrams, dr)
		pr.changed = pr.changed || changed
	}
	return pr
}

// absorb folds a later pass into this report. Diagrams are those of the last
// pass; each keeps the metrics from before the first pass that saw it, and the
// changes of every pass add up.
func (pr *pageReport) absorb(later *pageReport) {
	pr.snap.Snapped += later.snap.Snapped
	pr.snap.Details = append(pr.snap.Details, later.snap.Details...)
	pr.normalized = append(pr.normalized, later.normalized...)
	pr.issues = mergeIssues(pr.issues, later.issues)
	pr.decoration = later.decoration
	pr.changed = pr.changed || later.changed
	earlier := map[string]report.Diagram{}
	for _, d := range pr.diagrams {
		earlier[d.ID] = d
	}
	var out []report.Diagram
	for _, d := range later.diagrams {
		// A diagram that gained or lost members between passes is a different
		// diagram; its own figures stand.
		if e, ok := earlier[d.ID]; ok && e.Shapes == d.Shapes && e.Wires == d.Wires {
			d.Before = e.Before
			d.Operations = addOps(e.Operations, d.Operations)
			d.Details = append(e.Details, d.Details...)
			d.StepDowns = addNew(e.StepDowns, d.StepDowns)
			if e.Applied != "none" {
				d.Applied = e.Applied
			}
		}
		out = append(out, d)
	}
	pr.diagrams = out
}

// addNew appends the entries of b that a does not hold yet: a later pass that
// retries the same attempt gives the same reason.
func addNew(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, s := range b {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func addOps(a, b []report.OpCount) []report.OpCount {
	count := map[string]int{}
	for _, o := range append(append([]report.OpCount(nil), a...), b...) {
		count[o.Op] += o.Count
	}
	var out []report.OpCount
	for _, name := range opNames() {
		if n := count[name]; n > 0 {
			out = append(out, report.OpCount{Op: name, Count: n})
		}
	}
	return out
}

func (pr *pageReport) into(rep *report.File) {
	rep.Snap.Snapped += pr.snap.Snapped
	rep.Snap.Details = append(rep.Snap.Details, pr.snap.Details...)
	rep.Normalized = append(rep.Normalized, pr.normalized...)
	rep.Issues = mergeIssues(rep.Issues, pr.issues)
	rep.Diagrams = append(rep.Diagrams, pr.diagrams...)
	rep.Decoration += pr.decoration
	rep.Changed = rep.Changed || pr.changed
}

// tidyDiagram optimizes one diagram at the requested level, stepping down a
// level whenever the result scores worse than the original or breaks relative
// order where the level must keep it (docs/adr/0004). It patches the document
// with the kept result.
func tidyDiagram(sd *segment.Diagram, rest []geom.Rect, tm *text.Measure, lv Level, opt Options) (report.Diagram, bool) {
	w := tidy.New(sd, tm)
	w.Others = rest
	det := detect.Detect(sd)
	dr := report.Diagram{
		ID: sd.ID(), Page: sd.Page, Index: sd.Index, Name: sd.Name, Shapes: len(sd.Nodes), Wires: len(sd.Wires),
		Kind: det.Kind, Confidence: det.Confidence, Signals: det.Signals,
		Level: string(lv), Applied: "none",
	}
	if opt.Kind != "" {
		dr.Kind, dr.Forced, dr.Confidence = opt.Kind, true, ""
	}
	orig := w.Save()
	dr.Before = w.Measure()
	dr.After = dr.Before
	// Kinds with an optimizer of their own that works on the working copy.
	for _, k := range []struct {
		kind, op, as string
		lay          func(*tidy.Diagram) error
	}{
		{detect.Class, "classlayout", "a class diagram", classes.Layout},
		{detect.Sequence, "seqlayout", "a sequence diagram", sequence.Layout},
	} {
		if dr.Kind != k.kind || !opt.enabled(k.op, lv) {
			continue
		}
		if err := k.lay(w); err != nil {
			dr.StepDowns = append(dr.StepDowns, k.op+" not possible: "+err.Error())
			w.Restore(orig)
		} else if after := w.Measure(); worse(after, dr.Before) && !opt.Force {
			dr.StepDowns = append(dr.StepDowns, fmt.Sprintf("%s scored %.1f against %.1f before", k.op, after.Score, dr.Before.Score))
			w.Restore(orig)
		} else {
			log := &tidy.Log{Counts: map[string]int{k.op: 1}}
			dr.Applied, dr.After = string(lv), after
			dr.Operations = log.Ops(opNames())
			dr.Details = []report.Detail{{Op: k.op, Msg: "laid out as " + k.as}}
			if w.Changed() {
				w.Patch()
				return dr, true
			}
			return unchanged(dr), false
		}
	}
	flow := (dr.Kind == detect.Flowchart || dr.Kind == detect.Swimlane) && opt.enabled("flowlayout", lv)
	free := dr.Kind == detect.Unknown && opt.enabled("relayout", lv)
	if flow || free {
		op := "flowlayout"
		withLanes := dr.Kind == detect.Swimlane
		if free {
			op = "relayout"
			withLanes = hasLanes(sd)
		}
		reason := ""
		plan, err := relayout.Run(w, sd, withLanes, tm)
		if err != nil {
			reason = op + " not possible: " + err.Error()
		} else if after := w.Measure(); worse(after, dr.Before) && !opt.Force {
			reason = fmt.Sprintf("%s scored %.1f against %.1f before", op, after.Score, dr.Before.Score)
		} else {
			log := &tidy.Log{}
			log.Counts = map[string]int{op: 1}
			dr.Applied = string(lv)
			dr.After = after
			dr.Operations = log.Ops(opNames())
			as := dr.Kind
			if free {
				as = "flowchart"
				if withLanes {
					as = "swimlane"
				}
			}
			dr.Details = []report.Detail{{Op: op, Msg: fmt.Sprintf("laid out as a %s, direction %s", as, plan.Dir)}}
			if w.Changed() {
				w.Patch()
				return dr, true
			}
			return unchanged(dr), false
		}
		dr.StepDowns = append(dr.StepDowns, reason)
		w.Restore(orig)
	}
	// A flow the engine could not improve gets only the safe operations: the
	// shape operations of normal are for diagrams no optimizer understands.
	top := lv.rank()
	if (dr.Kind == detect.Flowchart || dr.Kind == detect.Swimlane) && top > Safe.rank() {
		top = Safe.rank()
	}
	for rank := top; rank >= 0; rank-- {
		at := Levels[rank]
		w.Restore(orig)
		log := &tidy.Log{}
		runLevel(w, at, opt, log)
		after := w.Measure()
		reason := ""
		if at != Aggressive {
			if err := w.OrderKept(); err != nil {
				reason = fmt.Sprintf("%s broke relative order (%v)", at, err)
			}
		}
		if reason == "" && worse(after, dr.Before) && !opt.Force {
			reason = fmt.Sprintf("%s scored %.1f against %.1f before", at, after.Score, dr.Before.Score)
		}
		if reason != "" {
			dr.StepDowns = append(dr.StepDowns, reason)
			continue
		}
		dr.Applied = string(at)
		dr.After = after
		dr.Operations = log.Ops(opNames())
		dr.Details = log.Details
		if w.Changed() {
			w.Patch()
			return dr, true
		}
		return unchanged(dr), false
	}
	w.Restore(orig)
	return dr, false
}

// runLevel applies the general operations of one level.
func runLevel(w *tidy.Diagram, at Level, opt Options, log *tidy.Log) {
	on := func(op string) bool { return opt.enabled(op, at) }
	gap := opt.value("min-gap")
	// The shape operations feed each other (aligning can open an overlap,
	// separating can unevenly space a row), so they repeat until a pass moves
	// nothing, or give up after a few passes.
	for pass := 0; pass < 4; pass++ {
		before := w.Save()
		if on("align") {
			w.Align(opt.value("align-tolerance"), log)
		}
		if on("resize") {
			w.Resize(log)
		}
		if on("samesize") {
			w.SameSize(log)
		}
		if on("separate") || on("containers") {
			w.Arrange(gap, on("separate"), on("containers"), log)
		}
		if on("spacing") {
			w.Spacing(opt.value("grid"), log)
		}
		if on("compact") {
			w.Compact(gap, log)
		}
		if on("grid") {
			w.Grid(opt.value("grid"), log)
		}
		if on("separate") || on("containers") {
			w.Arrange(gap, on("separate"), on("containers"), log)
		}
		if w.Unchanged(before) {
			break
		}
	}
	if on("reroute") {
		w.Reroute(log)
	}
	if on("labels") {
		w.Labels(log)
	}
}

func opNames() []string {
	out := make([]string, len(Operations))
	for i, op := range Operations {
		out[i] = op.Name
	}
	return out
}

// worse decides whether a result is worse than the original (docs/adr/0004).
// Defects decide; area and wire length, which tidying shapes up may grow a
// little, only count when the defects are equal and they grew by more than a
// tenth.
func worse(after, before report.Metrics) bool {
	da, db := tidy.DefectScore(after), tidy.DefectScore(before)
	if da != db {
		return da > db
	}
	return after.Area > 1.1*before.Area+1 || after.WireLength > 1.1*before.WireLength+1
}

// hasLanes reports whether a diagram's shapes sit in swimlanes.
func hasLanes(sd *segment.Diagram) bool {
	for _, n := range sd.Nodes {
		if n.Container && n.Shape == "swimlane" {
			for _, c := range n.Children {
				if !c.Container {
					return true
				}
			}
		}
	}
	return false
}

// others returns the boxes of everything on a page outside one diagram.
func others(seg segment.Page, sd *segment.Diagram) []geom.Rect {
	var out []geom.Rect
	for _, d := range seg.Diagrams {
		if d != sd {
			out = append(out, d.Bounds())
		}
	}
	for _, n := range seg.Decoration {
		out = append(out, n.Box)
	}
	for _, w := range seg.LooseWires {
		out = append(out, w.Path.Bounds())
	}
	return out
}

func snapParams(opt Options) snap.Params {
	return snap.Params{
		Distance: opt.value("snap-distance"),
		Ratio:    opt.value("snap-ratio"),
		Margin:   opt.value("snap-margin"),
	}
}

// mergeIssues adds issues not already reported: the second snap pass reports
// the ambiguous ends the first one did.
func mergeIssues(have, add []issue.Issue) []issue.Issue {
	seen := map[string]bool{}
	for _, is := range have {
		seen[is.Code+"|"+strings.Join(is.Cells, ",")] = true
	}
	for _, is := range add {
		if !seen[is.Code+"|"+strings.Join(is.Cells, ",")] {
			have = append(have, is)
		}
	}
	return have
}

func countCode(issues []issue.Issue, code string) int {
	n := 0
	for _, is := range issues {
		if is.Code == code {
			n++
		}
	}
	return n
}

// unchanged clears the operations of a diagram whose result equals what it
// had: they found nothing to change.
func unchanged(dr report.Diagram) report.Diagram {
	dr.Operations, dr.Details = nil, nil
	return dr
}
