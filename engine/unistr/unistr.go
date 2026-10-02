// Package unistr defines the whitespace and lowercasing that flowcast uses
// everywhere to trim and compare strings.
//
// Whitespace is every unicode.IsSpace character plus the four separators
// U+001C to U+001F. Those four often slip into cells when pasting from
// spreadsheets, and nobody wants a cell containing only them to count as having
// text. Lowercasing follows Unicode SpecialCasing, so "İ" becomes "i" plus a
// combining dot above. strings.TrimSpace, strings.Fields and strings.ToLower are
// not used directly because they differ at exactly those points.
//
// The Unicode tables are those of the Go version in use. A word-final Greek Σ
// is not turned into ς; the only place affected is the error message for a
// type written in Greek.
package unistr

import (
	"strings"
	"unicode"
)

// IsSpace reports whether r is whitespace.
func IsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

// Strip, LStrip, RStrip trim whitespace from both ends, the left end, the right end.
func Strip(s string) string  { return strings.TrimFunc(s, IsSpace) }
func LStrip(s string) string { return strings.TrimLeftFunc(s, IsSpace) }
func RStrip(s string) string { return strings.TrimRightFunc(s, IsSpace) }

// Fields splits a string on whitespace, dropping empty pieces.
func Fields(s string) []string { return strings.FieldsFunc(s, IsSpace) }

// Lower lowercases. U+0130 becomes "i" plus a combining dot above, per Unicode
// SpecialCasing.
func Lower(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == 0x130 {
			b.WriteString("i̇")
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
