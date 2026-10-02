// Package issue defines the findings every step reports back to the caller.
package issue

import "fmt"

// Severity orders how much a finding matters to the user.
type Severity string

const (
	// Info records something diakempt did or inferred.
	Info Severity = "info"
	// Warning is something the user should look at.
	Warning Severity = "warning"
	// Error stops a file from being processed.
	Error Severity = "error"
)

// Issue is one finding. Code is stable and machine-readable; Msg is for people.
type Issue struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	// Page is the zero-based page index, or -1 when the issue is about the file.
	Page    int      `json:"page"`
	Diagram string   `json:"diagram,omitempty"`
	Cells   []string `json:"cells,omitempty"`
	Msg     string   `json:"message"`
}

func (i Issue) Error() string { return i.Msg }

// New builds an issue about a page.
func New(sev Severity, code string, page int, cells []string, format string, args ...any) Issue {
	return Issue{Code: code, Severity: sev, Page: page, Cells: cells, Msg: fmt.Sprintf(format, args...)}
}

// FileError builds an error about the whole file.
func FileError(code, format string, args ...any) Issue {
	return Issue{Code: code, Severity: Error, Page: -1, Msg: fmt.Sprintf(format, args...)}
}
