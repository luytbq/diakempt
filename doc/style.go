package doc

import "strings"

// Style is a draw.io style string split into its parts, in order. A part
// without "=" is a bare name such as "ellipse" or "rhombus": draw.io treats the
// first one as the shape and the rest as named styles from the stylesheet.
// Editing one key never moves the others, so a patched style diffs as one key.
type Style struct {
	parts []stylePart
	// trailing records whether the original string ended with ";".
	trailing bool
}

type stylePart struct {
	key, val string
	bare     bool
}

// ParseStyle splits a style string.
func ParseStyle(s string) Style {
	st := Style{trailing: strings.HasSuffix(s, ";")}
	for _, p := range strings.Split(s, ";") {
		if p == "" {
			continue
		}
		k, v, ok := strings.Cut(p, "=")
		st.parts = append(st.parts, stylePart{key: k, val: v, bare: !ok})
	}
	return st
}

// String joins the style back into draw.io form.
func (st Style) String() string {
	var b strings.Builder
	for i, p := range st.parts {
		if i > 0 {
			b.WriteByte(';')
		}
		b.WriteString(p.key)
		if !p.bare {
			b.WriteByte('=')
			b.WriteString(p.val)
		}
	}
	if st.trailing && len(st.parts) > 0 {
		b.WriteByte(';')
	}
	return b.String()
}

// Get returns the value of the last occurrence of key, as draw.io does.
func (st Style) Get(key string) (string, bool) {
	for i := len(st.parts) - 1; i >= 0; i-- {
		if p := st.parts[i]; !p.bare && p.key == key {
			return p.val, true
		}
	}
	return "", false
}

// Value returns the value of key, or def when it is missing.
func (st Style) Value(key, def string) string {
	if v, ok := st.Get(key); ok {
		return v
	}
	return def
}

// Has reports whether the style carries key=value or the bare name key.
func (st Style) Has(key string) bool {
	for _, p := range st.parts {
		if p.key == key {
			return true
		}
	}
	return false
}

// Names returns the bare names in order.
func (st Style) Names() []string {
	var out []string
	for _, p := range st.parts {
		if p.bare {
			out = append(out, p.key)
		}
	}
	return out
}

// Set assigns key. An existing key keeps its place, a new one goes last.
func (st *Style) Set(key, val string) {
	for i := len(st.parts) - 1; i >= 0; i-- {
		if p := &st.parts[i]; !p.bare && p.key == key {
			p.val = val
			return
		}
	}
	st.parts = append(st.parts, stylePart{key: key, val: val})
	if len(st.parts) == 1 {
		st.trailing = true
	}
}

// Del removes every occurrence of key.
func (st *Style) Del(key string) {
	out := st.parts[:0]
	for _, p := range st.parts {
		if p.bare || p.key != key {
			out = append(out, p)
		}
	}
	st.parts = out
}

// Shape returns the shape the style draws: the shape key, else the first bare
// name, else "rectangle". Edges have no shape.
func (st Style) Shape() string {
	if v, ok := st.Get("shape"); ok {
		return v
	}
	if n := st.Names(); len(n) > 0 {
		return n[0]
	}
	return "rectangle"
}
