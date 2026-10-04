package diakempt

import (
	"fmt"

	"github.com/luytbq/diakempt/detect"
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/relayout"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/segment"
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
	lv := opt.level()
	rep := report.File{Pages: len(d.Pages), Issues: warns}
	tm := text.Default()
	for _, p := range d.Pages {
		if p.Empty() {
			continue
		}
		if opt.enabled("snap", lv) {
			r := snap.Run(view.Build(p), snap.Params{
				Distance: opt.value("snap-distance"),
				Ratio:    opt.value("snap-ratio"),
				Margin:   opt.value("snap-margin"),
			})
			rep.Snap.Snapped += r.Snapped
			rep.Snap.Ambiguous += r.Ambiguous
			rep.Snap.Details = append(rep.Snap.Details, r.Details...)
			rep.Issues = append(rep.Issues, r.Issues...)
		}
		seg := segment.Split(view.Build(p))
		rep.Decoration += len(seg.Decoration) + len(seg.LooseWires)
		for _, sd := range seg.Diagrams {
			dr, changed := tidyDiagram(sd, tm, lv, opt)
			rep.Diagrams = append(rep.Diagrams, dr)
			rep.Changed = rep.Changed || changed
		}
	}
	rep.Changed = rep.Changed || rep.Snap.Snapped > 0
	return Result{Output: d.Bytes(), Report: rep}, nil
}

// tidyDiagram optimizes one diagram at the requested level, stepping down a
// level whenever the result scores worse than the original or breaks relative
// order where the level must keep it (docs/adr/0004). It patches the document
// with the kept result.
func tidyDiagram(sd *segment.Diagram, tm *text.Measure, lv Level, opt Options) (report.Diagram, bool) {
	w := tidy.New(sd, tm)
	det := detect.Detect(sd)
	dr := report.Diagram{
		ID: sd.ID(), Page: sd.Page, Index: sd.Index, Name: sd.Name,
		Kind: det.Kind, Confidence: det.Confidence, Signals: det.Signals,
		Level: string(lv), Applied: "none",
	}
	if opt.Kind != "" {
		dr.Kind, dr.Forced, dr.Confidence = opt.Kind, true, ""
	}
	orig := w.Save()
	dr.Before = w.Measure()
	dr.After = dr.Before
	if (dr.Kind == detect.Flowchart || dr.Kind == detect.Swimlane) && opt.enabled("flowlayout", lv) {
		reason := ""
		plan, err := relayout.Run(w, sd, dr.Kind == detect.Swimlane, tm)
		if err != nil {
			reason = "flowlayout not possible: " + err.Error()
		} else if after := w.Measure(); worse(after, dr.Before) && !opt.Force {
			reason = fmt.Sprintf("flowlayout scored %.1f against %.1f before", after.Score, dr.Before.Score)
		} else {
			log := &tidy.Log{}
			log.Counts = map[string]int{"flowlayout": 1}
			dr.Applied = string(lv)
			dr.After = after
			dr.Operations = log.Ops(opNames())
			dr.Details = []report.Detail{{Op: "flowlayout", Msg: fmt.Sprintf("laid out as a %s, direction %s", dr.Kind, plan.Dir)}}
			if w.Changed() {
				w.Patch()
				return dr, true
			}
			return dr, false
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
		return dr, false
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
