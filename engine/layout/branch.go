package layout

import "sort"

// sameLaneOuts returns the non-back outgoing edges whose target is in the same
// lane as the source, in table row order.
func (l *Layout) sameLaneOuts(nid string) []*Edge {
	u := l.items[nid]
	var out []*Edge
	for _, x := range l.outs[nid] {
		if !x.Back && l.items[x.Dst].Lane == u.Lane {
			out = append(out, x)
		}
	}
	return out
}

// branchDrift reports which side this branch eventually moves to in terms of
// lanes: -1 left, 1 right, 0 unknown.
//
// Walks along the branch to the first edge that leaves the lane. Stops at a
// merge node because from there on the flow is shared and no longer tells the
// direction of this branch alone.
func (l *Layout) branchDrift(e *Edge) int {
	v := l.items[e.Dst]
	seen := map[string]bool{v.ID: true}
	queue := []string{v.ID}
	for len(queue) > 0 {
		nid := queue[0]
		queue = queue[1:]
		outs := append([]*Edge(nil), l.outs[nid]...)
		sort.SliceStable(outs, func(i, j int) bool { return outs[i].Order < outs[j].Order })
		for _, x := range outs {
			if x.Back {
				continue
			}
			w := l.items[x.Dst]
			if w.Lane != v.Lane {
				if w.Lane < v.Lane {
					return -1
				}
				return 1
			}
			if seen[w.ID] {
				continue
			}
			if l.nonBackIn(w.ID) > 1 {
				continue
			}
			seen[w.ID] = true
			queue = append(queue, w.ID)
		}
	}
	return 0
}

// mainEdge returns the index of the main branch among the same-lane outgoing
// edges.
//
// Diagrams with lanes follow the Flow Table convention: the main branch is the
// edge written last, because table authors are told to put the longest
// continuing branch at the end. Diagrams without lanes usually come from
// mermaid, which has no such convention, so the main branch is the deepest one;
// on a tie the edge written later still wins.
func (l *Layout) mainEdge(same []*Edge) int {
	last := len(same) - 1
	if !l.NoLanes {
		return last
	}
	best, bd := last, l.branchDepth(same[last].Dst)
	for i := last - 1; i >= 0; i-- {
		if d := l.branchDepth(same[i].Dst); d > bd {
			best, bd = i, d
		}
	}
	return best
}

// branchDepth is the number of nodes on the longest path from id, not following
// loop edges. Stops before a merge node: from there on the flow is shared by
// several branches and says nothing about the length of any single branch.
func (l *Layout) branchDepth(id string) int {
	if d, ok := l.depth[id]; ok {
		return d
	}
	d := 0
	if l.nonBackIn(id) <= 1 {
		d = 1
		for _, x := range l.outs[id] {
			if !x.Back {
				if k := 1 + l.branchDepth(x.Dst); k > d {
					d = k
				}
			}
		}
	}
	l.depth[id] = d
	return d
}

// spineOf returns the nodes of the spine: from each starting point, follow the
// main branch to the end. A merge node outside the spine is where side branches
// converge, such as a shared error-reporting step, and must not be pulled back
// to the main column.
func (l *Layout) spineOf(order []string) map[string]bool {
	spine := map[string]bool{}
	for _, id := range order {
		if l.nonBackIn(id) > 0 || l.items[id].Attach != "" {
			continue
		}
		for !spine[id] {
			spine[id] = true
			same := l.sameLaneOuts(id)
			if len(same) == 0 {
				break
			}
			id = same[l.mainEdge(same)].Dst
		}
	}
	return spine
}

func (l *Layout) nonBackIn(id string) int {
	n := 0
	for _, y := range l.ins[id] {
		if !y.Back {
			n++
		}
	}
	return n
}

// branchSlots returns the relative column for every same-lane outgoing edge of
// a node.
//
// The main branch keeps column 0. Side branches, those nearest the main branch
// first, shift toward the side the branch leads to, so the arrow leaving the
// branch does not have to loop back across another node. Branches with no clear
// direction alternate right then left. If one side already has a horizontal
// arrow, all side branches move to the other side.
//
// The result depends on hside, that is, on the horizontal arrows placed so far
// at the time of the call.
func (l *Layout) branchSlots(nid string) map[string]int {
	same := l.sameLaneOuts(nid)
	res := map[string]int{}
	if len(same) == 0 {
		return res
	}
	mi := l.mainEdge(same)
	res[same[mi].ID] = 0
	busy := l.hside[nid]
	onlyLeft := busy['R'] && !busy['L']
	onlyRight := busy['L'] && !busy['R']
	next := map[int]int{1: 1, -1: 1}
	for i := len(same) - 1; i >= 0; i-- {
		if i == mi {
			continue
		}
		e := same[i]
		var side int
		switch {
		case onlyLeft:
			side = -1
		case onlyRight:
			side = 1
		default:
			side = l.branchDrift(e)
			if side == 0 {
				side = -1
				if next[1] <= next[-1] {
					side = 1
				}
			}
		}
		res[e.ID] = side * next[side]
		next[side]++
	}
	return res
}

func (l *Layout) branchSlot(e *Edge) int { return l.branchSlots(e.Src)[e.ID] }

// mergeCol returns the column for a node with several incoming same-lane
// branches.
//
// Those branches split off from a common node; the shared flow should return to
// the column of the nearest branching node rather than follow the column of the
// branch placed last. Without a common node ok is false and the caller keeps
// its default placement.
func (l *Layout) mergeCol(same []*Edge, v *Item) (col int, ok bool) {
	var sets []map[string]bool
	for _, e := range same {
		seen := map[string]bool{}
		stack := []string{e.Src}
		for len(stack) > 0 {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[n] {
				continue
			}
			seen[n] = true
			for _, x := range l.ins[n] {
				w := l.items[x.Src]
				if !x.Back && w.Lane == v.Lane && w.Placed {
					stack = append(stack, x.Src)
				}
			}
		}
		sets = append(sets, seen)
	}
	var best *Item
	for id := range sets[0] {
		inAll := true
		for _, s := range sets[1:] {
			if !s[id] {
				inAll = false
				break
			}
		}
		if !inAll {
			continue
		}
		it := l.items[id]
		// Nearest means the largest row; on a tie, the later row in the table.
		// Row order is unique, so this does not depend on map iteration order.
		if best == nil || it.Row > best.Row || (it.Row == best.Row && it.Order > best.Order) {
			best = it
		}
	}
	if best == nil {
		return 0, false
	}
	return best.Col, true
}

// sideBranches counts the same-lane side branches: every same-lane outgoing
// edge except the main branch. The reference implementation counts edges whose
// distance to the main branch is greater than 0; those distances are a
// permutation of 0 to n-1, so both counts are always equal.
func (l *Layout) sideBranches(v *Item) int {
	if n := len(l.sameLaneOuts(v.ID)); n > 0 {
		return n - 1
	}
	return 0
}

// attachSide picks the side for a db or text standing next to a node: the side
// with no connecting edge. Returns 1 for right, -1 for left. If both sides have
// edges, right is chosen.
func (l *Layout) attachSide(v *Item) int {
	right, left := false, false
	all := append(append([]*Edge(nil), l.outs[v.ID]...), l.ins[v.ID]...)
	for _, e := range all {
		if e.Back {
			continue
		}
		// An element that takes incoming wires only at its top keeps both sides
		// free of them, wherever the source is.
		if e.Dst == v.ID && geometryOf(v.Type).EntryTopOnly {
			continue
		}
		oid := e.Src
		if e.Src == v.ID {
			oid = e.Dst
		}
		o := l.items[oid]
		switch {
		case o.Lane > v.Lane:
			right = true
		case o.Lane < v.Lane:
			left = true
		case e.Src == v.ID && l.branchSlot(e) > 0:
			right = true
		case e.Src == v.ID && l.branchSlot(e) < 0:
			left = true
		}
	}
	if !right {
		return 1
	}
	if !left {
		return -1
	}
	return 1
}
