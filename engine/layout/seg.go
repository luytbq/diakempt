package layout

import "fmt"

// Res is a resource that holds wires: a horizontal channel between two rows, or
// a vertical gutter between two columns. Several wire segments share a Res by
// lying on different tracks.
type Res struct {
	Kind byte // 'C' is a channel, 'G' is a gutter
	// For a channel, A is the row the channel lies directly above. For a gutter,
	// A is the lane and B is the gutter index within that lane.
	A, B int
}

func (r Res) String() string {
	if r.Kind == 'C' {
		return fmt.Sprintf("C:%d", r.A)
	}
	return fmt.Sprintf("G:%d:%d", r.A, r.B)
}

// label names the Res for someone reading a warning.
func (r Res) label() string {
	if r.Kind == 'C' {
		return fmt.Sprintf("channel %d", r.A)
	}
	return fmt.Sprintf("gutter %d of lane %d", r.B, r.A)
}

// Stub is where a wire segment connects to a node or to another segment, seen
// from inside the Res: Pos is the position along the Res, Dir is the side it
// connects from, -1 from the low side and 1 from the high side.
type Stub struct{ Pos, Dir int }

// Seg is a wire segment lying in a Res.
type Seg struct {
	Res    Res
	Lo, Hi int
	// Key is the id of the target node when this segment can be shared with
	// other segments to the same target, so they merge into one line. Empty
	// means a segment of its own.
	Key   string
	Track int
	Stubs []Stub
}

// RefKind is the source of a coordinate in Sym.
type RefKind byte

const (
	RefSrc RefKind = 's' // taken from the source node's exit port
	RefCol RefKind = 'c' // taken from the center of a column
	RefSeg RefKind = 'g' // taken from the track of a wire segment
)

type Ref struct {
	Kind      RefKind
	Lane, Col int
	Seg       *Seg
}

// SymPair says where the i-th bend point takes its x and its y from. The
// geometry phase builds real coordinates from this once the pixel positions of
// columns and tracks are known.
type SymPair struct{ X, Y Ref }

type sideKey struct {
	id   string
	side byte
}

func opp(s byte) byte {
	if s == 'R' {
		return 'L'
	}
	return 'R'
}

// sideFrac is the midpoint of each side, as fractions of width and height.
var sideFrac = map[byte][2]float64{
	'R': {1.0, 0.5},
	'L': {0.0, 0.5},
	'B': {0.5, 1.0},
	'T': {0.5, 0.0},
}
