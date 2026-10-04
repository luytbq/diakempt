package normalize

import (
	"os"
	"testing"

	"github.com/luytbq/diakempt/doc"
)

func TestRewrites(t *testing.T) {
	data, err := os.ReadFile("../corpus/cases/normalize.drawio")
	if err != nil {
		t.Fatal(err)
	}
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	p := d.Pages[0]
	box := p.Cell("box").AbsBounds()
	ds := Run(p)
	if len(ds) != 4 {
		for _, x := range ds {
			t.Log(x.Msg)
		}
		t.Fatalf("%d rewrites, want 4", len(ds))
	}
	if p.Cell("g") != nil || p.Cell("boxtext") != nil || p.Cell("note") != nil {
		t.Error("group, stacked text or label text still there")
	}
	if c := p.Cell("box"); c.Parent() != "1" || c.AbsBounds() != box || c.Label() != "Receive order" {
		t.Errorf("box: parent %q, bounds %+v, label %q", c.Parent(), c.AbsBounds(), c.Label())
	}
	if got := p.Cell("w1").Source(); got != "box" {
		t.Errorf("w1 source %q, want box", got)
	}
	if got := p.Cell("w3").Target(); got != "ship" {
		t.Errorf("w3 target %q, want ship", got)
	}
	if got := p.Cell("w2").Label(); got != "in stock" {
		t.Errorf("w2 label %q", got)
	}
	again, _, err := doc.Parse(d.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(Run(again.Pages[0])) != 0 {
		t.Error("a second run rewrote something")
	}
}
