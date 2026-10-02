package layout

// Route picks a routing style for each edge, then assigns ports and tracks.
//
// Styles are tried in turn from simple to general, one pass over all edges per
// style: A straight vertical, B straight horizontal, C L-shaped, D through
// channels and gutters. An edge takes the first style whose path is still free.
// The pass order matters: straight edges reserve their space first, and only
// then do the more complex styles have to avoid them.
//
// Calling it again starts from scratch, for the same reason as Place: an edge
// that already has a Case would be skipped by every pass, and the previous
// run's wire segments would share tracks with this run's.
func (l *Layout) Route() {
	for _, e := range l.Edges {
		e.Case, e.ExitSide, e.EntrySide, e.Sym = 0, 0, 'T', nil
		e.ExitFrac, e.EntryFrac = [2]float64{0.5, 1.0}, [2]float64{0.5, 0.0}
	}
	l.Segs = nil
	l.sideOut = map[sideKey][]*Edge{}
	l.sideIn = map[sideKey][]*Edge{}
	cellsH := map[cell]map[string]bool{}
	cellsV := map[cell]map[string]bool{}

	free := func(c cell) bool { _, taken := l.occ[c]; return !taken }
	// vOK: the vertical cell is free, or occupied only by other wires with the
	// same target; wires with the same target may overlap because they merge
	// into one line.
	vOK := func(c cell, dst string) bool {
		for id := range cellsV[c] {
			if id != dst {
				return false
			}
		}
		return free(c)
	}
	hOK := func(c cell) bool { return free(c) && len(cellsH[c]) == 0 }
	mark := func(m map[cell]map[string]bool, c cell, id string) {
		if m[c] == nil {
			m[c] = map[string]bool{}
		}
		m[c][id] = true
	}
	column := func(it *Item, from, to int) []cell {
		var out []cell
		for r := from; r < to; r++ {
			out = append(out, cell{it.Lane, it.Col, r})
		}
		return out
	}

	// A: straight vertical within the same column.
	for _, e := range l.Edges {
		u, v := l.items[e.Src], l.items[e.Dst]
		if e.Back || gkOf(u) != gkOf(v) || v.Row <= u.Row {
			continue
		}
		cells := column(u, u.Row+1, v.Row)
		if len(l.sideOut[sideKey{u.ID, 'B'}]) > 0 {
			continue
		}
		if all(cells, func(c cell) bool { return vOK(c, v.ID) }) {
			e.Case, e.ExitSide = 'A', 'B'
			for _, c := range cells {
				mark(cellsV, c, v.ID)
			}
			l.link(e, u, 'B', v, 'T')
		}
	}

	// B: straight horizontal on the same row.
	for _, e := range l.Edges {
		u, v := l.items[e.Src], l.items[e.Dst]
		if e.Case != 0 || e.Back || v.Row != u.Row || gkOf(u) == gkOf(v) || geometryOf(v.Type).EntryTopOnly {
			continue
		}
		s := face(u, v)
		if l.sideUsed(u, s) || l.sideUsed(v, opp(s)) {
			continue
		}
		if l.attachSides(u)[s] || l.attachSides(v)[opp(s)] {
			continue
		}
		cells := l.cellsBetween(gkOf(u), gkOf(v), u.Row)
		if all(cells, hOK) {
			e.Case, e.ExitSide, e.EntrySide = 'B', s, opp(s)
			for _, c := range cells {
				mark(cellsH, c, v.ID)
			}
			l.link(e, u, s, v, opp(s))
		}
	}

	// C: L-shaped, exits a lateral side then turns down into the target's top.
	for _, e := range l.Edges {
		u, v := l.items[e.Src], l.items[e.Dst]
		if e.Case != 0 || e.Back || v.Row <= u.Row || gkOf(u) == gkOf(v) {
			continue
		}
		s := face(u, v)
		if l.sideUsed(u, s) || l.attachSides(u)[s] {
			continue
		}
		turn := cell{v.Lane, v.Col, u.Row}
		hcells := append(l.cellsBetween(gkOf(u), gkOf(v), u.Row), turn)
		vcells := append([]cell{turn}, column(v, u.Row+1, v.Row)...)
		if all(hcells, hOK) && all(vcells, func(c cell) bool { return vOK(c, v.ID) }) {
			e.Case, e.ExitSide = 'C', s
			for _, c := range hcells {
				mark(cellsH, c, v.ID)
			}
			for _, c := range vcells {
				mark(cellsV, c, v.ID)
			}
			l.link(e, u, s, v, 'T')
			e.Sym = []SymPair{{colRef(v), srcRef}}
		}
	}

	// D: general, through horizontal channels between rows and vertical gutters between columns.
	for _, e := range l.Edges {
		if e.Case != 0 {
			continue
		}
		u, v := l.items[e.Src], l.items[e.Dst]
		s := face(u, v)
		e.Case = 'D'
		choices := []byte{s, 'B', opp(s)}
		if e.Back || v.Row <= u.Row {
			// A target above or on the same row cannot be reached by exiting from the bottom.
			choices = []byte{s, opp(s)}
		}
		e.ExitSide = l.pickExit(u, s, choices)
		l.link(e, u, e.ExitSide, v, 'T')
		cu, cv := l.XOrd[XKey{'c', u.Lane, u.Col}], l.XOrd[XKey{'c', v.Lane, v.Col}]
		chV := Res{Kind: 'C', A: v.Row}

		if e.ExitSide != 'B' {
			gs := Res{Kind: 'G', A: u.Lane, B: l.gutter(u.Lane, u.Col, e.ExitSide)}
			dir := 1
			if e.ExitSide == 'R' {
				dir = -1
			}
			s1 := l.seg(gs, 2*u.Row+1, 2*v.Row, v.ID, Stub{2*u.Row + 1, dir})
			s2 := l.seg(chV, l.XOrd[xkeyOf(gs)], cv, v.ID, Stub{cv, 1})
			e.Sym = []SymPair{{segRef(s1), srcRef}, {segRef(s1), segRef(s2)}, {colRef(v), segRef(s2)}}
			continue
		}

		ch1 := Res{Kind: 'C', A: u.Row + 1}
		if v.Row == u.Row+1 {
			s1 := l.seg(ch1, cu, cv, v.ID, Stub{cu, -1}, Stub{cv, 1})
			e.Sym = []SymPair{{srcRef, segRef(s1)}, {colRef(v), segRef(s1)}}
			continue
		}
		vcells := column(v, u.Row+1, v.Row)
		if all(vcells, func(c cell) bool { return vOK(c, v.ID) }) && gkOf(u) != gkOf(v) {
			for _, c := range vcells {
				mark(cellsV, c, v.ID)
			}
			s1 := l.seg(ch1, cu, cv, "", Stub{cu, -1}, Stub{cv, 1})
			e.Sym = []SymPair{{srcRef, segRef(s1)}, {colRef(v), segRef(s1)}}
			continue
		}
		gside := byte('L')
		if !gkOf(u).less(gkOf(v)) {
			gside = 'R'
		}
		gt := Res{Kind: 'G', A: v.Lane, B: l.gutter(v.Lane, v.Col, gside)}
		xg := l.XOrd[xkeyOf(gt)]
		s1 := l.seg(ch1, cu, xg, "", Stub{cu, -1})
		s2 := l.seg(gt, 2*(u.Row+1), 2*v.Row, v.ID)
		s3 := l.seg(chV, xg, cv, v.ID, Stub{cv, 1})
		e.Sym = []SymPair{{srcRef, segRef(s1)}, {segRef(s2), segRef(s1)}, {segRef(s2), segRef(s3)}, {colRef(v), segRef(s3)}}
	}

	l.assignPorts()
	l.assignTracks()
}

var srcRef = Ref{Kind: RefSrc}

func colRef(v *Item) Ref { return Ref{Kind: RefCol, Lane: v.Lane, Col: v.Col} }
func segRef(s *Seg) Ref  { return Ref{Kind: RefSeg, Seg: s} }

func xkeyOf(r Res) XKey { return XKey{'G', r.A, r.B} }

func gkOf(it *Item) gk { return gk{it.Lane, it.Col} }

// face is the side of u facing v. The same column counts as the right side.
func face(u, v *Item) byte {
	if gkOf(u) == gkOf(v) || gkOf(u).less(gkOf(v)) {
		return 'R'
	}
	return 'L'
}

func (l *Layout) link(e *Edge, u *Item, out byte, v *Item, in byte) {
	l.sideOut[sideKey{u.ID, out}] = append(l.sideOut[sideKey{u.ID, out}], e)
	l.sideIn[sideKey{v.ID, in}] = append(l.sideIn[sideKey{v.ID, in}], e)
}

func (l *Layout) sideUsed(it *Item, s byte) bool {
	return len(l.sideOut[sideKey{it.ID, s}]) > 0 || len(l.sideIn[sideKey{it.ID, s}]) > 0
}

// attachSides are the sides of u that have a db or text standing right next to it.
func (l *Layout) attachSides(u *Item) map[byte]bool {
	out := map[byte]bool{}
	for _, a := range l.Attachments[u.ID] {
		if a.Col > u.Col {
			out['R'] = true
		} else {
			out['L'] = true
		}
	}
	return out
}

// sideFree reports whether that side of u can take one more D-style outgoing
// edge.
//
// A shape with a single connection point per side (diamond, ellipse) is full
// with one edge. A rectangle can split one side into several ports, unless that
// side already has a B or C edge: those exit at the exact middle of the side,
// so it cannot be split.
func (l *Layout) sideFree(u *Item, side byte) bool {
	if l.attachSides(u)[side] || len(l.sideIn[sideKey{u.ID, side}]) > 0 {
		return false
	}
	used := l.sideOut[sideKey{u.ID, side}]
	if geometryOf(u.Type).Shape.singlePort() {
		return len(used) == 0
	}
	for _, x := range used {
		if x.Case == 'B' || x.Case == 'C' {
			return false
		}
	}
	return true
}

// pickExit picks the exit side for a D edge: the first free side, otherwise the
// side facing the target. That side may already have incoming wires;
// assignPorts will separate the exit port from the entry ports. That side may
// also have a db or text; they then do not hug the node but stand centered in
// the adjacent cell, and the wire runs in the gutter between them.
func (l *Layout) pickExit(u *Item, s byte, choices []byte) byte {
	for _, c := range choices {
		if l.sideFree(u, c) {
			return c
		}
	}
	return s
}

// cellsBetween are the cells strictly between columns a and b on row r.
func (l *Layout) cellsBetween(a, b gk, r int) []cell {
	if b.less(a) {
		a, b = b, a
	}
	var out []cell
	for lane := a.lane; lane <= b.lane; lane++ {
		for _, c := range l.Cols[lane] {
			k := gk{lane, c}
			if a.less(k) && k.less(b) {
				out = append(out, cell{lane, c, r})
			}
		}
	}
	return out
}

// gutter is the index of the gutter immediately left or right of a column.
func (l *Layout) gutter(lane, col int, side byte) int {
	for i, c := range l.Cols[lane] {
		if c == col {
			if side == 'L' {
				return i
			}
			return i + 1
		}
	}
	panic("layout: column not in lane")
}

func (l *Layout) seg(res Res, lo, hi int, key string, stubs ...Stub) *Seg {
	if hi < lo {
		lo, hi = hi, lo
	}
	s := &Seg{Res: res, Lo: lo, Hi: hi, Key: key, Stubs: stubs}
	l.Segs = append(l.Segs, s)
	return s
}

func all(cells []cell, ok func(cell) bool) bool {
	for _, c := range cells {
		if !ok(c) {
			return false
		}
	}
	return true
}
