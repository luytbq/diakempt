package diakempt

import (
	"fmt"
	"sort"
)

// Level sets how far diakempt may change a layout; see docs/adr/0003.
type Level string

const (
	Safe       Level = "safe"
	Normal     Level = "normal"
	Aggressive Level = "aggressive"
)

// Levels lists the levels from the most to the least conservative.
var Levels = []Level{Safe, Normal, Aggressive}

func (l Level) rank() int {
	for i, x := range Levels {
		if x == l {
			return i
		}
	}
	return -1
}

// Kinds lists the diagram kinds diakempt can be told to assume with
// Options.Kind. Unknown means "use the general tidy-up".
var Kinds = []string{"flowchart", "swimlane", "sequence", "class", "unknown"}

// Operation is one kind of change a level performs. Each can be switched on or
// off on its own, overriding the level.
type Operation struct {
	Name string
	// From is the lowest level that performs the operation.
	From Level
	Help string
}

// Operations lists every operation in the order it runs.
var Operations = []Operation{
	{"snap", Safe, "attach wire ends dropped next to a shape"},
	{"flowlayout", Safe, "lay recognized flowcharts and swimlanes out from scratch with the layout engine"},
	{"classlayout", Safe, "lay recognized class diagrams out: parents above children, generalizations as trunks"},
	{"separate", Safe, "push overlapping nodes apart"},
	{"containers", Safe, "grow containers to fit their children"},
	{"reroute", Safe, "reroute wires that cross a node or run on top of another wire"},
	{"labels", Safe, "move overlapping labels apart"},
	{"align", Normal, "line up nodes that are almost aligned"},
	{"resize", Normal, "resize nodes to fit their text"},
	{"samesize", Normal, "give nodes of the same style the same size"},
	{"spacing", Normal, "even out the gaps between neighboring nodes"},
	{"compact", Normal, "remove excess empty space"},
	{"grid", Normal, "snap node positions to the grid"},
	{"normalize", Aggressive, "rewrite structure: merge stacked shapes, dissolve convenience groups, join wires to wires"},
	{"relayout", Aggressive, "place unknown diagrams freely with the layout engine"},
}

// Param is a numeric setting with a default and a valid range.
type Param struct {
	Name     string
	Default  float64
	Min, Max float64
	Help     string
}

// Params lists every numeric setting. The CLI builds its flags from this list.
var Params = []Param{
	{"snap-distance", 20, 0, 200, "farthest a wire end may be from a shape outline to snap, in pixels"},
	{"snap-ratio", 0.25, 0, 1, "farthest a wire end may be from a shape outline to snap, as a share of the shape's shorter side"},
	{"snap-margin", 1.5, 1, 10, "how many times closer the nearest shape must be than the next one to snap"},
	{"align-tolerance", 8, 0, 50, "largest center offset, in pixels, that align treats as meant to be aligned"},
	{"min-gap", 20, 0, 200, "smallest gap between nodes, in pixels"},
	{"grid", 10, 1, 100, "grid size in pixels"},
}

// Options configures one run.
type Options struct {
	// Level defaults to Normal.
	Level Level
	// Kind forces the kind of every diagram in the file; empty detects it.
	Kind string
	// Ops switches single operations on (true) or off (false), overriding Level.
	Ops map[string]bool
	// Values overrides Params by name.
	Values map[string]float64
	// Force keeps results whose score got worse.
	Force bool
	// Seed feeds randomized optimizers.
	Seed int64
}

// Validate checks names and ranges.
func (o Options) Validate() error {
	if o.Level != "" && o.Level.rank() < 0 {
		return fmt.Errorf("unknown level %q (want safe, normal or aggressive)", o.Level)
	}
	if o.Kind != "" && !contains(Kinds, o.Kind) {
		return fmt.Errorf("unknown type %q (want one of %v)", o.Kind, Kinds)
	}
	for name := range o.Ops {
		if _, ok := opByName(name); !ok {
			return fmt.Errorf("unknown operation %q", name)
		}
	}
	for _, name := range sortedKeys(o.Values) {
		p, ok := paramByName(name)
		if !ok {
			return fmt.Errorf("unknown setting %q", name)
		}
		if v := o.Values[name]; v < p.Min || v > p.Max {
			return fmt.Errorf("%s = %v is out of range [%v, %v]", name, v, p.Min, p.Max)
		}
	}
	return nil
}

func (o Options) level() Level {
	if o.Level == "" {
		return Normal
	}
	return o.Level
}

// enabled reports whether an operation runs at level lv under these options.
func (o Options) enabled(op string, lv Level) bool {
	if on, ok := o.Ops[op]; ok {
		return on
	}
	x, _ := opByName(op)
	return lv.rank() >= x.From.rank()
}

// value returns a param's value.
func (o Options) value(name string) float64 {
	if v, ok := o.Values[name]; ok {
		return v
	}
	p, ok := paramByName(name)
	if !ok {
		panic("unknown param " + name)
	}
	return p.Default
}

func opByName(name string) (Operation, bool) {
	for _, op := range Operations {
		if op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}

func paramByName(name string) (Param, bool) {
	for _, p := range Params {
		if p.Name == name {
			return p, true
		}
	}
	return Param{}, false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
