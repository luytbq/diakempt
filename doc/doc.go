// Package doc reads a draw.io file into pages and cells, lets later steps edit
// geometry, and writes the file back with everything else untouched.
//
// Reading a file and writing it back without edits yields an equivalent file:
// same elements, attributes and attribute order, same text and whitespace between
// elements, each page compressed or not as it was. Comments inside the tree are
// dropped.
package doc

import (
	"bytes"
	"strings"

	"github.com/luytbq/diakempt/internal/xmltree"
	"github.com/luytbq/diakempt/issue"
)

// Document is a parsed draw.io file.
type Document struct {
	prefix string // declaration and comments before the root element
	suffix string // whatever follows the root element
	root   *xmltree.Element
	Pages  []*Page
}

// Page is one diagram element of the file.
type Page struct {
	Index int
	Name  string
	ID    string
	// Compressed is true when the page was stored deflated and base64 encoded.
	Compressed bool

	diagram  *xmltree.Element // nil when the file is a bare mxGraphModel
	model    *xmltree.Element // nil for a page with no content
	origXML  string           // the decompressed model as read
	origText string           // the compressed text as read

	Cells    []*Cell
	byID     map[string]*Cell
	children map[string][]*Cell
}

// Cell is one mxCell, possibly wrapped in an object or UserObject element.
type Cell struct {
	ID string
	// Elem is the element directly under root: the wrapper when there is one,
	// otherwise the mxCell itself.
	Elem *xmltree.Element
	// Node is the mxCell element.
	Node *xmltree.Element
	Page *Page
}

// Error codes returned by Parse.
const (
	CodeMalformed  = "input.malformed"
	CodeNotDrawio  = "input.not_drawio"
	CodeEmbedded   = "input.embedded"
	CodeNoPages    = "input.no_pages"
	CodePageDecode = "input.page_decode"
)

// Parse reads a draw.io document. The format is told from the content, not
// from a file name.
func Parse(data []byte) (*Document, []issue.Issue, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if bytes.HasPrefix(data, []byte("\x89PNG")) {
		return nil, nil, issue.FileError(CodeEmbedded,
			"embedded diagrams are not supported yet, export to .drawio first")
	}
	start := rootStart(data)
	if start < 0 {
		return nil, nil, issue.FileError(CodeNotDrawio, "not a draw.io document: no XML element found")
	}
	root, err := xmltree.Parse(data)
	if err != nil {
		return nil, nil, issue.FileError(CodeMalformed, "cannot read the XML: %v", err)
	}
	d := &Document{prefix: string(data[:start]), root: root}
	end := bytes.LastIndexByte(data, '>')
	if end >= 0 {
		d.suffix = string(data[end+1:])
	}
	var warns []issue.Issue
	switch root.Tag {
	case "mxfile":
		diagrams := root.FindAll("diagram")
		if len(diagrams) == 0 {
			return nil, nil, issue.FileError(CodeNoPages, "the file has no pages")
		}
		for i, dg := range diagrams {
			p := &Page{Index: i, Name: dg.Attr("name"), ID: dg.Attr("id"), diagram: dg}
			if m := dg.Find("mxGraphModel"); m != nil {
				p.model = m
			} else if text := strings.TrimSpace(dg.Text); text != "" {
				xml, err := decompressPage(text)
				if err != nil {
					return nil, nil, issue.FileError(CodePageDecode, "cannot decompress page %d %q: %v", i+1, p.Name, err)
				}
				m, err := xmltree.Parse([]byte(xml))
				if err != nil {
					return nil, nil, issue.FileError(CodePageDecode, "cannot read page %d %q: %v", i+1, p.Name, err)
				}
				p.model, p.Compressed, p.origText = m, true, dg.Text
				p.origXML = m.String()
			}
			warns = append(warns, p.index()...)
			d.Pages = append(d.Pages, p)
		}
	case "mxGraphModel":
		p := &Page{model: root}
		warns = append(warns, p.index()...)
		d.Pages = append(d.Pages, p)
	case "svg":
		if strings.Contains(root.Attr("content"), "mxfile") || strings.Contains(root.Attr("content"), "mxGraphModel") {
			return nil, nil, issue.FileError(CodeEmbedded,
				"embedded diagrams are not supported yet, export to .drawio first")
		}
		return nil, nil, issue.FileError(CodeNotDrawio, "not a draw.io document: an SVG image without a diagram")
	default:
		return nil, nil, issue.FileError(CodeNotDrawio, "not a draw.io document: root element is <%s>", root.Tag)
	}
	return d, warns, nil
}

// rootStart returns the offset of the root element's "<", skipping the XML
// declaration, comments and a doctype.
func rootStart(data []byte) int {
	i := 0
	for {
		j := bytes.IndexByte(data[i:], '<')
		if j < 0 {
			return -1
		}
		i += j
		rest := data[i:]
		switch {
		case bytes.HasPrefix(rest, []byte("<?")):
			k := bytes.Index(rest, []byte("?>"))
			if k < 0 {
				return -1
			}
			i += k + 2
		case bytes.HasPrefix(rest, []byte("<!--")):
			k := bytes.Index(rest, []byte("-->"))
			if k < 0 {
				return -1
			}
			i += k + 3
		case bytes.HasPrefix(rest, []byte("<!")):
			k := bytes.IndexByte(rest, '>')
			if k < 0 {
				return -1
			}
			i += k + 1
		default:
			return i
		}
	}
}

// index collects the page's cells and reports duplicate ids.
func (p *Page) index() []issue.Issue {
	p.Cells = nil
	p.byID = map[string]*Cell{}
	p.children = map[string][]*Cell{}
	if p.model == nil {
		return nil
	}
	root := p.model.Find("root")
	if root == nil {
		return nil
	}
	var warns []issue.Issue
	for _, el := range root.Children {
		node := el
		if el.Tag != "mxCell" {
			if node = el.Find("mxCell"); node == nil {
				continue
			}
		}
		c := &Cell{ID: el.Attr("id"), Elem: el, Node: node, Page: p}
		p.Cells = append(p.Cells, c)
		if c.ID == "" {
			continue
		}
		if _, dup := p.byID[c.ID]; dup {
			warns = append(warns, issue.New(issue.Warning, "input.duplicate_id", p.Index, []string{c.ID},
				"page %d has two cells with id %q; the second is left alone", p.Index+1, c.ID))
			continue
		}
		p.byID[c.ID] = c
	}
	for _, c := range p.Cells {
		if par := c.Parent(); par != "" {
			p.children[par] = append(p.children[par], c)
		}
	}
	return warns
}

// Bytes writes the document. Compressed pages whose content did not change are
// written with their original text, so an untouched page diffs as nothing.
func (d *Document) Bytes() []byte {
	for _, p := range d.Pages {
		if !p.Compressed || p.model == nil {
			continue
		}
		xml := p.model.String()
		if xml == p.origXML {
			p.diagram.Text = p.origText
		} else {
			p.diagram.Text = compressPage(xml)
		}
	}
	var b bytes.Buffer
	b.WriteString(d.prefix)
	b.WriteString(d.root.String())
	b.WriteString(d.suffix)
	return b.Bytes()
}

// Cell returns the cell with the id, or nil.
func (p *Page) Cell(id string) *Cell { return p.byID[id] }

// Children returns the cells whose parent is id, in document order.
func (p *Page) Children(id string) []*Cell { return p.children[id] }

// Empty reports whether the page has no content.
func (p *Page) Empty() bool { return p.model == nil }

// Model returns the mxGraphModel element, or nil for an empty page.
func (p *Page) Model() *xmltree.Element { return p.model }

// IsRoot reports whether c is the root cell, the one with no parent.
func (c *Cell) IsRoot() bool { return c.Parent() == "" && !c.IsVertex() && !c.IsEdge() }

// IsLayer reports whether c is a layer: a direct child of the root cell.
func (c *Cell) IsLayer() bool {
	if c.IsVertex() || c.IsEdge() {
		return false
	}
	par := c.Page.Cell(c.Parent())
	return par != nil && par.IsRoot()
}

// Parent returns the parent id.
func (c *Cell) Parent() string { return c.Node.Attr("parent") }

// IsVertex reports whether c is a vertex: a shape, text or container.
func (c *Cell) IsVertex() bool { return c.Node.Attr("vertex") == "1" }

// IsEdge reports whether c is a wire.
func (c *Cell) IsEdge() bool { return c.Node.Attr("edge") == "1" }

// Source returns the id of the cell the wire starts at, empty when it is free.
func (c *Cell) Source() string { return c.Node.Attr("source") }

// Target returns the id of the cell the wire ends at, empty when it is free.
func (c *Cell) Target() string { return c.Node.Attr("target") }

// SetTerminal attaches the source (source true) or target end of a wire.
func (c *Cell) SetTerminal(source bool, id string) {
	if source {
		c.Node.Set("source", id)
	} else {
		c.Node.Set("target", id)
	}
}

// Style returns the parsed style.
func (c *Cell) Style() Style { return ParseStyle(c.Node.Attr("style")) }

// SetStyle writes the style back.
func (c *Cell) SetStyle(st Style) { c.Node.Set("style", st.String()) }

// Label returns the raw label: the label attribute of a wrapper, otherwise the
// value attribute. It may hold HTML when the style has html=1.
func (c *Cell) Label() string {
	if c.Elem != c.Node {
		return c.Elem.Attr("label")
	}
	return c.Node.Attr("value")
}

// geometry returns the mxGeometry element, creating it when create is set.
func (c *Cell) geometry(create bool) *xmltree.Element {
	for _, g := range c.Node.FindAll("mxGeometry") {
		if as, _ := g.Get("as"); as == "geometry" || as == "" {
			return g
		}
	}
	if !create {
		return nil
	}
	return c.Node.Add("mxGeometry", "as", "geometry")
}

// RelativeGeometry reports whether the geometry is relative to the parent, as
// for a label placed on a wire.
func (c *Cell) RelativeGeometry() bool {
	g := c.geometry(false)
	return g != nil && g.Attr("relative") == "1"
}
