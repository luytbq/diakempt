package layout

import (
	"time"

	"github.com/luytbq/diakempt/engine/model"
	"github.com/luytbq/diakempt/text"
)

// Options configures one layout run.
type Options struct {
	Config Config
	// Direction is TD, BT, LR or RL. Empty means TD.
	Direction string
	// Trace, when set, receives a line per processing step.
	Trace Trace
	// Checkpoint, when set, is called between phases; a non-nil error stops the
	// run and is returned. Build uses it to enforce a time limit.
	Checkpoint func() error
}

// Laid is the outcome of a layout run.
type Laid struct {
	// Result is the finished diagram, already mapped to the requested direction.
	Result Result
	// Warnings are places where the engine had to accept a worse option.
	Warnings []Warning
	// Findings are the self-check results, computed before the axis swap.
	Findings []Finding
}

// Lay lays out a validated table: size, place, route, geometry and labels on the
// grid, then optimization, self-check and the axis swap. It is the only entry
// point of the package, so the order of the phases lives here: the direction
// must be set before sizing, and the self-check must see the top-down space.
//
// The engine only panics when it breaks one of its own invariants. Lay turns
// that into an error with code layout.internal, so a program using the library
// or the web service does not crash on an unusual table.
func Lay(rows []model.Row, tm *text.Measure, opt Options) (out Laid, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = model.Errf("layout.internal", "internal layout error: %v", p)
		}
	}()
	dir := opt.Direction
	if dir == "" {
		dir = DirTD
	}
	t0 := time.Now()
	l := newLayout(rows, opt.Config, tm)
	l.Trace = opt.Trace
	opt.Trace.Log("size: measured and wrapped the text of %d elements and %d edges %s",
		len(l.ItemOrder), len(l.Edges), Since(t0))
	l.setDirection(dir)
	l.run()
	if opt.Checkpoint != nil {
		if err := opt.Checkpoint(); err != nil {
			return Laid{}, err
		}
	}
	res := optimize(l.Result(), opt.Config, opt.Trace)
	for _, w := range l.Warnings {
		opt.Trace.Log("warning: %s %s", w.Code, w.Msg)
	}
	t0 = time.Now()
	findings := Check(res)
	ne := 0
	for _, f := range findings {
		if f.Level == "error" {
			ne++
		}
		opt.Trace.Log("check:   %s %s %s", f.Level, f.Code, f.Msg)
	}
	opt.Trace.Log("check: %d errors, %d warnings %s", ne, len(findings)-ne, Since(t0))
	if dir != DirTD {
		opt.Trace.Log("orient: mapped the top-down layout to %s", dir)
	}
	return Laid{Result: orientResult(res), Warnings: l.Warnings, Findings: findings}, nil
}
