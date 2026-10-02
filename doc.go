// Package diakempt tidies the layout of existing draw.io diagrams. It changes
// geometry only: styles, text and decorations are kept as they are.
//
// The package takes bytes and returns bytes plus a report. It does not read or
// write files, call external processes or print.
package diakempt
