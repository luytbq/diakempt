package layout

import (
	"github.com/luytbq/diakempt/engine/unistr"
	"math"
	"strings"

	"github.com/luytbq/diakempt/engine/num"
)

// gzKey is one half of a gutter: the part next to the column on the left ('l')
// or on the right ('r'). Space reserved for labels and for hugging dbs is added
// to exactly that half.
type gzKey struct {
	lane, gi int
	side     byte
}

type hug struct {
	u    *Item
	side byte
}

// Geometry of one computation. The geometry phase runs two passes, so
// everything here is rebuilt from scratch on each pass.
type geom struct {
	lzG, azG map[gzKey]float64
	lzC      map[int]float64
	hugs     map[string]hug
	gutX     map[[2]int]float64
	colX     map[[2]int][2]float64 // left x and width of a column
	chanY    map[int]float64
	rowY     map[int][2]float64 // top y and height of a row
}

// Origin is the margin from the page edge to the pool.
var Origin = [2]float64{40, 40}

// labelReservations reserves space for edge labels: in the gutter on the exit
// port side when the edge exits sideways, in the channel just below the source
// when the edge exits from the bottom.
//
// spans is the length of the first horizontal part of B and C edges, measured
// after the first pass. The first pass has none yet, so it passes nil.
func (l *Layout) labelReservations(spans map[string]float64) (map[gzKey]float64, map[int]float64) {
	lzG := map[gzKey]float64{}
	lzC := map[int]float64{}
	for _, e := range l.Edges {
		if len(e.Lines) == 0 {
			continue
		}
		u := l.items[e.Src]
		if e.ExitSide == 'B' {
			lzC[u.Row+1] = fmax(lzC[u.Row+1], e.LH+8)
			continue
		}
		key := gzKey{u.Lane, l.gutter(u.Lane, u.Col, e.ExitSide), 'r'}
		if e.ExitSide == 'R' {
			key.side = 'l'
		}
		crosses := l.items[e.Dst].Lane != u.Lane
		switch {
		case e.Case == 'D' || (crosses && spans != nil):
			lzG[key] = fmax(lzG[key], e.LW+8)
		case spans != nil:
			if deficit := e.LW + 16 - spans[e.ID]; deficit > 0 {
				lzG[key] = fmax(lzG[key], deficit)
			}
		}
	}
	return lzG, lzC
}

// hugPlan picks which dbs and texts are placed hugging their node, and on which
// side.
//
// An element hugs only when that side has exactly one element in the adjacent
// cell and no wire exits or enters that side: a horizontal wire segment next to
// the node would cross the intended spot. On a busy side the element is
// centered in the adjacent cell like every other element.
func (l *Layout) hugPlan() map[string]hug {
	hugs := map[string]hug{}
	for aid, atts := range l.Attachments {
		u := l.items[aid]
		for _, side := range []byte{'L', 'R'} {
			var near []*Item
			for _, a := range atts {
				if (a.Col > u.Col) == (side == 'R') && abs(a.Col-u.Col) == 1 {
					near = append(near, a)
				}
			}
			if len(near) != 1 || l.sideUsed(u, side) {
				continue
			}
			hugs[near[0].ID] = hug{u, side}
		}
	}
	return hugs
}

// computeGeometry turns the grid of lanes, columns, rows and track counts into
// pixel coordinates.
//
// The order of additions is part of the result, because floating-point addition
// is not associative: the x of the next column is accumulated separately from
// the width of each gutter and each column, so it may differ in the last bit
// from the lane's x plus the total width. Changing the order of additions
// changes the output file.
func (l *Layout) computeGeometry(lzG map[gzKey]float64, lzC map[int]float64) {
	cfg := l.Cfg
	g := &geom{
		lzG: lzG, lzC: lzC, azG: map[gzKey]float64{}, hugs: l.hugPlan(),
		gutX: map[[2]int]float64{}, colX: map[[2]int][2]float64{},
		chanY: map[int]float64{}, rowY: map[int][2]float64{},
	}
	colW := map[[2]int]float64{}
	rowH := map[int]float64{}
	for _, it := range l.ItemOrder {
		if _, hugged := g.hugs[it.ID]; !hugged {
			colW[[2]int{it.Lane, it.Col}] = fmax(colW[[2]int{it.Lane, it.Col}], it.W)
		}
		rowH[it.Row] = fmax(rowH[it.Row], it.H)
	}
	for aid, h := range g.hugs {
		key := gzKey{h.u.Lane, l.gutter(h.u.Lane, h.u.Col, h.side), 'r'}
		if h.side == 'R' {
			key.side = 'l'
		}
		g.azG[key] = fmax(g.azG[key], float64(cfg.AttachGap)+l.items[aid].W)
	}

	l.LaneX, l.LaneW, l.LaneMinW = nil, nil, nil
	x := 0.0
	for lane, lrow := range l.Lanes {
		cols := l.Cols[lane]
		widths := make([]float64, len(cols)+1)
		for gi := range widths {
			n := l.NTracks[Res{Kind: 'G', A: lane, B: gi}]
			inner := 0.0
			if n > 0 {
				inner = float64(2*cfg.GutterMargin + (n-1)*cfg.TrackGap)
			}
			widths[gi] = g.azG[gzKey{lane, gi, 'l'}] + g.azG[gzKey{lane, gi, 'r'}] +
				lzG[gzKey{lane, gi, 'l'}] + lzG[gzKey{lane, gi, 'r'}] + fmax(float64(cfg.MinGutter), inner)
		}
		total := 0.0
		for _, w := range widths {
			total += w
		}
		cw := 0.0
		for _, c := range cols {
			cw += colW[[2]int{lane, c}]
		}
		total += cw
		// The lane name joins its lines with newline characters before being
		// measured, so each line break adds the width of one .notdef glyph.
		head := l.tm.W(unistr.Strip(strings.Join(lrow.Lines, "\n"))) + 30
		if need := fmax(float64(cfg.MinLaneW), head) - total; need > 0 {
			widths[0] += need / 2
			widths[len(widths)-1] += need / 2
			total += need
		}
		l.LaneX = append(l.LaneX, x)
		l.LaneW = append(l.LaneW, total)
		l.LaneMinW = append(l.LaneMinW, fmax(float64(cfg.MinLaneW), head))
		cx := x
		for gi := range widths {
			g.gutX[[2]int{lane, gi}] = cx
			cx += widths[gi]
			if gi < len(cols) {
				w := colW[[2]int{lane, cols[gi]}]
				g.colX[[2]int{lane, cols[gi]}] = [2]float64{cx, w}
				cx += w
			}
		}
		x += total
	}
	l.PoolW = x

	y := float64(cfg.PoolHeader + cfg.LaneHeader)
	for k := 0; k <= l.NRows; k++ {
		n := l.NTracks[Res{Kind: 'C', A: k}]
		inner := 0.0
		if n > 0 {
			inner = float64(2*cfg.ChannelMargin + (n-1)*cfg.TrackGap)
		}
		h := fmax(float64(cfg.MinChannel), lzC[k]+inner)
		g.chanY[k] = y
		y += h
		if k < l.NRows {
			g.rowY[k] = [2]float64{y, rowH[k]}
			y += rowH[k]
		}
	}
	l.PoolH = y
	l.g = g

	for _, it := range l.ItemOrder {
		c, r := g.colX[[2]int{it.Lane, it.Col}], g.rowY[it.Row]
		it.X = c[0] + (c[1]-it.W)/2
		it.Y = r[0] + (r[1]-it.H)/2
	}
	for aid, h := range g.hugs {
		a := l.items[aid]
		if h.side == 'R' {
			a.X = h.u.X + h.u.W + float64(cfg.AttachGap)
		} else {
			a.X = h.u.X - float64(cfg.AttachGap) - a.W
		}
	}
}

func (l *Layout) trackX(s *Seg) float64 {
	g, cfg := l.g, l.Cfg
	return g.gutX[[2]int{s.Res.A, s.Res.B}] + g.azG[gzKey{s.Res.A, s.Res.B, 'l'}] +
		g.lzG[gzKey{s.Res.A, s.Res.B, 'l'}] + float64(cfg.GutterMargin) + float64(s.Track*cfg.TrackGap)
}

func (l *Layout) trackY(s *Seg) float64 {
	g, cfg := l.g, l.Cfg
	return g.chanY[s.Res.A] + g.lzC[s.Res.A] + float64(cfg.ChannelMargin) + float64(s.Track*cfg.TrackGap)
}

// resolvePaths builds the polyline of each edge from its sym, then drops
// duplicate points and points lying between two collinear segments.
func (l *Layout) resolvePaths() {
	for _, e := range l.Edges {
		u, v := l.items[e.Src], l.items[e.Dst]
		p0 := [2]float64{u.X + float64(e.ExitFrac[0]*u.W), u.Y + float64(e.ExitFrac[1]*u.H)}
		p1 := [2]float64{v.X + float64(e.EntryFrac[0]*v.W), v.Y + float64(e.EntryFrac[1]*v.H)}
		rx := func(r Ref) float64 {
			switch r.Kind {
			case RefCol:
				if r.Lane == v.Lane && r.Col == v.Col {
					return v.X + v.W/2
				}
				c := l.g.colX[[2]int{r.Lane, r.Col}]
				return c[0] + c[1]/2
			case RefSeg:
				return l.trackX(r.Seg)
			}
			return p0[0]
		}
		ry := func(r Ref) float64 {
			if r.Kind == RefSeg {
				return l.trackY(r.Seg)
			}
			return p0[1]
		}
		pts := [][2]float64{p0}
		for _, p := range e.Sym {
			pts = append(pts, [2]float64{rx(p.X), ry(p.Y)})
		}
		pts = append(pts, p1)

		clean := [][2]float64{pts[0]}
		for _, p := range pts[1:] {
			last := clean[len(clean)-1]
			if math.Abs(p[0]-last[0]) < 0.01 && math.Abs(p[1]-last[1]) < 0.01 {
				continue
			}
			clean = append(clean, p)
		}
		out := [][2]float64{clean[0]}
		for i := 1; i < len(clean)-1; i++ {
			a, b, c := out[len(out)-1], clean[i], clean[i+1]
			vertical := math.Abs(a[0]-b[0]) < 0.01 && math.Abs(b[0]-c[0]) < 0.01
			horizontal := math.Abs(a[1]-b[1]) < 0.01 && math.Abs(b[1]-c[1]) < 0.01
			if vertical || horizontal {
				continue
			}
			out = append(out, b)
		}
		// Always append the last point, even when clean has only one point left:
		// the path then has two identical points, and every wire still has both
		// a start and an end point.
		out = append(out, clean[len(clean)-1])
		for i := range out {
			out[i] = [2]float64{num.Round(out[i][0], 2), num.Round(out[i][1], 2)}
		}
		e.Pts = out
	}
}

// horizontalSpans measures the length of the first horizontal part of labeled B
// and C edges, where the label will sit, after the first pass.
func (l *Layout) horizontalSpans() map[string]float64 {
	spans := map[string]float64{}
	for _, e := range l.Edges {
		if (e.Case == 'B' || e.Case == 'C') && len(e.Lines) > 0 && len(e.Pts) >= 2 {
			spans[e.ID] = math.Abs(e.Pts[1][0] - e.Pts[0][0])
		}
	}
	return spans
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
