package doc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luytbq/diakempt/geom"
	"github.com/luytbq/diakempt/issue"
)

const cases = "../corpus/cases"

func load(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(cases, name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func parse(t *testing.T, data []byte) *Document {
	t.Helper()
	d, _, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// canonical renders a document so that two equivalent documents compare equal:
// every page decompressed, attributes and text as parsed.
func canonical(d *Document) string {
	var b strings.Builder
	b.WriteString(d.root.Tag)
	for _, p := range d.Pages {
		b.WriteString("\n--- page " + p.Name + "\n")
		if p.model != nil {
			b.WriteString(p.model.String())
		}
	}
	return b.String()
}

func TestRoundTripIsEquivalent(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join(cases, "*"))
	n := 0
	for _, f := range files {
		name := filepath.Base(f)
		if strings.Contains(name, ".drawio.") {
			continue
		}
		n++
		t.Run(name, func(t *testing.T) {
			in := load(t, name)
			d1 := parse(t, in)
			out := d1.Bytes()
			d2 := parse(t, out)
			if canonical(d1) != canonical(d2) {
				t.Fatalf("round trip changed the document")
			}
			if again := d2.Bytes(); string(again) != string(out) {
				t.Fatalf("writing is not stable on its own output")
			}
		})
	}
	if n == 0 {
		t.Fatal("no corpus files found")
	}
}

func TestUncompressedFileRoundTripsByteForByte(t *testing.T) {
	for _, name := range []string{"basic.drawio", "nested.drawio"} {
		in := load(t, name)
		if out := parse(t, in).Bytes(); string(out) != string(in) {
			t.Errorf("%s: output differs from input:\n%s", name, out)
		}
	}
}

func TestUntouchedCompressedPageKeepsItsText(t *testing.T) {
	in := load(t, "multipage-compressed.drawio")
	d := parse(t, in)
	if !d.Pages[0].Compressed || d.Pages[1].Compressed {
		t.Fatalf("compression flags = %v, %v; want true, false", d.Pages[0].Compressed, d.Pages[1].Compressed)
	}
	if out := d.Bytes(); string(out) != string(in) {
		t.Errorf("output differs from input")
	}
}

func TestEditedCompressedPageStaysCompressed(t *testing.T) {
	d := parse(t, load(t, "multipage-compressed.drawio"))
	p := d.Pages[0]
	p.Cell("task").SetAbsBounds(geom.Rect{X: 300, Y: 130, W: 120, H: 60})
	d2 := parse(t, d.Bytes())
	if !d2.Pages[0].Compressed {
		t.Fatal("page lost its compression")
	}
	if got := d2.Pages[0].Cell("task").AbsBounds(); got.X != 300 {
		t.Errorf("x = %v, want 300", got.X)
	}
	if d2.Pages[0].Cell("task").Label() != "Check &amp; approve<br>the order" {
		t.Errorf("label changed: %q", d2.Pages[0].Cell("task").Label())
	}
}

func TestBareModelKeepsDeclaration(t *testing.T) {
	in := load(t, "bare-model.xml")
	d := parse(t, in)
	if len(d.Pages) != 1 {
		t.Fatalf("pages = %d", len(d.Pages))
	}
	out := string(d.Bytes())
	if !strings.HasPrefix(out, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- exported by another tool -->\n<mxGraphModel>") {
		t.Errorf("prefix lost:\n%s", out)
	}
	if got := d.Pages[0].Cell("a").Label(); got != "Line one\nline two" {
		t.Errorf("label = %q", got)
	}
}

func TestEmbeddedAndForeignFilesAreRejected(t *testing.T) {
	for name, code := range map[string]string{
		"embedded.drawio.svg": CodeEmbedded,
		"embedded.drawio.png": CodeEmbedded,
	} {
		_, _, err := Parse(load(t, name))
		var is issue.Issue
		if !errors.As(err, &is) || is.Code != code {
			t.Errorf("%s: err = %v, want code %s", name, err, code)
		}
	}
	for in, code := range map[string]string{
		"<html><body/></html>":                     CodeNotDrawio,
		"<svg xmlns='x'><rect/></svg>":             CodeNotDrawio,
		"just text":                                CodeNotDrawio,
		"<mxfile></mxfile>":                        CodeNoPages,
		"<mxfile><diagram>%%%</diagram>":           CodeMalformed,
		"<mxfile><diagram>AAAA</diagram></mxfile>": CodePageDecode,
	} {
		_, _, err := Parse([]byte(in))
		var is issue.Issue
		if !errors.As(err, &is) || is.Code != code {
			t.Errorf("%q: err = %v, want code %s", in, err, code)
		}
	}
}

func TestNestedGeometryIsAbsolute(t *testing.T) {
	p := parse(t, load(t, "nested.drawio")).Pages[0]
	for id, want := range map[string]geom.Rect{
		"pool": {X: 100, Y: 100, W: 400, H: 200},
		"lane": {X: 120, Y: 100, W: 380, H: 200},
		"pay":  {X: 160, Y: 160, W: 100, H: 40},
		"box":  {X: 320, Y: 150, W: 120, H: 80},
		"note": {X: 540, Y: 120, W: 120, H: 60},
	} {
		if got := p.Cell(id).AbsBounds(); got != want {
			t.Errorf("%s: %+v, want %+v", id, got, want)
		}
	}
	w := p.Cell("w")
	if pts := w.Points(); len(pts) != 1 || pts[0] != (geom.Point{X: 280, Y: 180}) {
		t.Errorf("points = %v", pts)
	}
	if pt, ok := w.TerminalPoint(false); !ok || pt != (geom.Point{X: 290, Y: 180}) {
		t.Errorf("target point = %v %v", pt, ok)
	}
	if _, ok := w.TerminalPoint(true); ok {
		t.Errorf("an attached source has no stored point here")
	}
}

func TestUserObjectKeepsWrapper(t *testing.T) {
	d := parse(t, load(t, "nested.drawio"))
	c := d.Pages[0].Cell("pay")
	if c.Label() != "Pay <b>now</b>" || c.Elem.Attr("owner") != "team-a" {
		t.Fatalf("wrapper not read: %q %q", c.Label(), c.Elem.Attr("owner"))
	}
	c.SetAbsBounds(geom.Rect{X: 170, Y: 160, W: 100, H: 40})
	out := string(d.Bytes())
	if !strings.Contains(out, `<UserObject label="Pay &lt;b&gt;now&lt;/b&gt;" owner="team-a" link="https://example.com" id="pay">`) {
		t.Errorf("wrapper changed")
	}
	if !strings.Contains(out, `<mxGeometry x="50" y="60" width="100" height="40" as="geometry" />`) {
		t.Errorf("geometry not written relative to the lane")
	}
}

func TestLayersAndRoot(t *testing.T) {
	p := parse(t, load(t, "nested.drawio")).Pages[0]
	if !p.Cell("0").IsRoot() || !p.Cell("1").IsLayer() || !p.Cell("layer2").IsLayer() || p.Cell("pool").IsLayer() {
		t.Error("root and layers misread")
	}
	if got := len(p.Children("grp")); got != 2 {
		t.Errorf("group children = %d", got)
	}
}

func TestSetPointsRemovesEmptyList(t *testing.T) {
	d := parse(t, load(t, "nested.drawio"))
	w := d.Pages[0].Cell("w")
	w.SetPoints([]geom.Point{{X: 300, Y: 180}, {X: 300, Y: 200}})
	if pts := w.Points(); len(pts) != 2 || pts[1] != (geom.Point{X: 300, Y: 200}) {
		t.Errorf("points = %v", pts)
	}
	w.SetPoints(nil)
	if strings.Contains(string(d.Bytes()), `as="points"`) {
		t.Errorf("empty points list was kept")
	}
}

func TestStyleEditKeepsOrder(t *testing.T) {
	st := ParseStyle("ellipse;whiteSpace=wrap;html=1;fillColor=#fff;")
	st.Set("whiteSpace", "nowrap")
	st.Set("exitX", "1")
	if got := st.String(); got != "ellipse;whiteSpace=nowrap;html=1;fillColor=#fff;exitX=1;" {
		t.Errorf("got %q", got)
	}
	if st.Shape() != "ellipse" {
		t.Errorf("shape = %q", st.Shape())
	}
	st.Del("html")
	if got := st.String(); got != "ellipse;whiteSpace=nowrap;fillColor=#fff;exitX=1;" {
		t.Errorf("got %q", got)
	}
	if got := ParseStyle("shape=cylinder3;rounded=0").Shape(); got != "cylinder3" {
		t.Errorf("shape = %q", got)
	}
	if got := ParseStyle("").Shape(); got != "rectangle" {
		t.Errorf("shape = %q", got)
	}
}

func TestCompressionRoundTrip(t *testing.T) {
	xml := `<mxGraphModel><root><mxCell id="0" value="Tiếng Việt &amp; 100%" /></root></mxGraphModel>`
	got, err := decompressPage(compressPage(xml))
	if err != nil || got != xml {
		t.Errorf("got %q, %v", got, err)
	}
}
