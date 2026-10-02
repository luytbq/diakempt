package diakempt

import (
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/report"
	"github.com/luytbq/diakempt/snap"
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
	}
	rep.Changed = rep.Snap.Snapped > 0
	return Result{Output: d.Bytes(), Report: rep}, nil
}
