// Package layout does placement and routing for activity-swimlane diagrams.
package layout

import "github.com/luytbq/diakempt/engine/model"

// Config holds the layout parameters of this kind. All values are in pixels.
//
// This is the kind config, not the core config: every field here assumes a
// diagram with lanes, tasks and branching diamonds. Kind-independent config
// lives in CoreConfig. See section 6 of docs/core-design.md.
//
// The range of each field is declared in Fields, so the CLI generates flags and
// the web generates forms from the same source.
type Config struct {
	TaskMinW      int // minimum width of a task box
	TaskMaxW      int // maximum width of a task box
	CondWrap      int // wrap width inside a diamond
	TermWrap      int // wrap width for start, end, external
	DBWrap        int
	TextWrap      int
	LabelWrap     int // wrap width for edge labels
	TrackGap      int // spacing between two tracks
	GutterMargin  int // margin from the gutter edge to the first track
	ChannelMargin int
	MinGutter     int // minimum space between two columns
	MinChannel    int // minimum space between two rows
	AttachGap     int // distance from a db or text to the node it attaches to
	LaneHeader    int
	PoolHeader    int
	MinLaneW      int
	LabelPad      int
	// Optimize is the number of geometric optimization rounds run after
	// layout; 0 turns the optimizer off. See Optimize.
	Optimize int
}

// Field declares a config field: its name, range and description.
//
// The CLI generates flags from this list, and the web generates forms and
// validates values from it too, so the two cannot drift apart. The ranges are
// deliberately wide: users may choose a config that produces an ugly layout,
// which the self-check will report, but may not choose values that make the
// engine misbehave, such as negative numbers.
type Field struct {
	Name    string // CLI flag name, also the web field name
	Help    string
	Default int
	Lo, Hi  int
	get     func(*Config) *int
}

// Get returns a pointer to the field of c that f declares.
func (f Field) Get(c *Config) *int { return f.get(c) }

var fields = []Field{
	{"task-min-w", "minimum width of a task box", 120, 20, 2000, func(c *Config) *int { return &c.TaskMinW }},
	{"task-max-w", "maximum width of a task box", 240, 20, 2000, func(c *Config) *int { return &c.TaskMaxW }},
	{"cond-wrap", "wrap width inside a diamond", 150, 20, 2000, func(c *Config) *int { return &c.CondWrap }},
	{"term-wrap", "wrap width for start, end and external", 170, 20, 2000, func(c *Config) *int { return &c.TermWrap }},
	{"db-wrap", "wrap width for db", 130, 20, 2000, func(c *Config) *int { return &c.DBWrap }},
	{"text-wrap", "wrap width for notes", 260, 20, 2000, func(c *Config) *int { return &c.TextWrap }},
	{"label-wrap", "wrap width for edge labels", 180, 20, 2000, func(c *Config) *int { return &c.LabelWrap }},
	{"track-gap", "spacing between two wire tracks", 12, 0, 500, func(c *Config) *int { return &c.TrackGap }},
	{"gutter-margin", "margin from the gutter edge to the first track", 15, 0, 500, func(c *Config) *int { return &c.GutterMargin }},
	{"channel-margin", "margin from the channel edge to the first track", 12, 0, 500, func(c *Config) *int { return &c.ChannelMargin }},
	{"min-gutter", "minimum space between two columns", 24, 0, 2000, func(c *Config) *int { return &c.MinGutter }},
	{"min-channel", "minimum space between two rows", 30, 0, 2000, func(c *Config) *int { return &c.MinChannel }},
	{"attach-gap", "distance from a db or note to the node it attaches to", 40, 0, 2000, func(c *Config) *int { return &c.AttachGap }},
	{"lane-header", "lane header thickness", 30, 0, 500, func(c *Config) *int { return &c.LaneHeader }},
	{"pool-header", "pool header thickness", 30, 0, 500, func(c *Config) *int { return &c.PoolHeader }},
	{"min-lane-w", "minimum thickness of a lane", 120, 20, 4000, func(c *Config) *int { return &c.MinLaneW }},
	{"label-pad", "padding around edge label text", 4, 0, 100, func(c *Config) *int { return &c.LabelPad }},
	{"optimize", "rounds of geometric optimization after layout; 0 turns it off", 3, 0, 100, func(c *Config) *int { return &c.Optimize }},
}

// Fields returns the declarations of all config fields, in Config order.
func Fields() []Field { return append([]Field(nil), fields...) }

// DefaultConfig returns the default parameters.
func DefaultConfig() Config {
	var c Config
	for _, f := range fields {
		*f.Get(&c) = f.Default
	}
	return c
}

// ValidateConfig returns the first error found where a field lies outside its
// range.
func ValidateConfig(c Config) error {
	for _, f := range fields {
		if v := *f.Get(&c); v < f.Lo || v > f.Hi {
			return model.Errf("schema.out_of_range", "%s = %d is outside the range %d..%d", f.Name, v, f.Lo, f.Hi)
		}
	}
	return nil
}
