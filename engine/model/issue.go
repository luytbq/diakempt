package model

import (
	"fmt"
	"github.com/luytbq/diakempt/engine/unistr"
	"strings"
)

// Severity of an Issue.
const (
	LevelError   = "error"
	LevelWarning = "warning"
)

// Location says where in the source a finding is.
//
// Structured rather than a string, so the web UI can point at the right cell and
// line. String is the form the CLI prints.
type Location struct {
	Kind   string `json:"kind"`             // "table" or "text"
	Row    int    `json:"row,omitempty"`    // table: line number in the file, 0 means unknown
	Sheet  string `json:"sheet,omitempty"`  // xlsx
	Cell   string `json:"cell,omitempty"`   // xlsx: cell address, e.g. B7, or a merged range like A1:B2
	Column string `json:"column,omitempty"` // table: column name, when the finding maps to one cell
	Line   int    `json:"line,omitempty"`   // text: line number in the source, used for mermaid
}

// LineLoc is the location of a table row in a text file.
func LineLoc(row int) Location { return Location{Kind: "table", Row: row} }

func (l Location) String() string {
	switch {
	case l.Sheet != "" && l.Cell != "":
		return l.Sheet + "!" + l.Cell
	case l.Sheet != "" && l.Row > 0:
		return fmt.Sprintf("%s row %d", l.Sheet, l.Row)
	case l.Sheet != "":
		return l.Sheet
	case l.Kind == "text" && l.Line > 0:
		return fmt.Sprintf("line %d", l.Line)
	case l.Row > 0:
		return fmt.Sprintf("line %d", l.Row)
	}
	return ""
}

// Issue is a finding returned to the caller.
//
// Code is a stable machine code, for the caller to classify and for the web
// service to return as JSON. Msg is the human-readable message. The core never
// prints Issues; the caller presents them.
type Issue struct {
	Code   string
	Level  string
	Loc    Location
	ID     string
	Msg    string
	Params map[string]string
}

// String is the form the CLI prints for an Issue.
func (i Issue) String() string {
	tag := ""
	if i.ID != "" {
		tag = " [" + i.ID + "]"
	}
	loc := i.Loc.String()
	if loc == "" {
		loc = "-"
	}
	return fmt.Sprintf("%-7s %s%s: %s", strings.ToUpper(i.Level), loc, tag, i.Msg)
}

func joinLines(lines []string) string { return strings.Join(lines, "\n") }
func trimSpace(s string) string       { return unistr.Strip(s) }

// Error is an error that stops reading and cannot be tied to a specific row:
// header not found, corrupt file, limit exceeded. Everything else is an Issue.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func Errf(code, format string, a ...any) error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, a...)}
}
