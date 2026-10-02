package layout

// Result is a fully laid-out diagram as plain data: the coordinates are computed
// and the algorithm has finished.
//
// The self-check and writers read Result, not Layout. This lets the self-check
// examine any geometry, including broken geometry that a correct engine never
// produces; that is the only way to test the self-check itself.
//
// An element carries its primitive shape in Shape, and writers read only Shape.
// Kind is the semantic type (task, condition, ...), kept for callers that want
// to know what the element is, for example when writing coordinates to JSON.
type Result struct {
	PoolW, PoolH           float64
	Origin                 [2]float64
	PoolHeader, LaneHeader int
	// NoLanes: the diagram has no lanes. Lanes is then one hidden lane that is
	// not drawn, and there is no pool.
	NoLanes bool
	// Dir is the diagram direction. A Result returned by Layout.Result is always
	// in the virtual TD space; Lay maps it to the real direction before returning it. Writers need
	// the direction to draw lanes as vertical or horizontal bands.
	Dir          string
	MinChannel   int
	Lanes        []PlacedLane
	LaneX, LaneW []float64
	// LaneMinW is the narrowest each lane may become when the optimizer shrinks
	// it: the minimum lane width or the width its name needs.
	LaneMinW []float64
	Items    []PlacedItem // in table row order
	Edges    []PlacedEdge // in table row order
	// TopoOrder is the topological order of the nodes. Merge places new nodes in
	// this order so that nodes earlier in the flow get their spot first.
	TopoOrder []string
}

type PlacedLane struct {
	ID    string
	Lines []string // lane name as in the table, not yet wrapped
}

type PlacedItem struct {
	ID         string
	Type       string
	Shape      Shape
	Lane       int
	Order      int
	Attach     string
	Row, Col   int
	Lines      []string
	Highlight  bool
	X, Y, W, H float64
}

func (it PlacedItem) box() box { return box{it.X, it.Y, it.X + it.W, it.Y + it.H} }

type PlacedEdge struct {
	ID, Src, Dst        string
	Case, ExitSide      byte
	Order               int
	Lines               []string
	Dashed, Highlight   bool
	Bold, NoArrow       bool
	Back                bool
	ExitFrac, EntryFrac [2]float64
	Pts                 [][2]float64
	Label               *[4]float64
	LabelT              float64
	LabelOff            [2]float64

	// The fields below are set only by merge.

	// Auto: let draw.io do the routing; write no anchors, waypoints or label.
	Auto bool
	// Kept: the wire is kept as-is from the existing file. Constraints replace
	// the computed anchors, Waypoints replace the bend points.
	Kept        bool
	Constraints [][2]string
	Waypoints   [][2]float64
	// NoLabelPos: the label goes to the middle of the path, because the wire was
	// edited manually and the label position computed for the old path no
	// longer holds.
	NoLabelPos bool
}

// Result snapshots the current output of Layout. Call it after run.
func (l *Layout) Result() Result {
	r := Result{
		PoolW: l.PoolW, PoolH: l.PoolH, Origin: Origin,
		PoolHeader: l.Cfg.PoolHeader, LaneHeader: l.Cfg.LaneHeader, MinChannel: l.Cfg.MinChannel,
		NoLanes: l.NoLanes, Dir: l.Dir,
		LaneX: l.LaneX, LaneW: l.LaneW, LaneMinW: l.LaneMinW, TopoOrder: l.TopoOrder,
	}
	for _, ln := range l.Lanes {
		r.Lanes = append(r.Lanes, PlacedLane{ln.ID, ln.Lines})
	}
	for _, it := range l.ItemOrder {
		r.Items = append(r.Items, PlacedItem{ID: it.ID, Type: it.Type, Shape: geometryOf(it.Type).Shape, Lane: it.Lane, Order: it.Order, Attach: it.Attach,
			Row: it.Row, Col: it.Col, Lines: it.Lines, Highlight: it.Highlight, X: it.X, Y: it.Y, W: it.W, H: it.H})
	}
	for _, e := range l.Edges {
		r.Edges = append(r.Edges, PlacedEdge{
			ID: e.ID, Src: e.Src, Dst: e.Dst, Case: e.Case, ExitSide: e.ExitSide, Order: e.Order, Back: e.Back,
			Lines: e.Lines, Dashed: e.Dashed, Highlight: e.Highlight, Bold: e.Bold, NoArrow: e.NoArrow,
			ExitFrac: e.ExitFrac, EntryFrac: e.EntryFrac, Pts: e.Pts,
			Label: e.Label, LabelT: e.LabelT, LabelOff: e.LabelOff,
		})
	}
	return r
}
