package layout

import (
	"fmt"
	"github.com/luytbq/diakempt/engine/model"
	"github.com/luytbq/diakempt/engine/schema"
	"github.com/luytbq/diakempt/text"
)

// Item is a drawn element: a node, or a db or text standing next to a node.
type Item struct {
	ID        string
	Type      string
	Lane      int
	Lines     []string
	W, H      float64
	Order     int // row order in the table, used to break ties
	Highlight bool
	// Attach is the id of the node a db or text stands next to. Empty for nodes.
	Attach string

	// Row and Col are meaningful only when Placed. Col may be negative, because
	// side branches drift to the left of the origin column.
	Placed   bool
	Row, Col int
	X, Y     float64
}

// Edge is an arrow between two nodes.
type Edge struct {
	ID        string
	Src, Dst  string
	Lines     []string
	LW, LH    float64
	Order     int
	Dashed    bool
	Highlight bool
	Bold      bool // bold stroke
	NoArrow   bool // no arrowhead
	// Back marks an edge that forms a loop. It takes no part in row assignment.
	Back bool

	// Output of route. Case is one of A, B, C, D.
	Case      byte
	ExitSide  byte
	EntrySide byte
	ExitFrac  [2]float64
	EntryFrac [2]float64
	Sym       []SymPair

	// Output of the geometry and label phases, in pool coordinates.
	Pts      [][2]float64
	Label    *[4]float64 // label box; nil when the edge has no label or it could not be placed
	LabelT   float64     // label position along the path, from -1 at the source to 1 at the target
	LabelOff [2]float64  // offset from the anchor on the path to the center of the label box
}

// Layout is the state of one layout run. The phases run in sequence and each
// phase reads the output of the previous one.
type Layout struct {
	Cfg Config
	tm  *text.Measure

	Lanes   []model.Row
	laneIdx map[string]int
	// NoLanes: the table has no lanes; Lanes holds only one hidden lane.
	NoLanes bool
	depth   map[string]int // memo for branchDepth, reset on each Place
	// Dir is the diagram direction, see axis.go. Empty means TD.
	Dir string

	// items and Edges keep table row order. Go maps iterate in random order, so
	// every loop that needs an order goes through ItemOrder.
	items     map[string]*Item
	ItemOrder []*Item
	Edges     []*Edge
	outs, ins map[string][]*Edge

	Warnings []Warning

	// Trace, when set, receives a line per processing step. See Trace.
	Trace Trace

	// Output of place.
	TopoOrder   []string
	NRows       int
	Cols        map[int][]int
	Attachments map[string][]*Item
	XOrd        map[XKey]int
	occ         map[cell]string
	hside       map[string]map[byte]bool

	// Output of route.
	Segs    []*Seg
	NTracks map[Res]int
	sideOut map[sideKey][]*Edge
	sideIn  map[sideKey][]*Edge

	// Output of the geometry phase.
	LaneX, LaneW []float64
	// LaneMinW is the narrowest each lane may be: the minimum lane width or the
	// width its name needs, whichever is larger.
	LaneMinW     []float64
	PoolW, PoolH float64
	g            *geom
}

// Item returns the element with the given id.
func (l *Layout) Item(id string) *Item { return l.items[id] }

// New builds the initial state from rows that have passed validate.
//
// It does not re-check the input: calling it with a table that still has errors
// violates a precondition.
func newLayout(rows []model.Row, cfg Config, tm *text.Measure) *Layout {
	l := &Layout{
		Cfg:     cfg,
		tm:      tm,
		laneIdx: map[string]int{},
		items:   map[string]*Item{},
		outs:    map[string][]*Edge{},
		ins:     map[string][]*Edge{},
	}
	byID := map[string]model.Row{}
	for _, r := range rows {
		byID[r.ID] = r
		if r.Type == "lane" {
			l.laneIdx[r.ID] = len(l.Lanes)
			l.Lanes = append(l.Lanes, r)
		}
	}
	if len(l.Lanes) == 0 {
		// Flowchart: one hidden lane holds every element, with no headers. The
		// layout algorithm runs unchanged on a single lane.
		l.NoLanes = true
		l.laneIdx[""] = 0
		l.Lanes = []model.Row{{Type: "lane"}}
		l.Cfg.PoolHeader, l.Cfg.LaneHeader = 0, 0
	}
	for _, r := range rows {
		if !schema.NodeTypes[r.Type] && !(schema.AttachTypes[r.Type] && !schema.IsRestMarker(r)) {
			continue
		}
		attach := ""
		if schema.AttachTypes[r.Type] {
			attach = r.Meta["attach"]
		}
		// A db or text is drawn in the lane of the node it attaches to, not the
		// lane it declares itself. validate has already warned when the two differ.
		laneID := r.Parent
		if attach != "" {
			laneID = byID[attach].Parent
		}
		lines, w, h := SizeItem(scaled(tm, r.FontScale), cfg, r.Type, r.Lines)
		if r.Size[0] > 0 && r.Size[1] > 0 {
			w, h = r.Size[0], r.Size[1]
		}
		it := &Item{
			ID: r.ID, Type: r.Type, Lane: l.laneIdx[laneID], Lines: lines, W: w, H: h,
			Order: r.Idx, Highlight: hasStyle(r, "highlight"), Attach: attach,
		}
		l.items[r.ID] = it
		l.ItemOrder = append(l.ItemOrder, it)
	}
	for _, r := range rows {
		if r.Type != "edge" {
			continue
		}
		lines, lw, lh := SizeLabel(scaled(tm, r.FontScale), cfg, r.Lines)
		e := &Edge{
			ID: r.ID, Src: r.Meta["from"], Dst: r.Meta["to"], Lines: lines, LW: lw, LH: lh,
			Order: r.Idx, Dashed: hasStyle(r, "dashed"), Highlight: hasStyle(r, "highlight"),
			Bold: hasStyle(r, "bold"), NoArrow: hasStyle(r, "noarrow"),
			Back:      r.Meta["back"] == "true",
			EntrySide: 'T',
			ExitFrac:  [2]float64{0.5, 1.0},
			EntryFrac: [2]float64{0.5, 0.0},
		}
		l.Edges = append(l.Edges, e)
		l.outs[e.Src] = append(l.outs[e.Src], e)
		l.ins[e.Dst] = append(l.ins[e.Dst], e)
	}
	return l
}

// scaled returns a measure for text drawn at s times the table's font size.
func scaled(tm *text.Measure, s float64) *text.Measure {
	if s <= 0 || s == 1 {
		return tm
	}
	c := *tm
	c.Size *= s
	c.LineH *= s
	return &c
}

func hasStyle(r model.Row, v string) bool {
	for _, s := range splitStyle(r.Meta["style"]) {
		if s == v {
			return true
		}
	}
	return false
}

// Warning is an engine warning: the diagram can still be built, but somewhere
// the engine had to accept a worse option. Code is a stable machine code, ID is
// the element or edge involved if any, Msg is the human-readable message.
type Warning struct {
	Code string
	ID   string
	Msg  string
}

func (l *Layout) warn(code, id, format string, a ...any) {
	l.Warnings = append(l.Warnings, Warning{code, id, fmt.Sprintf(format, a...)})
}
