package diakempt

import (
	"github.com/luytbq/diakempt/doc"
	"github.com/luytbq/diakempt/report"
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
	return Result{Output: d.Bytes(), Report: rep}, nil
}
