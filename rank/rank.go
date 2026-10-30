// Package rank numbers the ranks of a layered layout: the rows a shape is
// placed in, counted from the top.
package rank

import (
	"math"
	"sort"

	"github.com/luytbq/diakempt/geom"
)

// Graph holds which items are drawn above which. It never holds a cycle.
type Graph[T comparable] struct {
	below  map[T][]T
	ranked map[T]bool
}

// New returns an empty graph.
func New[T comparable]() *Graph[T] {
	return &Graph[T]{below: map[T][]T{}, ranked: map[T]bool{}}
}

// Add puts up above down. It refuses, and returns false, a relation that
// would close a cycle with those added before, so relations added first win.
func (g *Graph[T]) Add(up, down T) bool {
	if g.reaches(down, up) {
		return false
	}
	g.below[up] = append(g.below[up], down)
	g.ranked[up], g.ranked[down] = true, true
	return true
}

// Ranked reports whether an item takes part in a relation.
func (g *Graph[T]) Ranked(n T) bool { return g.ranked[n] }

// Empty reports whether the graph holds no relation.
func (g *Graph[T]) Empty() bool { return len(g.ranked) == 0 }

func (g *Graph[T]) reaches(from, to T) bool {
	seen := map[T]bool{}
	stack := []T{from}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n == to {
			return true
		}
		if !seen[n] {
			seen[n] = true
			stack = append(stack, g.below[n]...)
		}
	}
	return false
}

// Ranks gives each item one rank below the lowest item it hangs from; an
// item nothing hangs above is in rank 0.
func (g *Graph[T]) Ranks(items []T) map[T]int {
	rank := map[T]int{}
	var visit func(n T, r int)
	visit = func(n T, r int) {
		if old, ok := rank[n]; ok && old >= r {
			return
		}
		rank[n] = r
		for _, m := range g.below[n] {
			visit(m, r+1)
		}
	}
	above := map[T]int{}
	for _, ms := range g.below {
		for _, m := range ms {
			above[m]++
		}
	}
	for _, n := range items {
		if above[n] == 0 {
			visit(n, 0)
		}
	}
	return rank
}

// Drawn numbers the rows items were drawn in, from the top: an item starting
// below the middle of the row so far opens a new one.
func Drawn[T comparable](items []T, box func(T) geom.Rect) map[T]int {
	sorted := append([]T(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return box(sorted[i]).Y < box(sorted[j]).Y })
	rank := map[T]int{}
	r, mid := 0, math.Inf(-1)
	for i, n := range sorted {
		b := box(n)
		if i > 0 && b.Y > mid {
			r++
		}
		if i == 0 || b.Y > mid {
			mid = b.Center().Y
		}
		rank[n] = r
	}
	return rank
}
