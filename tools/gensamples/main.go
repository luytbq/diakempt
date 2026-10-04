// Command gensamples writes the synthetic clean diagrams of kinds flowcast does
// not draw, into corpus/clean/<kind>/. Styles follow what the draw.io editor
// writes for its own shape libraries, so detection is tested on realistic
// input. The output is deterministic; rerun after changing a generator.
//
//	go run ./tools/gensamples [-out corpus/clean]
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	out := flag.String("out", "corpus/clean", "output directory")
	flag.Parse()
	for kind, gens := range map[string][]func() *page{
		"sequence":     {seqUML(3, 4), seqUML(4, 7), seqUML(5, 10), seqHand(3, 5), seqHand(4, 6)},
		"architecture": {arch(3, 2), arch(4, 3), arch(2, 4)},
		"network":      {network(5), network(8)},
		"mindmap":      {mindmap(5), mindmap(8)},
		"class":        {class(3), class(5), classModel("bank", false), classModel("bank-scattered", true)},
		"er":           {er(3), er(4)},
		"state":        {state(4), state(6)},
		"handflow":     {handflow("basic", 12, false), handflow("library", 12, true), handflow("big-font", 16, false)},
	} {
		dir := filepath.Join(*out, kind)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		for i, g := range gens {
			p := g()
			name := filepath.Join(dir, fmt.Sprintf("%02d-%s.drawio", i+1, p.slug))
			if err := os.WriteFile(name, []byte(p.file()), 0o644); err != nil {
				fail(err)
			}
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

type page struct {
	slug  string
	title string
	cells []string
	n     int
}

func (p *page) id() string {
	p.n++
	return fmt.Sprintf("c%d", p.n)
}

func (p *page) vertex(parent, value, style string, x, y, w, h float64) string {
	id := p.id()
	p.cells = append(p.cells, fmt.Sprintf(`        <mxCell id="%s" value="%s" style="%s" vertex="1" parent="%s">
          <mxGeometry x="%s" y="%s" width="%s" height="%s" as="geometry" />
        </mxCell>`, id, esc(value), style, parent, num(x), num(y), num(w), num(h)))
	return id
}

func (p *page) edge(parent, value, style, src, dst string, pts ...[2]float64) string {
	id := p.id()
	var geo strings.Builder
	if len(pts) > 0 {
		geo.WriteString("\n            <Array as=\"points\">\n")
		for _, q := range pts {
			fmt.Fprintf(&geo, "              <mxPoint x=\"%s\" y=\"%s\" />\n", num(q[0]), num(q[1]))
		}
		geo.WriteString("            </Array>\n          ")
	}
	attrs := ""
	if src != "" {
		attrs += fmt.Sprintf(` source="%s"`, src)
	}
	if dst != "" {
		attrs += fmt.Sprintf(` target="%s"`, dst)
	}
	p.cells = append(p.cells, fmt.Sprintf(`        <mxCell id="%s" value="%s" style="%s" edge="1" parent="%s"%s>
          <mxGeometry relative="1" as="geometry">%s</mxGeometry>
        </mxCell>`, id, esc(value), style, parent, attrs, geo.String()))
	return id
}

// freeEdge is a line with both ends free, as hand-drawn lifelines are.
func (p *page) freeEdge(parent, style string, x1, y1, x2, y2 float64) string {
	id := p.id()
	p.cells = append(p.cells, fmt.Sprintf(`        <mxCell id="%s" value="" style="%s" edge="1" parent="%s">
          <mxGeometry relative="1" as="geometry">
            <mxPoint x="%s" y="%s" as="sourcePoint" />
            <mxPoint x="%s" y="%s" as="targetPoint" />
          </mxGeometry>
        </mxCell>`, id, style, parent, num(x1), num(y1), num(x2), num(y2)))
	return id
}

func (p *page) file() string {
	return fmt.Sprintf(`<mxfile host="Electron" version="24.7.17">
  <diagram id="%s" name="%s">
    <mxGraphModel dx="1200" dy="800" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="1169" pageHeight="827" math="0" shadow="0">
      <root>
        <mxCell id="0" />
        <mxCell id="1" parent="0" />
%s
      </root>
    </mxGraphModel>
  </diagram>
</mxfile>
`, p.slug, esc(p.title), strings.Join(p.cells, "\n"))
}

func esc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func num(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

var actors = []string{"Customer", "Web app", "API", "Payments", "Database", "Mailer", "Queue"}
var verbs = []string{"submit order", "validate", "charge card", "store order", "ack", "send receipt", "enqueue job", "poll status", "return result", "notify"}

// seqUML draws a sequence diagram with the UML lifeline shape.
func seqUML(parts, msgs int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("uml-%dx%d", parts, msgs), title: "Checkout sequence"}
		ids := make([]string, parts)
		height := 80 + float64(msgs)*40
		for i := 0; i < parts; i++ {
			ids[i] = p.vertex("1", actors[i], "shape=umlLifeline;perimeter=lifelinePerimeter;whiteSpace=wrap;html=1;container=1;dropTarget=0;collapsible=0;recursiveResize=0;outlineConnect=0;portConstraint=eastwest;newEdgeStyle={&quot;curved&quot;:0,&quot;rounded&quot;:0};",
				80+float64(i)*160, 40, 100, height)
		}
		for m := 0; m < msgs; m++ {
			a := m % parts
			b := (m*2 + 1) % parts
			if a == b {
				b = (a + 1) % parts
			}
			y := 110 + float64(m)*40
			style := "html=1;verticalAlign=bottom;endArrow=block;curved=0;rounded=0;"
			if m%3 == 2 {
				style = "html=1;verticalAlign=bottom;endArrow=open;dashed=1;endSize=8;curved=0;rounded=0;"
			}
			xa, xb := 130+float64(a)*160, 130+float64(b)*160
			p.edge("1", verbs[m%len(verbs)], style, ids[a], ids[b], [2]float64{xa, y}, [2]float64{xb, y})
		}
		return p
	}
}

// seqHand draws a sequence diagram the way people do without the UML library:
// boxes for participants, dashed lines below them, arrows between the lines.
func seqHand(parts, msgs int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("hand-%dx%d", parts, msgs), title: "Login flow"}
		bottom := 120 + float64(msgs)*45
		for i := 0; i < parts; i++ {
			x := 60 + float64(i)*180
			p.vertex("1", actors[i], "rounded=0;whiteSpace=wrap;html=1;fillColor=#dae8fc;strokeColor=#6c8ebf;", x, 40, 120, 40)
			p.freeEdge("1", "endArrow=none;dashed=1;html=1;", x+60, 80, x+60, bottom)
		}
		for m := 0; m < msgs; m++ {
			a := m % parts
			b := (a + 1 + m%2) % parts
			if a == b {
				b = (a + 1) % parts
			}
			y := 110 + float64(m)*45
			id := p.freeEdge("1", "endArrow=classic;html=1;", 120+float64(a)*180, y, 120+float64(b)*180, y)
			p.cells = append(p.cells, fmt.Sprintf(`        <mxCell id="%s-l" value="%s" style="edgeLabel;html=1;align=center;verticalAlign=middle;resizable=0;points=[];" vertex="1" connectable="0" parent="%s">
          <mxGeometry x="-0.1" y="1" relative="1" as="geometry">
            <mxPoint y="-10" as="offset" />
          </mxGeometry>
        </mxCell>`, id, verbs[m%len(verbs)], id))
		}
		return p
	}
}

// arch draws an architecture diagram: tiers in group containers, services as
// boxes and cylinders, connections both ways without a consistent direction.
func arch(tiers, per int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("tiers-%dx%d", tiers, per), title: "System context"}
		names := []string{"Edge", "Services", "Data", "Ops"}
		var prev []string
		for t := 0; t < tiers; t++ {
			x := 40 + float64(t)*260
			grp := p.vertex("1", names[t%len(names)], "rounded=1;whiteSpace=wrap;html=1;verticalAlign=top;fillColor=#f5f5f5;strokeColor=#666666;dashed=1;container=1;", x, 40, 220, 60+float64(per)*90)
			var cur []string
			for i := 0; i < per; i++ {
				style := "rounded=1;whiteSpace=wrap;html=1;fillColor=#d5e8d4;strokeColor=#82b366;"
				if t == tiers-1 && i%2 == 0 {
					style = "shape=cylinder3;whiteSpace=wrap;html=1;boundedLbl=1;backgroundOutline=1;size=15;fillColor=#fff2cc;strokeColor=#d6b656;"
				}
				cur = append(cur, p.vertex(grp, fmt.Sprintf("%s %d", names[t%len(names)], i+1), style, 40, 50+float64(i)*90, 140, 60))
			}
			for i, c := range cur {
				if len(prev) == 0 {
					continue
				}
				style := "endArrow=classic;startArrow=classic;html=1;"
				if i%2 == 1 {
					style = "endArrow=none;html=1;"
				}
				p.edge("1", "", style, prev[i%len(prev)], c)
				if i+1 < len(prev) {
					p.edge("1", "HTTPS", "endArrow=classic;html=1;", c, prev[i+1])
				}
			}
			prev = cur
		}
		return p
	}
}

// network draws a network: routers and clouds joined by plain lines.
func network(n int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("lan-%d", n), title: "Office network"}
		core := p.vertex("1", "Core switch", "shape=mxgraph.cisco.switches.layer_3_switch;html=1;pointerEvents=1;dashed=0;fillColor=#036897;strokeColor=#ffffff;strokeWidth=2;verticalLabelPosition=bottom;verticalAlign=top;align=center;outlineConnect=0;", 400, 300, 64, 64)
		cloud := p.vertex("1", "Internet", "ellipse;shape=cloud;whiteSpace=wrap;html=1;", 380, 40, 120, 80)
		p.edge("1", "", "endArrow=none;html=1;", cloud, core)
		for i := 0; i < n; i++ {
			a := 2 * math.Pi * float64(i) / float64(n)
			x, y := 400+220*math.Cos(a), 330+180*math.Sin(a)+60
			id := p.vertex("1", fmt.Sprintf("PC-%d", i+1), "shape=mxgraph.cisco.computers_and_peripherals.pc;html=1;pointerEvents=1;dashed=0;fillColor=#036897;strokeColor=#ffffff;strokeWidth=2;verticalLabelPosition=bottom;verticalAlign=top;align=center;outlineConnect=0;", x, y, 78, 53)
			p.edge("1", "", "endArrow=none;html=1;", core, id)
		}
		return p
	}
}

// mindmap draws a mind map: a central idea and branches on curved lines.
func mindmap(n int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("ideas-%d", n), title: "Release ideas"}
		root := p.vertex("1", "Release 2.0", "ellipse;whiteSpace=wrap;html=1;align=center;treeFolding=1;treeMoving=1;newEdgeStyle={&quot;edgeStyle&quot;:&quot;entityRelationEdgeStyle&quot;,&quot;startArrow&quot;:&quot;none&quot;,&quot;endArrow&quot;:&quot;none&quot;,&quot;segment&quot;:10,&quot;curved&quot;:1,&quot;sourcePerimeterSpacing&quot;:0,&quot;targetPerimeterSpacing&quot;:0};", 400, 300, 140, 60)
		topics := []string{"Performance", "Docs", "Pricing", "Mobile", "Security", "Hiring", "Support", "Launch"}
		for i := 0; i < n; i++ {
			side := 1.0
			if i%2 == 1 {
				side = -1
			}
			x := 470 + side*260 - 60
			y := 120 + float64(i/2)*90
			id := p.vertex("1", topics[i%len(topics)], "whiteSpace=wrap;html=1;rounded=1;arcSize=50;align=center;verticalAlign=middle;strokeWidth=1;autosize=1;spacing=4;treeFolding=1;treeMoving=1;newEdgeStyle={&quot;edgeStyle&quot;:&quot;entityRelationEdgeStyle&quot;,&quot;startArrow&quot;:&quot;none&quot;,&quot;endArrow&quot;:&quot;none&quot;,&quot;segment&quot;:10,&quot;curved&quot;:1};", x, y, 120, 30)
			p.edge("1", "", "edgeStyle=entityRelationEdgeStyle;startArrow=none;endArrow=none;segment=10;curved=1;sourcePerimeterSpacing=0;targetPerimeterSpacing=0;", root, id)
		}
		return p
	}
}

// class draws UML classes as the editor's class shape: a swimlane holding
// attribute and method rows, joined by inheritance and association arrows.
func class(n int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("classes-%d", n), title: "Domain model"}
		names := []string{"Order", "Customer", "LineItem", "Product", "Invoice"}
		var ids []string
		for i := 0; i < n; i++ {
			x, y := 60+float64(i%3)*240, 60+float64(i/3)*220
			c := p.vertex("1", names[i], "swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=26;horizontalStack=0;resizeParent=1;resizeParentMax=0;resizeLast=0;collapsible=1;marginBottom=0;whiteSpace=wrap;html=1;", x, y, 160, 120)
			p.vertex(c, "+ id: int", "text;strokeColor=none;fillColor=none;align=left;verticalAlign=top;spacingLeft=4;spacingRight=4;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;whiteSpace=wrap;html=1;", 0, 26, 160, 26)
			p.vertex(c, "+ name: string", "text;strokeColor=none;fillColor=none;align=left;verticalAlign=top;spacingLeft=4;spacingRight=4;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;whiteSpace=wrap;html=1;", 0, 52, 160, 26)
			p.vertex(c, "", "line;strokeWidth=1;fillColor=none;align=left;verticalAlign=middle;spacingTop=-1;spacingLeft=3;spacingRight=3;rotatable=0;labelPosition=right;points=[];portConstraint=eastwest;strokeColor=inherit;", 0, 78, 160, 8)
			p.vertex(c, "+ save(): void", "text;strokeColor=none;fillColor=none;align=left;verticalAlign=top;spacingLeft=4;spacingRight=4;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;whiteSpace=wrap;html=1;", 0, 86, 160, 26)
			ids = append(ids, c)
		}
		for i := 1; i < n; i++ {
			style := "endArrow=block;endSize=16;endFill=0;html=1;"
			if i%2 == 0 {
				style = "endArrow=open;html=1;endSize=12;startArrow=diamondThin;startSize=14;startFill=1;edgeStyle=orthogonalEdgeStyle;align=left;verticalAlign=bottom;"
			}
			p.edge("1", "", style, ids[i], ids[(i-1)/2])
		}
		return p
	}
}

// er draws entity tables joined by crow's-foot relations.
func er(n int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("tables-%d", n), title: "Schema"}
		names := []string{"users", "orders", "items", "payments"}
		var ids []string
		for i := 0; i < n; i++ {
			x := 60 + float64(i)*230
			t := p.vertex("1", names[i], "shape=table;startSize=30;container=1;collapsible=1;childLayout=tableLayout;fixedRows=1;rowLines=0;fontStyle=1;align=center;resizeLast=1;html=1;", x, 80, 180, 120)
			for r := 0; r < 3; r++ {
				p.vertex(t, "", "shape=tableRow;horizontal=0;startSize=0;swimlaneHead=0;swimlaneBody=0;fillColor=none;collapsible=0;dropTarget=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;top=0;left=0;right=0;bottom=0;html=1;", 0, 30+float64(r)*30, 180, 30)
			}
			ids = append(ids, t)
		}
		for i := 1; i < n; i++ {
			p.edge("1", "", "edgeStyle=entityRelationEdgeStyle;fontSize=12;html=1;endArrow=ERmany;startArrow=ERmandOne;", ids[i-1], ids[i])
		}
		return p
	}
}

// state draws a state machine: a filled start dot, rounded states, a final
// state, transitions that loop back.
func state(n int) func() *page {
	return func() *page {
		p := &page{slug: fmt.Sprintf("states-%d", n), title: "Order lifecycle"}
		start := p.vertex("1", "", "ellipse;html=1;shape=startState;fillColor=#000000;strokeColor=#ff0000;", 60, 160, 30, 30)
		names := []string{"New", "Paid", "Packed", "Shipped", "Delivered", "Returned"}
		var ids []string
		for i := 0; i < n; i++ {
			ids = append(ids, p.vertex("1", names[i], "rounded=1;whiteSpace=wrap;html=1;arcSize=40;fontColor=#000000;fillColor=#ffffc0;strokeColor=#ff0000;", 140+float64(i)*170, 150, 120, 50))
		}
		end := p.vertex("1", "", "ellipse;html=1;shape=endState;fillColor=#000000;strokeColor=#ff0000;", 160+float64(n)*170, 160, 30, 30)
		p.edge("1", "", "edgeStyle=orthogonalEdgeStyle;html=1;verticalAlign=bottom;endArrow=open;endSize=8;strokeColor=#ff0000;", start, ids[0])
		for i := 1; i < n; i++ {
			p.edge("1", "next", "edgeStyle=orthogonalEdgeStyle;html=1;verticalAlign=bottom;endArrow=open;endSize=8;strokeColor=#ff0000;", ids[i-1], ids[i])
		}
		for i := 2; i < n; i += 2 {
			p.edge("1", "retry", "edgeStyle=orthogonalEdgeStyle;html=1;verticalAlign=bottom;endArrow=open;endSize=8;strokeColor=#ff0000;", ids[i], ids[i-2], [2]float64{200 + float64(i)*170, 250}, [2]float64{200 + float64(i-2)*170, 250})
		}
		p.edge("1", "", "edgeStyle=orthogonalEdgeStyle;html=1;verticalAlign=bottom;endArrow=open;endSize=8;strokeColor=#ff0000;", ids[n-1], end)
		return p
	}
}

// handflow draws an approval flowchart the way people do in the editor: shapes
// placed by hand slightly off a grid, default styles (or the flowchart
// library), an actor icon with its label below it, and a chosen font size.
func handflow(slug string, font float64, library bool) func() *page {
	return func() *page {
		p := &page{slug: slug, title: "Expense approval"}
		fs := ""
		if font != 12 {
			fs = fmt.Sprintf("fontSize=%s;", num(font))
		}
		proc, dec, term := "rounded=1;whiteSpace=wrap;html=1;", "rhombus;whiteSpace=wrap;html=1;", "ellipse;whiteSpace=wrap;html=1;"
		if library {
			proc = "shape=mxgraph.flowchart.process;whiteSpace=wrap;html=1;"
			dec = "shape=mxgraph.flowchart.decision;whiteSpace=wrap;html=1;"
			term = "shape=mxgraph.flowchart.terminator;whiteSpace=wrap;html=1;"
		}
		actor := p.vertex("1", "Employee", "shape=umlActor;verticalLabelPosition=bottom;verticalAlign=top;html=1;outlineConnect=0;"+fs, 233, 20, 30, 60)
		submit := p.vertex("1", "Submit expense report", proc+fs, 188, 125, 130, 55)
		check := p.vertex("1", "Amount over 500?", dec+fs, 178, 220, 150, 80)
		manager := p.vertex("1", "Manager approves", proc+fs, 380, 232, 130, 55)
		ok := p.vertex("1", "Approved?", dec+fs, 382, 330, 125, 75)
		reject := p.vertex("1", "Notify rejection", proc+fs, 560, 345, 130, 50)
		pay := p.vertex("1", "Finance pays out", proc+fs, 195, 430, 120, 55)
		done := p.vertex("1", "Done", term+fs, 215, 530, 80, 45)
		e := "edgeStyle=orthogonalEdgeStyle;rounded=0;orthogonalLoop=1;jettySize=auto;html=1;" + fs
		p.edge("1", "", e, actor, submit)
		p.edge("1", "", e, submit, check)
		p.edge("1", "Yes", e, check, manager)
		p.edge("1", "No", e, check, pay)
		p.edge("1", "", e, manager, ok)
		p.edge("1", "No", e, ok, reject)
		p.edge("1", "Yes", e, ok, pay)
		p.edge("1", "", e, reject, done, [2]float64{625, 552})
		p.edge("1", "", e, pay, done)
		return p
	}
}

// classBox draws a UML class with the editor's class shape: a title, attribute
// rows, a divider and method rows. The height follows the rows.
func (p *page) classBox(name string, attrs, methods []string, x, y float64) string {
	const row, w = 26.0, 180.0
	h := 26 + row*float64(len(attrs)+len(methods)) + 8
	c := p.vertex("1", name, "swimlane;fontStyle=1;align=center;verticalAlign=top;childLayout=stackLayout;horizontal=1;startSize=26;horizontalStack=0;resizeParent=1;resizeParentMax=0;resizeLast=0;collapsible=1;marginBottom=0;whiteSpace=wrap;html=1;", x, y, w, h)
	rowStyle := "text;strokeColor=none;fillColor=none;align=left;verticalAlign=top;spacingLeft=4;spacingRight=4;overflow=hidden;rotatable=0;points=[[0,0.5],[1,0.5]];portConstraint=eastwest;whiteSpace=wrap;html=1;"
	ry := 26.0
	for _, a := range attrs {
		p.vertex(c, a, rowStyle, 0, ry, w, row)
		ry += row
	}
	p.vertex(c, "", "line;strokeWidth=1;fillColor=none;align=left;verticalAlign=middle;spacingTop=-1;spacingLeft=3;spacingRight=3;rotatable=0;labelPosition=right;points=[];portConstraint=eastwest;strokeColor=inherit;", 0, ry, w, 8)
	ry += 8
	for _, m := range methods {
		p.vertex(c, m, rowStyle, 0, ry, w, row)
		ry += row
	}
	return c
}

// endLabel puts a multiplicity next to one end of a wire, as the editor's
// association shapes do: at -1 for the source end, 1 for the target end.
func (p *page) endLabel(wire, value string, at float64) {
	id := p.id()
	align := "left"
	if at > 0 {
		align = "right"
	}
	p.cells = append(p.cells, fmt.Sprintf(`        <mxCell id="%s" value="%s" style="edgeLabel;resizable=0;html=1;align=%s;verticalAlign=bottom;" vertex="1" connectable="0" parent="%s">
          <mxGeometry x="%s" relative="1" as="geometry" />
        </mxCell>`, id, esc(value), align, wire, num(at)))
}

// classModel draws a banking domain model: an interface and an abstract class
// with subclasses, compositions, an aggregation, associations with
// multiplicities and a dependency. scattered places the classes the way a
// model grows over time, out of any order.
func classModel(slug string, scattered bool) func() *page {
	return func() *page {
		p := &page{slug: slug, title: "Banking domain"}
		pos := map[string][2]float64{
			"Bank": {320, 40}, "Branch": {320, 260}, "Customer": {40, 260}, "Account": {600, 260},
			"Checking": {440, 520}, "Savings": {660, 520}, "Credit": {880, 520},
			"Payable": {1000, 260}, "Invoice": {1100, 520}, "Transaction": {600, 760},
		}
		if scattered {
			pos = map[string][2]float64{
				"Bank": {700, 600}, "Branch": {40, 40}, "Customer": {420, 380}, "Account": {60, 520},
				"Checking": {700, 60}, "Savings": {380, 40}, "Credit": {1000, 380},
				"Payable": {960, 40}, "Invoice": {420, 760}, "Transaction": {40, 820},
			}
		}
		at := func(n string) (float64, float64) { return pos[n][0], pos[n][1] }
		id := map[string]string{}
		x, y := at("Bank")
		id["Bank"] = p.classBox("Bank", []string{"- name: String", "- swift: String"}, []string{"+ openBranch(): Branch"}, x, y)
		x, y = at("Branch")
		id["Branch"] = p.classBox("Branch", []string{"- code: String", "- address: String"}, []string{"+ accounts(): List"}, x, y)
		x, y = at("Customer")
		id["Customer"] = p.classBox("Customer", []string{"- id: UUID", "- name: String", "- email: String"}, []string{"+ open(type): Account"}, x, y)
		x, y = at("Account")
		id["Account"] = p.classBox("<i>Account</i>", []string{"# number: String", "# balance: Money"}, []string{"+ deposit(m: Money)", "+ withdraw(m: Money)"}, x, y)
		x, y = at("Checking")
		id["Checking"] = p.classBox("CheckingAccount", []string{"- overdraft: Money"}, []string{"+ writeCheck()"}, x, y)
		x, y = at("Savings")
		id["Savings"] = p.classBox("SavingsAccount", []string{"- rate: Percent"}, []string{"+ accrue()"}, x, y)
		x, y = at("Credit")
		id["Credit"] = p.classBox("CreditAccount", []string{"- limit: Money"}, []string{"+ statement()"}, x, y)
		x, y = at("Payable")
		id["Payable"] = p.classBox("&laquo;interface&raquo;<br>Payable", nil, []string{"+ amountDue(): Money", "+ pay(m: Money)"}, x, y)
		x, y = at("Invoice")
		id["Invoice"] = p.classBox("Invoice", []string{"- due: Date", "- total: Money"}, []string{"+ send()"}, x, y)
		x, y = at("Transaction")
		id["Transaction"] = p.classBox("Transaction", []string{"- at: Instant", "- amount: Money"}, []string{"+ reverse()"}, x, y)
		inherit := "endArrow=block;endSize=16;endFill=0;html=1;"
		realize := "endArrow=block;dashed=1;endFill=0;endSize=12;html=1;"
		compose := "endArrow=none;html=1;startArrow=diamondThin;startSize=14;startFill=1;edgeStyle=orthogonalEdgeStyle;"
		aggregate := "endArrow=open;html=1;endSize=12;startArrow=diamondThin;startSize=14;startFill=0;edgeStyle=orthogonalEdgeStyle;"
		assoc := "endArrow=open;html=1;endSize=12;edgeStyle=orthogonalEdgeStyle;"
		depend := "endArrow=open;endSize=12;dashed=1;html=1;"
		p.edge("1", "", inherit, id["Checking"], id["Account"])
		p.edge("1", "", inherit, id["Savings"], id["Account"])
		p.edge("1", "", inherit, id["Credit"], id["Account"])
		p.edge("1", "", realize, id["Credit"], id["Payable"])
		p.edge("1", "", realize, id["Invoice"], id["Payable"])
		w := p.edge("1", "", compose, id["Bank"], id["Branch"])
		p.endLabel(w, "1", -1)
		p.endLabel(w, "1..*", 1)
		w = p.edge("1", "", aggregate, id["Branch"], id["Account"])
		p.endLabel(w, "0..*", 1)
		w = p.edge("1", "owns", assoc, id["Customer"], id["Account"])
		p.endLabel(w, "1", -1)
		p.endLabel(w, "1..*", 1)
		w = p.edge("1", "", assoc, id["Customer"], id["Invoice"])
		p.endLabel(w, "0..*", 1)
		p.edge("1", "", depend, id["Transaction"], id["Account"])
		return p
	}
}
