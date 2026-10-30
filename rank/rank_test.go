package rank

import (
	"testing"

	"github.com/luytbq/diakempt/geom"
)

func TestAddRefusesARelationThatClosesACycle(t *testing.T) {
	g := New[string]()
	if !g.Add("a", "b") || !g.Add("b", "c") {
		t.Fatal("a chain must be accepted")
	}
	if g.Add("c", "a") {
		t.Error("c above a closes a cycle")
	}
	r := g.Ranks([]string{"a", "b", "c"})
	if r["a"] != 0 || r["b"] != 1 || r["c"] != 2 {
		t.Errorf("ranks %v", r)
	}
}

func TestRanksPlaceAnItemBelowTheLowestItemAboveIt(t *testing.T) {
	g := New[string]()
	g.Add("a", "b")
	g.Add("b", "c")
	g.Add("a", "c")
	r := g.Ranks([]string{"a", "b", "c", "lone"})
	if r["c"] != 2 {
		t.Errorf("c hangs from b in rank 1, got rank %d", r["c"])
	}
	if r["lone"] != 0 || g.Ranked("lone") {
		t.Errorf("an item in no relation is unranked in rank 0, got %d", r["lone"])
	}
}

func TestDrawnOpensARowBelowTheMiddleOfTheRowSoFar(t *testing.T) {
	boxes := map[string]geom.Rect{
		"a": {X: 0, Y: 0, W: 50, H: 40},
		"b": {X: 100, Y: 15, W: 50, H: 40}, // starts above a's middle
		"c": {X: 0, Y: 30, W: 50, H: 40},   // starts below a's middle
	}
	r := Drawn([]string{"c", "b", "a"}, func(s string) geom.Rect { return boxes[s] })
	if r["a"] != 0 || r["b"] != 0 || r["c"] != 1 {
		t.Errorf("rows %v", r)
	}
}
