package layout

// Diagram direction: the way the flow runs.
const (
	DirTD = "TD" // top to bottom
	DirBT = "BT" // bottom to top
	DirLR = "LR" // left to right
	DirRL = "RL" // right to left
)

// ValidDirection reports whether dir is a valid direction.
func ValidDirection(dir string) bool {
	switch dir {
	case DirTD, DirBT, DirLR, DirRL:
		return true
	}
	return false
}

// The engine knows only one direction: top to bottom. Other directions are built
// by laying out in a virtual TD space and applying an axis swap at the end. For
// LR and RL, the width and height of every element and label are swapped before
// layout, so that a row of the virtual space has exactly the thickness of a real
// column. Text is still wrapped along the real direction.
//
// As a result place, route, geometry and labels have no code branch per
// direction, and TD diagrams are unaffected when directions are added. The
// self-check runs on the virtual result: axis swap and flip preserve every
// overlap relation and distance.

func transposed(dir string) bool { return dir == DirLR || dir == DirRL }

// setDirection selects the direction. Lay calls it before run.
func (l *Layout) setDirection(dir string) {
	l.Dir = dir
	if !transposed(dir) {
		return
	}
	for _, it := range l.ItemOrder {
		it.W, it.H = it.H, it.W
	}
	for _, e := range l.Edges {
		e.LW, e.LH = e.LH, e.LW
	}
}

// orient is the transform from the virtual space to the real direction.
type orient struct {
	dir string
	// The flow flips within the content area [hdr, end]: the pool and lane
	// headers stay at the start of the band, only the content reverses.
	hdr, end float64
}

func (o orient) pt(p [2]float64) [2]float64 {
	x, y := p[0], p[1]
	switch o.dir {
	case DirBT:
		return [2]float64{x, o.hdr + o.end - y}
	case DirLR:
		return [2]float64{y, x}
	case DirRL:
		return [2]float64{o.hdr + o.end - y, x}
	}
	return p
}

func (o orient) box(b [4]float64) [4]float64 {
	a, c := o.pt([2]float64{b[0], b[1]}), o.pt([2]float64{b[2], b[3]})
	return [4]float64{fmin(a[0], c[0]), fmin(a[1], c[1]), fmax(a[0], c[0]), fmax(a[1], c[1])}
}

// frac transforms an anchor point given as fractions along the sides of a box.
func (o orient) frac(f [2]float64) [2]float64 {
	switch o.dir {
	case DirBT:
		return [2]float64{f[0], 1 - f[1]}
	case DirLR:
		return [2]float64{f[1], f[0]}
	case DirRL:
		return [2]float64{1 - f[1], f[0]}
	}
	return f
}

// vec transforms an offset, such as a label's offset from its anchor on the path.
func (o orient) vec(v [2]float64) [2]float64 {
	switch o.dir {
	case DirBT:
		return [2]float64{v[0], -v[1]}
	case DirLR:
		return [2]float64{v[1], v[0]}
	case DirRL:
		return [2]float64{-v[1], v[0]}
	}
	return v
}

// orientResult maps a result from the virtual space to the real direction. The
// returned result shares no memory with r. For TD it returns r unchanged.
func orientResult(r Result) Result {
	if r.Dir == "" || r.Dir == DirTD {
		return r
	}
	o := orient{dir: r.Dir, hdr: float64(r.PoolHeader + r.LaneHeader), end: r.PoolH}
	out := r
	if transposed(r.Dir) {
		out.PoolW, out.PoolH = r.PoolH, r.PoolW
	}
	out.Items = make([]PlacedItem, len(r.Items))
	for i, it := range r.Items {
		b := o.box([4]float64{it.X, it.Y, it.X + it.W, it.Y + it.H})
		it.X, it.Y, it.W, it.H = b[0], b[1], b[2]-b[0], b[3]-b[1]
		out.Items[i] = it
	}
	out.Edges = make([]PlacedEdge, len(r.Edges))
	for i, e := range r.Edges {
		pts := make([][2]float64, len(e.Pts))
		for k, p := range e.Pts {
			pts[k] = o.pt(p)
		}
		e.Pts = pts
		e.ExitFrac, e.EntryFrac = o.frac(e.ExitFrac), o.frac(e.EntryFrac)
		if e.Label != nil {
			b := o.box(*e.Label)
			e.Label = &b
		}
		e.LabelOff = o.vec(e.LabelOff)
		out.Edges[i] = e
	}
	return out
}
