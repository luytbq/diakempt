package diakempt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/luytbq/diakempt/classes"
	"github.com/luytbq/diakempt/communication"
	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/issue"
	"github.com/luytbq/diakempt/normalize"
	"github.com/luytbq/diakempt/packages"
	"github.com/luytbq/diakempt/relayout"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/segment"
	"github.com/luytbq/diakempt/sequence"
	"github.com/luytbq/diakempt/snap"
	"github.com/luytbq/diakempt/text"
	"github.com/luytbq/diakempt/tidy"
	"github.com/luytbq/diakempt/usecase"
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
	rep.Issues = append(rep.Issues, unmatchedKinds(opt, rep.Diagrams)...)
	rep.Snap.Ambiguous = countCode(rep.Issues, "snap.ambiguous")
	return Result{Output: d.Bytes(), Report: rep}, nil
}

// unmatchedKinds warns about each diagram a kind was forced on that the file
// does not hold, most likely a mistyped identifier.
func unmatchedKinds(opt Options, diagrams []report.Diagram) []issue.Issue {
	var out []issue.Issue
	for _, ref := range sortedKeys(opt.DiagramKinds) {
		found := false
		for _, d := range diagrams {
			if refMatches(ref, d.Page, d.Index, d.Name) {
				found = true
				break
			}
		}
		if !found {
			is := issue.New(issue.Warning, "type.no_diagram", -1, nil,
				"no diagram %s to treat as %s; %s", ref, opt.DiagramKinds[ref], whatIsThere(ref, diagrams))
			is.Diagram = ref
			out = append(out, is)
		}
	}
	return out
}

// whatIsThere describes the diagrams near a reference that matched none, so
// the identifier can be corrected without another run.
func whatIsThere(ref string, diagrams []report.Diagram) string {
	page, index, _, _ := parseRef(ref)
	last := 0
	for _, d := range diagrams {
		if d.Page != page {
			continue
		}
		if d.Index == index {
			if d.Name == "" {
				return fmt.Sprintf("Page-%d #%d has no name", page+1, index+1)
			}
			return fmt.Sprintf("Page-%d #%d is %q", page+1, index+1, d.Name)
		}
		last = max(last, d.Index+1)
	}
	switch last {
	case 0:
		return fmt.Sprintf("page %d holds no diagram", page+1)
	case 1:
		return fmt.Sprintf("page %d has only #1", page+1)
	}
	return fmt.Sprintf("page %d has #1 to #%d", page+1, last)
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
		earlier[d.Members] = d
	}
	var out []report.Diagram
	for _, d := range later.diagrams {
		// A diagram that gained or lost members between passes is a different
		// diagram; its own figures stand. One that kept them keeps the name
		// it was found under, even when moving shapes changed its first one.
		if e, ok := earlier[d.Members]; ok {
			d.ID, d.Index, d.Name = e.ID, e.Index, e.Name
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
	// The same rule as the keep decision (docs/adr/0004), applied to the
	// final geometry against the original. A page tidied in several passes
	// keeps its first Before, so this can differ from what each pass judged.
	for i := range pr.diagrams {
		pr.diagrams[i].Worse = worse(pr.diagrams[i].After, pr.diagrams[i].Before)
	}
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
		Members: members(sd),
		Kind:    det.Kind, Confidence: det.Confidence, Signals: det.Signals,
		Level: string(lv), Applied: "none",
	}
	if k := opt.kindFor(sd.Page, sd.Index, sd.Name); k != "" {
		dr.Kind, dr.Forced, dr.Confidence = k, true, ""
	}
	orig := w.Save()
	dr.Before = w.Measure()
	dr.After = dr.Before
	for _, a := range attempts(dr.Kind, lv, opt, tm) {
		if !a.try(w, &dr, opt) {
			w.Restore(orig)
			continue
		}
		if w.Changed() {
			w.Patch()
			return dr, true
		}
		return unchanged(dr), false
	}
	return dr, false
}

// An attempt is one way to lay out a diagram. A diagram keeps the first
// attempt whose result is not worse than the original.
type attempt struct {
	// op names a layout of its own; empty for the general operations of a
	// level.
	op string
	at Level
	// lay changes the working copy. It returns the detail the report gives a
	// kept layout, or an error saying why it cannot lay out this diagram.
	lay func(w *tidy.Diagram, log *tidy.Log) (string, error)
}

// attempts lists, in the order they are tried, the ways a diagram of a kind
// may be laid out: the kind's own optimizer, then the engine, then the
// general operations of each level from the requested one down to safe.
func attempts(kind string, lv Level, opt Options, tm *text.Measure) []attempt {
	var out []attempt
	for _, k := range []struct {
		kind, op, as string
		lay          func(*tidy.Diagram) error
	}{
		{detect.Class, "classlayout", "a class diagram", classes.Layout},
		{detect.Sequence, "seqlayout", "a sequence diagram", sequence.Layout},
		{detect.ER, "erlayout", "an ER diagram", classes.LayoutER},
		{detect.UseCase, "usecaselayout", "a use case diagram", usecase.Layout},
		{detect.Comm, "commlayout", "a communication diagram", communication.Layout},
		{detect.Package, "pkglayout", "a package diagram", packages.Layout},
		{detect.Deploy, "deploylayout", "a deployment diagram", communication.LayoutDeployment},
	} {
		if kind == k.kind && opt.enabled(k.op, lv) {
			out = append(out, attempt{op: k.op, at: lv, lay: func(w *tidy.Diagram, _ *tidy.Log) (string, error) {
				return "laid out as " + k.as, k.lay(w)
			}})
		}
	}
	flow := (kind == detect.Flowchart || kind == detect.Swimlane || kind == detect.State) && opt.enabled("flowlayout", lv)
	free := kind == detect.Unknown && opt.enabled("relayout", lv)
	if flow || free {
		op := "flowlayout"
		if free {
			op = "relayout"
		}
		out = append(out, attempt{op: op, at: lv, lay: func(w *tidy.Diagram, _ *tidy.Log) (string, error) {
			withLanes := kind == detect.Swimlane
			if free {
				withLanes = hasLanes(w.Seg)
			}
			plan, err := relayout.Run(w, withLanes, tm)
			if err != nil {
				return "", err
			}
			as := kind
			if as == detect.State {
				as = "state machine"
			}
			if free {
				as = "flowchart"
				if withLanes {
					as = "swimlane"
				}
			}
			return fmt.Sprintf("laid out as a %s, direction %s", as, plan.Dir), nil
		}})
	}
	// A flow the engine could not improve gets only the safe operations: the
	// shape operations of normal are for diagrams no optimizer understands.
	top := lv.rank()
	if (kind == detect.Flowchart || kind == detect.Swimlane) && top > Safe.rank() {
		top = Safe.rank()
	}
	for rank := top; rank >= 0; rank-- {
		at := Levels[rank]
		out = append(out, attempt{at: at, lay: func(w *tidy.Diagram, log *tidy.Log) (string, error) {
			runLevel(w, at, opt, log)
			return "", nil
		}})
	}
	return out
}

// try runs the attempt on the working copy and judges the result. A kept
// result is recorded in dr; a rejected one adds its step down reason and
// leaves the working copy for the caller to restore.
func (a attempt) try(w *tidy.Diagram, dr *report.Diagram, opt Options) bool {
	name := a.op
	if name == "" {
		name = string(a.at)
	}
	log := &tidy.Log{}
	msg, err := a.lay(w, log)
	if err != nil {
		dr.StepDowns = append(dr.StepDowns, name+" not possible: "+err.Error())
		return false
	}
	after := w.Measure()
	if a.op == "" && a.at != Aggressive {
		if err := w.OrderKept(); err != nil {
			dr.StepDowns = append(dr.StepDowns, fmt.Sprintf("%s broke relative order (%v)", a.at, err))
			return false
		}
	}
	if worse(after, dr.Before) && !opt.Force {
		dr.StepDowns = append(dr.StepDowns, fmt.Sprintf("%s scored %.1f against %.1f before", name, after.Score, dr.Before.Score))
		return false
	}
	dr.Details = nil
	if a.op != "" {
		after, log = settle(w, after, a.at, opt, a.op)
		dr.Details = []report.Detail{{Op: a.op, Msg: msg}}
	}
	dr.Applied, dr.After = string(a.at), after
	dr.Operations = log.Ops(opNames())
	dr.Details = append(dr.Details, log.Details...)
	return true
}

// settle finishes a layout that was just accepted. wirestyle turns the
// straight wires the layout left broken orthogonal, each change scoring
// better. Then labels moves the labels off the wires and shapes the layout
// put them on; it minimizes each label's own hits, not the score, so its
// moves can still raise the measured defects, and when they do by the
// never-worse rule (docs/adr/0004) they go back.
func settle(w *tidy.Diagram, laid report.Metrics, lv Level, opt Options, op string) (report.Metrics, *tidy.Log) {
	log := &tidy.Log{Counts: map[string]int{op: 1}}
	if opt.enabled("wirestyle", lv) {
		w.WireStyle(log)
		laid = w.Measure()
	}
	if !opt.enabled("labels", lv) {
		return laid, log
	}
	saved := w.Save()
	moved := &tidy.Log{}
	w.Labels(moved)
	if settled := w.Measure(); !worse(settled, laid) {
		for op, n := range moved.Counts {
			log.Counts[op] += n
		}
		log.Details = append(log.Details, moved.Details...)
		return settled, log
	}
	w.Restore(saved)
	return laid, log
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
	if on("wirestyle") {
		w.WireStyle(log)
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

// members lists the cells of a diagram's nodes and wires.
func members(sd *segment.Diagram) string {
	var ids []string
	for _, n := range sd.Nodes {
		ids = append(ids, n.ID)
	}
	for _, w := range sd.Wires {
		ids = append(ids, w.Cell.ID)
	}
	slices.Sort(ids)
	return strings.Join(ids, "\x00")
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
