# diakempt features

This file lists what diakempt can do today, and nothing that is only planned. AI
agents use it to learn the tool, so every capability change updates it in the same
commit. Planned work is in [design.md](design.md). A test fails when a flag,
level, operation, setting or kind exists in the code but not here.

## Command

    diakempt [flags] file.drawio ...

Flags may come before, between or after file names; "--" ends flags. Each file is
processed on its own; a file that fails does not stop the others.

## Output

| Flag | Effect |
|---|---|
| (none) | write the result next to the input, with .tidy before the extension: diagram.drawio --> diagram.tidy.drawio. A name already ending in .tidy is written over, never stacked. An existing output file is replaced and the report says so. |
| -o path | write to path; one input file only |
| -o - | write the result to stdout |
| --in-place | overwrite the input, after copying it to name.bak (or name.bak.1, .bak.2, ... when taken). A file with no changes is left alone and gets no backup. |
| --dry-run | write no file, print the report only |

Files are written through a temporary file and a rename, so a failed write never
leaves a half-written file.

## Report

| Flag | Effect |
|---|---|
| (none) | human-readable report on stderr |
| --json | JSON report on stdout: {"version", "files": [{"file", "output", "report", "error"}]}. Cannot be combined with -o -. |
| --verbose | also list every change, detection signals and info issues |
| --strict | exit 3 when any file has a warning or a diagram scored worse |

Report fields per file: pages, diagrams, decoration, snap (snapped, ambiguous,
details), normalized, issues (code, severity, page, diagram, cells, message),
changed. Per diagram: id, page, index, name, kind, confidence, signals, forced,
level, applied, step_downs, operations, details, before, after. Metrics:
wires_through_nodes, node_overlaps, label_overlaps, wire_crossings, wire_overlaps,
area, wire_length, score.

Today every field is filled except normalized, which stays empty until the
aggressive level exists.

## Exit codes

- 0: success
- 1: a file could not be read, processed or written
- 2: bad flags
- 3: --strict found a warning or a worse result

## Levels and operations

| Flag | Effect |
|---|---|
| --level safe, normal, aggressive | how far the layout may change; default normal |
| --type flowchart, swimlane, sequence, unknown | treat every diagram as this kind |
| --with-NAME | run operation NAME whatever the level |
| --no-NAME | skip operation NAME |
| --force | keep results that score worse than the original |
| --seed N | seed for randomized optimizers, default 1 |

These flags are accepted and validated. Implemented today: snap, flowlayout and
the safe operations (separate, containers, reroute, labels). The normal and aggressive
operations do not change the output yet, so --level normal and --level
aggressive currently behave like safe.

### snap (all levels)

Attaches wire ends that were dropped next to a shape instead of on it:

- an end inside a shape attaches to it (the smallest one when shapes nest);
- an end outside a shape attaches when within min(--snap-distance, --snap-ratio
  x the shape's shorter side) of its outline;
- when the nearest candidate is not --snap-margin times closer than the next,
  the end stays free and the report warns with snap.ambiguous;
- a text box laid over a shape counts as that shape, and the wire attaches to
  the shape underneath;
- an end near a container's outline, with no shape nearby, attaches to the
  innermost such container; an end in the middle of empty container space stays
  free;
- a wire is never snapped back onto the shape at its other end; a candidate that
  would duplicate an existing wire loses ties;
- ends with no stored position are left alone.

The report counts snapped and ambiguous ends; --verbose lists each snap with its
wire, end, target and distance.

Operations, by the lowest level that runs them:

- safe: snap, flowlayout, separate, containers, reroute, labels
- normal: align, resize, samesize, spacing, compact, grid
- aggressive: normalize, relayout

### flowlayout (all levels)

A diagram detected (or forced) as a flowchart or swimlane is laid out from
scratch by the layout engine copied from flowcast, at every level. The diagram
is written as a Flow Table: shapes become elements (diamonds with two or more
outgoing wires are conditions; ellipses are starts, ends or externals; cylinders
are data; others are tasks), wires become edges, swimlanes become lanes, shapes
with no wire attach to the nearest wired shape. The current drawing supplies
the direction (lanes as columns or rows, else the way most wires go), the lane
order, the main branch (the target most in line with its source) and the back
edges (a depth-first walk in reading order).

The result keeps the diagram's top-left corner, the shapes' styles and text,
and gives every wire right-angle waypoints through fixed ports. Shapes with
HTML labels get whiteSpace=wrap so text wraps inside the computed size.

Diagrams the engine cannot represent fall back to the general operations, with
the reason in the report: a wire with a free end or ending on a container, a
wire looping to its own shape, containers that are not lanes or their pool, a
shape outside every lane, or a table that does not validate. A result that
scores worse than the original also falls back.

### Diagrams and decoration

Each page is split into diagrams: groups of shapes joined by wires or by sitting
in the same top-level container. A free wire end within 4px of another wire or
of a shape joins it, so arrows drawn between dashed lifelines belong to their
sequence diagram. A free text within 40px of a wired shape joins its diagram. Everything else (lone shapes, titles, legends, wires attached to
nothing) is decoration and is never changed. Each diagram is reported with an
identifier such as Page-1 #2 "Login": page, index in reading order, and the top
container's title or the first shape's text.

### separate (safe)

Pushes overlapping shapes in the same container apart until they are --min-gap
apart, along the axis that needs the smaller push. Space is opened by moving
everything at or past the pushed shape on that axis, so no two shapes swap
sides. A shape drawn entirely inside another (a badge on a box) is left alone.

### containers (safe)

A container that a shape sticks out of grows to hold it, with 10px padding
around shapes and none around lanes. Containers whose children fit are left as
drawn. Lanes of a stacked pool (childLayout=stackLayout) are laid edge to edge
again and stretched to a common height (or width) without moving their
contents across the stack.

### reroute (safe)

A wire that crosses a shape, runs through its own end shape, or runs on top of
another wire gets a new route, kept only if the diagram scores better:

- orthogonal wires get right-angle waypoints through fixed ports (exitX/exitY,
  entryX/entryY), chosen over every pair of side ports, avoiding shapes by
  10px, charging bends, crossings and ports another wire already uses;
- straight and curved wires keep their style and get waypoints around the
  shapes in the way.

Passes repeat until no wire changes.

### labels (safe)

A free text that overlaps a shape or another label moves to the nearest clear
spot within 60px. A wire label that overlaps something slides along its wire to
the clear position nearest where it was.

### Score and stepping down

Each diagram is scored before and after: wires through shapes and overlapping
shapes (including a shape sticking out of its container) weigh 100, overlapping
labels 20, wires on top of wires 10, crossings 5, plus small weights for area
and wire length. If the result scores worse than the original, or swaps two
shapes' sides at safe or normal, the diagram is retried one level lower, and
keeps its original geometry if safe fails too. Snapped wire ends are kept in
every case. --force keeps a worse result.

## Settings

Each setting is a flag taking a number in its range.

| Flag | Default | Range | Meaning |
|---|---|---|---|
| --snap-distance | 20 | 0 to 200 | farthest a wire end may be from a shape outline to snap, in pixels |
| --snap-ratio | 0.25 | 0 to 1 | the same limit as a share of the shape's shorter side |
| --snap-margin | 1.5 | 1 to 10 | how many times closer the nearest shape must be than the next one |
| --align-tolerance | 8 | 0 to 50 | largest center offset that align treats as meant to be aligned |
| --min-gap | 20 | 0 to 200 | smallest gap between nodes, in pixels |
| --grid | 10 | 1 to 100 | grid size in pixels |

## Detected kinds

Each diagram is scored as flowchart, swimlane and sequence by rules, never by
learning, so the report can say why (--verbose lists the signals):

- flowchart: three or more shapes, mostly one-way arrows going the same
  direction, decision diamonds or start and end shapes, a clear start and end,
  no lanes;
- swimlane: the same, inside swimlane containers (a pool with lanes, or
  swimlanes side by side as bands);
- sequence: two or more lifelines (the UML lifeline shape, or dashed vertical
  lines without arrowheads) with horizontal messages between them.

Shapes or arrows of other notations (UML classes, ER tables and relations,
state machine start and end states, network and cloud icons, mind map links,
UML associations and inheritance) and containers that are not lanes push the
flow scores down.

A kind is assigned only with high confidence: a score of at least 0.8 and 0.25
ahead of the next kind. Otherwise the diagram is unknown, reported with medium
or low confidence and, at medium, the kind it came closest to. Diagrams too
small to judge (fewer than three shapes or two directed wires) are unknown.

Flowcharts and swimlanes are laid out from scratch (see flowlayout below);
sequence and unknown diagrams get the general operations. --type overrides the
detected kind, so --type flowchart or --type swimlane forces a relayout attempt
and --type unknown prevents one.

## Input formats

- .drawio and .xml files holding an mxfile with one or more pages, each page
  compressed or not, or a bare mxGraphModel. The format is told from the content.
- Every page is kept. An untouched compressed page is written back with its
  original text.
- .drawio.svg and .drawio.png files are rejected with input.embedded.
- Anything else is rejected with input.not_drawio, input.malformed,
  input.no_pages or input.page_decode.

## Other

| Flag | Effect |
|---|---|
| --version | print the version |
| --help | print the flags |
