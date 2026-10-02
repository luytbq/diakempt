// Package data embeds the font width table into the binary, so diakempt
// measures text the same on every machine without reading font files.
package data

import _ "embed"

// Verdana is the advance width table of Verdana.
//
//go:embed verdana.json
var Verdana []byte
