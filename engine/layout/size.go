package layout

import (
	"github.com/luytbq/diakempt/engine/unistr"
	"strings"

	"github.com/luytbq/diakempt/engine/num"
	"github.com/luytbq/diakempt/text"
)

// WrapBudget is the wrap width of an element type, from its declaration in
// Geometry.
//
// It is a named function because behavior cannot pin it down: Wrap shrinks to
// the smallest width that keeps the same line count, so a budget off by a few
// pixels only shows when a character boundary falls exactly within that gap.
// Pinning the number directly does not depend on luck.
func WrapBudget(cfg Config, elementType string) float64 {
	return geometryOf(elementType).Wrap(cfg)
}

// SizeItem wraps an element's content and returns the size of its box, rounded
// up to an even pixel.
//
// It depends only on the element type, content and config, not on where the
// element sits in the diagram. That is why it runs before any placement step.
func SizeItem(tm *text.Measure, cfg Config, elementType string, lines []string) (wrapped []string, w, h float64) {
	g := geometryOf(elementType)
	wl := tm.Wrap(lines, g.Wrap(cfg))
	tw, th := tm.Box(wl)
	bw, bh := g.Size(cfg, tw, th)
	return wl, float64(num.Rnd(bw)), float64(num.Rnd(bh))
}

// SizeLabel measures an edge's label.
//
// An edge without a label still gets a size, not zero: the width equals the
// padding on both sides and the height is 2. The reference implementation adds
// the padding outside the empty-string check, so an empty edge comes out 8 x 2
// rather than 0 x 0.
func SizeLabel(tm *text.Measure, cfg Config, lines []string) (wrapped []string, w, h float64) {
	var wl []string
	if HasText(lines) {
		wl = tm.Wrap(lines, float64(cfg.LabelWrap))
	}
	var lw, lh float64
	if len(wl) > 0 {
		lw, lh = tm.Box(wl)
	}
	return wl, lw + float64(2*cfg.LabelPad), lh + 2
}

// HasText reports whether a table row's content has real text: join the lines
// with newlines, then trim whitespace.
func HasText(lines []string) bool {
	return unistr.Strip(strings.Join(lines, "\n")) != ""
}
