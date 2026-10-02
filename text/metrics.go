// Package text measures text and wraps lines.
//
// It reads no font files. Measurements come from a pre-extracted advance width
// table, so with the same table every machine measures the same, the
// distributed binary carries no font, and nothing depends on whether the target
// machine has Verdana installed.
package text

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/luytbq/diakempt/data"
)

// Metrics is an advance width table per codepoint, in font units.
type Metrics struct {
	Family string
	UPEM   int
	Notdef int
	adv    map[rune]int
}

type metricsFile struct {
	Family   string         `json:"family"`
	UPEM     int            `json:"upem"`
	Notdef   int            `json:"notdef"`
	Advances map[string]int `json:"advances"`
}

// LoadMetrics reads a width table file.
func LoadMetrics(path string) (*Metrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := ParseMetrics(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// ParseMetrics reads a metrics table from bytes, for example the table embedded in the data package.
func ParseMetrics(data []byte) (*Metrics, error) {
	var f metricsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.UPEM == 0 {
		return nil, fmt.Errorf("missing upem")
	}
	m := &Metrics{Family: f.Family, UPEM: f.UPEM, Notdef: f.Notdef, adv: make(map[rune]int, len(f.Advances))}
	for k, v := range f.Advances {
		cp, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("codepoint %q is not a number", k)
		}
		m.adv[rune(cp)] = v
	}
	return m, nil
}

// Width returns the width of the string at font size size, in pixels.
//
// Codepoints missing from the table use the advance of the .notdef glyph, as
// a font does when its cmap has no entry.
func (m *Metrics) Width(s string, size float64) float64 {
	total := 0
	for _, r := range s {
		if w, ok := m.adv[r]; ok {
			total += w
		} else {
			total += m.Notdef
		}
	}
	return float64(total) * size / float64(m.UPEM)
}

// Default returns the embedded Verdana table at the default size.
func Default() *Measure {
	defaultOnce.Do(func() {
		m, err := ParseMetrics(data.Verdana)
		if err != nil {
			panic("embedded font table: " + err.Error())
		}
		defaultMetrics = m
	})
	return NewMeasure(defaultMetrics)
}

var (
	defaultOnce    sync.Once
	defaultMetrics *Metrics
)
