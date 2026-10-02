// Package xmltree reads and writes XML as an element tree that can be written
// back almost exactly as it was read.
//
// Rules that keep a read-write round trip stable:
//
//   - Text is the text before the first child element, Tail the text after an
//     element. Whitespace between elements is kept, so the original indentation
//     survives. Comments and processing instructions inside the tree are dropped.
//   - Tab, newline and CR written literally in attribute values become spaces,
//     as the XML specification requires; written as character references they are
//     kept. encoding/xml cannot tell the two apart after decoding, so literal ones
//     are replaced on the raw bytes first.
//   - Attributes keep their document order. An element with no children and no
//     text is written as "<tag ... />".
//
// Namespaces are not handled: draw.io files do not use them.
package xmltree

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Element is an XML element.
type Element struct {
	Tag      string
	Attrs    [][2]string
	Text     string
	Tail     string
	Children []*Element
}

// New builds an element from attribute key and value pairs.
func New(tag string, kv ...string) *Element {
	e := &Element{Tag: tag}
	for i := 0; i+1 < len(kv); i += 2 {
		e.Attrs = append(e.Attrs, [2]string{kv[i], kv[i+1]})
	}
	return e
}

// Add appends a new child element and returns it.
func (e *Element) Add(tag string, kv ...string) *Element {
	c := New(tag, kv...)
	e.Children = append(e.Children, c)
	return c
}

// Get reads an attribute.
func (e *Element) Get(k string) (string, bool) {
	for _, a := range e.Attrs {
		if a[0] == k {
			return a[1], true
		}
	}
	return "", false
}

// Attr reads an attribute, empty when missing.
func (e *Element) Attr(k string) string {
	v, _ := e.Get(k)
	return v
}

// Set assigns an attribute. A new attribute goes last; an existing one keeps its
// position.
func (e *Element) Set(k, v string) {
	for i := range e.Attrs {
		if e.Attrs[i][0] == k {
			e.Attrs[i][1] = v
			return
		}
	}
	e.Attrs = append(e.Attrs, [2]string{k, v})
}

// Del removes an attribute.
func (e *Element) Del(k string) {
	for i := range e.Attrs {
		if e.Attrs[i][0] == k {
			e.Attrs = append(e.Attrs[:i], e.Attrs[i+1:]...)
			return
		}
	}
}

// Find returns the first direct child with the tag.
func (e *Element) Find(tag string) *Element {
	for _, c := range e.Children {
		if c.Tag == tag {
			return c
		}
	}
	return nil
}

// FindAll returns every direct child with the tag.
func (e *Element) FindAll(tag string) []*Element {
	var out []*Element
	for _, c := range e.Children {
		if c.Tag == tag {
			out = append(out, c)
		}
	}
	return out
}

// Remove removes a direct child.
func (e *Element) Remove(c *Element) {
	for i, x := range e.Children {
		if x == c {
			e.Children = append(e.Children[:i], e.Children[i+1:]...)
			return
		}
	}
}

// Copy makes a deep copy.
func (e *Element) Copy() *Element {
	c := &Element{Tag: e.Tag, Text: e.Text, Tail: e.Tail, Attrs: append([][2]string(nil), e.Attrs...)}
	for _, ch := range e.Children {
		c.Children = append(c.Children, ch.Copy())
	}
	return c
}

// normalizeAttrs replaces tab, newline and CR written literally inside attribute
// values with spaces, skipping comments, CDATA and processing instructions.
func normalizeAttrs(data []byte) []byte {
	var out bytes.Buffer
	n := len(data)
	skip := func(i int, open, close string) (int, bool) {
		if !bytes.HasPrefix(data[i:], []byte(open)) {
			return i, false
		}
		j := bytes.Index(data[i+len(open):], []byte(close))
		if j < 0 {
			out.Write(data[i:])
			return n, true
		}
		end := i + len(open) + j + len(close)
		out.Write(data[i:end])
		return end, true
	}
	for i := 0; i < n; {
		if j, ok := skip(i, "<!--", "-->"); ok {
			i = j
			continue
		}
		if j, ok := skip(i, "<![CDATA[", "]]>"); ok {
			i = j
			continue
		}
		if j, ok := skip(i, "<?", "?>"); ok {
			i = j
			continue
		}
		if data[i] != '<' {
			out.WriteByte(data[i])
			i++
			continue
		}
		var quote byte
		for i < n {
			c := data[i]
			i++
			if quote == 0 {
				out.WriteByte(c)
				if c == '"' || c == '\'' {
					quote = c
				} else if c == '>' {
					break
				}
				continue
			}
			switch c {
			case quote:
				quote = 0
				out.WriteByte(c)
			case '\r':
				// CRLF is one line break, so it becomes one space.
				out.WriteByte(' ')
				if i < n && data[i] == '\n' {
					i++
				}
			case '\n', '\t':
				out.WriteByte(' ')
			default:
				out.WriteByte(c)
			}
		}
	}
	return out.Bytes()
}

// Parse reads an XML document and returns its root element.
func Parse(data []byte) (*Element, error) {
	d := xml.NewDecoder(bytes.NewReader(normalizeAttrs(data)))
	d.Strict = true
	var stack []*Element
	var root *Element
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			e := &Element{Tag: t.Name.Local}
			for _, a := range t.Attr {
				e.Attrs = append(e.Attrs, [2]string{a.Name.Local, a.Value})
			}
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, e)
			} else if root == nil {
				root = e
			} else {
				return nil, fmt.Errorf("more than one root element")
			}
			stack = append(stack, e)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			cur := stack[len(stack)-1]
			if len(cur.Children) == 0 {
				cur.Text += string(t)
			} else {
				cur.Children[len(cur.Children)-1].Tail += string(t)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("no element found")
	}
	return root, nil
}

var (
	attrEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;",
		"\r", "&#13;", "\n", "&#10;", "\t", "&#9;")
	textEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
)

// String writes the element and its subtree, without its own Tail.
func (e *Element) String() string {
	var b strings.Builder
	e.write(&b, false)
	return b.String()
}

func (e *Element) write(b *strings.Builder, tail bool) {
	b.WriteString("<" + e.Tag)
	for _, a := range e.Attrs {
		b.WriteString(" " + a[0] + "=\"" + attrEsc.Replace(a[1]) + "\"")
	}
	if e.Text != "" || len(e.Children) > 0 {
		b.WriteString(">" + textEsc.Replace(e.Text))
		for _, c := range e.Children {
			c.write(b, true)
		}
		b.WriteString("</" + e.Tag + ">")
	} else {
		b.WriteString(" />")
	}
	if tail {
		b.WriteString(textEsc.Replace(e.Tail))
	}
}
