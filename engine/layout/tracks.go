package layout

import "sort"

// assignPorts sets the positions of exit ports on the source node's sides.
//
// Incoming wires always connect at the middle of a side. Outgoing edges on the
// same side share a source, so they may merge; they only need separate ports
// when that side also has incoming wires, because an incoming wire differs in
// both source and target, so overlapping it is wrong.
//
// Diamonds and ellipses exit at the exact middle of a side unless they must be
// separated: the port then lies on the outline, off the vertex. A rectangle with
// a single outgoing edge exits at the middle of the side; with several, A, B and
// C edges keep the middle and D edges share the rest.
func (l *Layout) assignPorts() {
	for key, es := range l.sideOut {
		u := l.items[key.id]
		side := key.side
		if len(es) == 0 {
			continue
		}
		entries := len(l.sideIn[key]) > 0
		if !entries && (geometryOf(u.Type).Shape.singlePort() || len(es) == 1) {
			for _, e := range es {
				e.ExitFrac = sideFrac[side]
			}
			continue
		}
		var fixed, loose []*Edge
		for _, e := range es {
			if !entries && (e.Case == 'A' || e.Case == 'B' || e.Case == 'C') {
				fixed = append(fixed, e)
			} else {
				loose = append(loose, e)
			}
		}
		for _, e := range fixed {
			e.ExitFrac = sideFrac[side]
		}
		n := len(loose)
		var fr []float64
		if n == 1 && entries {
			// Shift off the middle toward where the wire will turn, so the first segment does not cross the incoming wire.
			fr = []float64{0.25}
			if d := l.items[loose[0].Dst]; (side == 'B' && gkOf(u).less(gkOf(d))) || (side != 'B' && d.Row > u.Row) {
				fr = []float64{0.75}
			}
		} else if (len(fixed) > 0 || entries) && n <= 6 {
			// The middle of the side already has a fixed edge, so the remaining ports avoid 0.5.
			fr = append(fr, []float64{0.25, 0.75, 0.125, 0.875, 0.375, 0.625}[:n]...)
			sort.Float64s(fr)
		} else {
			for i := 0; i < n; i++ {
				fr = append(fr, float64(i+1)/float64(n+1))
			}
		}
		// Each wire takes a port on the side it turns toward, so the first
		// segments do not cross each other.
		sort.SliceStable(loose, func(i, j int) bool {
			a, b := l.items[loose[i].Dst], l.items[loose[j].Dst]
			if side == 'L' || side == 'R' {
				if a.Row != b.Row {
					return a.Row < b.Row
				}
				return loose[i].Order < loose[j].Order
			}
			if a.Lane != b.Lane {
				return a.Lane < b.Lane
			}
			if a.Col != b.Col {
				return a.Col < b.Col
			}
			return loose[i].Order < loose[j].Order
		})
		for i, e := range loose {
			e.ExitFrac = geometryOf(u.Type).Shape.outline(side, fr[i])
		}
	}
	for _, e := range l.Edges {
		e.EntryFrac = sideFrac[e.EntrySide]
	}
}

// assignTracks performs interval coloring for each channel and gutter.
//
// Two overlapping segments must lie on different tracks unless they share a
// target, because then they merge into one line. There is one more ordering
// constraint: when two segments have stubs connecting at the same position from
// opposite sides, the segment from the low side must lie on the lower track,
// otherwise the two stubs would overlap.
//
// The algorithm is greedy in the order segments were added, so that order is
// part of the result.
func (l *Layout) assignTracks() {
	var order []Res
	byRes := map[Res][]*Seg{}
	for _, s := range l.Segs {
		if _, ok := byRes[s.Res]; !ok {
			order = append(order, s.Res)
		}
		byRes[s.Res] = append(byRes[s.Res], s)
	}
	l.NTracks = map[Res]int{}

	for _, res := range order {
		var tracks [][]*Seg
		orderOK := func(s *Seg, ti int) bool {
			for tj, tl := range tracks {
				for _, t := range tl {
					if mustPrecede(s, t) && !(ti < tj) {
						return false
					}
					if mustPrecede(t, s) && !(tj < ti) {
						return false
					}
				}
			}
			return true
		}
		fits := func(s *Seg, ti int, busOnly bool) bool {
			tl := tracks[ti]
			if busOnly {
				shared := false
				for _, t := range tl {
					if t.Key == s.Key {
						shared = true
						break
					}
				}
				if !shared {
					return false
				}
			}
			for _, t := range tl {
				if !(s.Key != "" && t.Key == s.Key) && overlap(t, s) {
					return false
				}
			}
			return orderOK(s, ti)
		}

		for _, s := range byRes[res] {
			chosen := -1
			if s.Key != "" {
				for ti := range tracks {
					if fits(s, ti, true) {
						chosen = ti
						break
					}
				}
			}
			if chosen < 0 {
				for ti := range tracks {
					if fits(s, ti, false) {
						chosen = ti
						break
					}
				}
			}
			if chosen < 0 {
				lo, hi := 0, len(tracks)
				for tj, tl := range tracks {
					for _, t := range tl {
						if mustPrecede(t, s) && tj+1 > lo {
							lo = tj + 1
						}
						if mustPrecede(s, t) && tj < hi {
							hi = tj
						}
					}
				}
				if lo > hi {
					l.warn("layout.track-order", "", "could not order the tracks in %s", res.label())
					chosen = len(tracks)
				} else {
					chosen = max(lo, min(hi, len(tracks)))
				}
				tracks = append(tracks, nil)
				copy(tracks[chosen+1:], tracks[chosen:])
				tracks[chosen] = nil
			}
			tracks[chosen] = append(tracks[chosen], s)
		}
		for ti, tl := range tracks {
			for _, t := range tl {
				t.Track = ti
			}
		}
		l.NTracks[res] = len(tracks)
	}
}

func overlap(a, b *Seg) bool { return a.Lo <= b.Hi && b.Lo <= a.Hi }

// mustPrecede reports whether a must lie on a lower track than b: both have
// stubs connecting at the same position, and a's stub comes from the lower side.
// Two segments with the same target are unconstrained, because they merge into
// one.
func mustPrecede(a, b *Seg) bool {
	if a.Key != "" && a.Key == b.Key {
		return false
	}
	for _, pa := range a.Stubs {
		for _, pb := range b.Stubs {
			if pa.Pos == pb.Pos && pa.Dir < pb.Dir {
				return true
			}
		}
	}
	return false
}
