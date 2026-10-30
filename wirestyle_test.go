package diakempt

import (
	"strings"
	"testing"

	"github.com/luytbq/diakempt/doc"
)

// throughB is a chain A, B, C with a straight wire from A to C drawn through B.
const throughB = `<mxfile><diagram name="P" id="p"><mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>
<mxCell id="a" value="A" style="rounded=0;html=1;" vertex="1" parent="1"><mxGeometry x="40" y="100" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="b" value="B" style="rounded=0;html=1;" vertex="1" parent="1"><mxGeometry x="200" y="100" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="c" value="C" style="rounded=0;html=1;" vertex="1" parent="1"><mxGeometry x="360" y="100" width="80" height="40" as="geometry"/></mxCell>
<mxCell id="w1" style="STYLEendArrow=classic;html=1;" edge="1" parent="1" source="a" target="c"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="w2" style="endArrow=classic;html=1;" edge="1" parent="1" source="a" target="b"><mxGeometry relative="1" as="geometry"/></mxCell>
<mxCell id="w3" style="endArrow=classic;html=1;" edge="1" parent="1" source="b" target="c"><mxGeometry relative="1" as="geometry"/></mxCell>
</root></mxGraphModel></diagram></mxfile>`

// wireStyleOnly leaves wirestyle the only operation that can clear w1.
var wireStyleOnly = Options{Level: Aggressive, Kind: "unknown", Ops: map[string]bool{"reroute": false, "relayout": false}}

func styleOf(t *testing.T, data []byte, id string) doc.Style {
	t.Helper()
	d, _, err := doc.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range d.Pages[0].Cells {
		if c.ID == id {
			return c.Style()
		}
	}
	t.Fatalf("no cell %s", id)
	return doc.Style{}
}

func TestWireStyleTurnsABrokenStraightWireOrthogonalAtAggressive(t *testing.T) {
	res, err := Tidy([]byte(strings.Replace(throughB, "STYLE", "", 1)), wireStyleOnly)
	if err != nil {
		t.Fatal(err)
	}
	if st := styleOf(t, res.Output, "w1"); st.Value("edgeStyle", "") != "orthogonalEdgeStyle" {
		t.Errorf("w1 style %q", st.String())
	}
	d := res.Report.Diagrams[0]
	if d.After.WiresThroughNodes != 0 {
		t.Errorf("%d wires through nodes after", d.After.WiresThroughNodes)
	}
	found := false
	for _, det := range d.Details {
		found = found || (det.Op == "wirestyle" && det.Msg == "made wire w1 orthogonal")
	}
	if !found {
		t.Errorf("no wirestyle detail in %+v", d.Details)
	}
}

func TestWireStyleKeepsCurvedWiresAndLevelsBelowAggressive(t *testing.T) {
	curved := wireStyleOnly
	normal := wireStyleOnly
	normal.Level = Normal
	for name, tc := range map[string]struct {
		style string
		opt   Options
	}{
		"curved at aggressive":   {"curved=1;", curved},
		"straight at normal":     {"", normal},
		"straight, switched off": {"", Options{Level: Aggressive, Kind: "unknown", Ops: map[string]bool{"reroute": false, "relayout": false, "wirestyle": false}}},
	} {
		res, err := Tidy([]byte(strings.Replace(throughB, "STYLE", tc.style, 1)), tc.opt)
		if err != nil {
			t.Fatal(err)
		}
		if st := styleOf(t, res.Output, "w1"); st.Value("edgeStyle", "") != "" {
			t.Errorf("%s: w1 became %q", name, st.String())
		}
	}
}
