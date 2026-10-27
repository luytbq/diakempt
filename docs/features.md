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
| --strict | exit 3 when any file has a warning or a diagram's kept result is worse than its original under the never-worse rule |

Report fields per file: pages, diagrams, decoration, snap (snapped, ambiguous,
details), normalized, issues (code, severity, page, diagram, cells, message),
changed. Per diagram: id, page, index, name, shapes, wires, kind, confidence, signals, forced,
level, applied, step_downs, operations, details, before, after, worse. Metrics:
wires_through_nodes, node_overlaps, label_overlaps, wire_crossings, wire_overlaps,
area, wire_length, score, defects (the weighted defect part of score).

A diagram whose kept result is worse than its original under the never-worse
rule gets a worse: line naming the defect score before and after, or the area
and wire length before and after when only those grew.

normalized is filled at the aggressive level only, and left out when empty.

## Exit codes

- 0: success
- 1: a file could not be read, processed or written
- 2: bad flags
- 3: --strict found a warning, or the final kept result is worse than the
  original under the never-worse rule (docs/adr/0004). --force keeps such a
  result, and several passes can add up past the tolerance even though no
  single pass exceeded it.

## Levels and operations

| Flag | Effect |
|---|---|
| --level safe, normal, aggressive | how far the layout may change; default normal |
| --type flowchart, swimlane, sequence, class, state, er, usecase, communication, package, deployment, unknown | treat every diagram as this kind |
| --with-NAME | run operation NAME whatever the level |
| --no-NAME | skip operation NAME |
| --force | keep results that score worse than the original |
| --seed N | seed for randomized optimizers, default 1 |

These flags are accepted and validated. Every operation is implemented.

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
- an end outside every shape that lies on a hand-drawn lifeline, within 6px
  of a UML lifeline's center line, or within 2px of a frame's outline (see
  seqlayout) stays free: it is a message end, a gate or an operand separator
  put there on purpose;
- frames (alt, loop, opt, ref, sd) are never targets: an end inside a frame
  attaches to the shape it was dropped next to;
- a wire is never snapped back onto the shape at its other end; a candidate that
  would duplicate an existing wire loses ties;
- ends with no stored position are left alone.

The report counts snapped and ambiguous ends; --verbose lists each snap with its
wire, end, target and distance.

Operations, by the lowest level that runs them:

- safe: snap, flowlayout, seqlayout, classlayout, erlayout, commlayout, deploylayout, pkglayout, usecaselayout, separate, containers, reroute, labels
- normal: align, resize, samesize, spacing, compact, grid
- aggressive: normalize, relayout

### flowlayout (all levels)

A diagram detected (or forced) as a flowchart, swimlane or state machine is laid out from
scratch by the layout engine copied from flowcast, at every level. The diagram
is written as a Flow Table: shapes become elements (diamonds with two or more
outgoing wires are conditions; ellipses are starts, ends or externals; cylinders
are data; others are tasks), wires become edges, swimlanes become lanes, shapes
with no wire attach to the nearest wired shape (of their own lane, when the
lane has one). The current drawing supplies
the direction (lanes as columns or rows, else the way most wires go), the lane
order, the main branch (the target most in line with its source) and the back
edges (a depth-first walk in reading order).

The result keeps the diagram's top-left corner, the shapes' styles and text,
and gives every wire right-angle waypoints through fixed ports. Wire labels
get room on one line, since draw.io never wraps them. Shapes are
sized from their text, measured at their own fontSize; shapes with HTML labels
get whiteSpace=wrap so text wraps inside the computed size. Shapes without text, images, stencil
icons (other than the flowchart library), state machine start and end states
and shapes whose label sits outside them keep their drawn size; a label below or above such a shape is reserved
room, and wires stop beyond it (an exitDy or entryDy offset with
exitPerimeter=0 or entryPerimeter=0).

The table is read from positions, so a first layout can read back as a
slightly different table. flowlayout lays out again from its own result until
the table read back stops changing (at most four rounds, else the round with
the fewest defects wins), so tidying the output again changes nothing.

Diagrams the engine cannot represent fall back to the general operations, with
the reason in the report: a wire with a free end or ending on a container, a
wire looping to its own shape, containers that are not lanes or their pool, a
shape outside every lane, or a table that does not validate. A result that
scores worse than the original also falls back.

### seqlayout (all levels)

A sequence diagram drawn with the editor's UML lifeline shape (shape=umlLifeline)
is laid out from scratch:

- lifelines stay in their left-to-right order, in one row with their heads
  aligned, at least 40px apart and wide enough apart for every message label
  between them (a self call's label counts on its right);
- messages keep their time order (top to bottom; ties in document order) and
  get one row each, two for a self call, every message horizontal; the row
  height fits the tallest label;
- a message attached to an activation bar leaves it from the side facing its
  target and enters one on the side facing its source; one attached to a bare
  lifeline meets its center line;
- each activation bar spans its first to its last message, 10px beyond each,
  keeping its offset from the lifeline's center; a bar with no message follows
  the rows;
- a UML frame (alt, loop, opt; shape=umlFrame) is redrawn around the messages
  it enclosed, 60px beyond the outermost lifeline involved; frames are overlays
  that are never pushed aside and never count as overlaps;
- notes follow the nearest lifeline sideways and the rows downward;
- lifelines extend 40px below the last message.

A frame is the editor's UML frame (shape=umlFrame) or an unfilled rectangle
(fillColor=none, text at the top) whose text starts with alt, opt, loop,
break, par, critical, neg, assert, ignore, consider, seq, strict, ref or sd.
Messages drawn with free ends work as well as attached ones:

- a free end within 6px of a lifeline's center line is on that lifeline, and
  moves with it;
- a free end on a frame's left or right edge, or a message attached to a
  small shape off the lifelines (a found message's dot), is a gate: it stays
  on the frame's new edge, or keeps its offset from the nearest lifeline, and
  the dot follows it;
- a line without arrowheads running across a frame is an operand separator:
  it gets a row of its own and spans the frame's new width;
- a frame enclosing no message (an interaction use such as "ref Login") gets
  rows of its own height, across the lifelines it crossed;
- a frame drawn around the lifeline heads (an sd frame) is redrawn around the
  lifelines with the margins it had;
- a frame opening or closing at a message adds 20px or 10px of room there,
  and nested frames keep 10px apart.

The diagram keeps its top-left corner. One UML lifeline, a message that
touches no lifeline and is no separator, or a container other than a lifeline
falls back to the general operations, as does a result that scores worse.

A sequence diagram drawn by hand, with no UML lifeline, is laid out the same
way from its own parts:

- a lifeline is a dashed line without arrowheads or waypoints running at least
  60px straight down, with free ends or its top attached to a shape; its head
  is that shape, or the closest shape spanning the line's x whose bottom is at
  most 40px above the line's top;
- a message is any other wire whose free ends lie within 6px of a lifeline's
  x; a self call needs waypoints;
- heads go in one row with their tops aligned, spaced as UML lifelines are;
  each line hangs from the bottom center of its head to 40px below the last
  message; messages get one row each (two for a self call), drawn as
  horizontal lines between the lines; notes and other shapes follow as with
  UML lifelines.

Only free wire ends move. A lifeline that hangs from no shape, two lifelines
under one shape, a message attached to a shape, a message without waypoints
starting and ending on one lifeline, or a container falls back to the general
operations.

### classlayout (all levels)

A diagram detected (or forced) as a class diagram is laid out from scratch:

- ranks from the top: the target of a generalization or realization (hollow
  triangle) above its source, the diamond end of a composition or aggregation
  above the other end, and the client of a dependency (a dashed open arrow)
  above its supplier, each where it does not contradict the ones before; a
  class with none sits in the rank of its median neighbor;
- when no relation is a generalization, realization, composition,
  aggregation or dependency: boundary classes on top, controls under them and entities at
  the bottom, when every class carries one of those stereotypes («boundary»,
  «control», «entity») and they are not all the same; otherwise the rows the
  classes were drawn in;
- within a rank, classes ordered to reduce crossings (sweeps over the average
  position of neighbors), each class centered under the classes it hangs from;
  neighbors joined by a wire get room for its labels (role names and
  multiplicities near the ends);
- every wire between adjacent ranks runs through the gap between them:
  generalizations as trunks, from each child's top up to a level shared by all
  children of one parent, across, and into the parent's bottom; every other
  wire on a level of its own, from the upper class's bottom to the lower
  class's top; levels are ordered to avoid crossings, ports are spread along
  each side in the order of the classes they lead to, and a gap grows to fit
  its levels;
- every other relation routed around the classes.

Classes keep their size, since their rows set it, and the diagram keeps its
top-left corner. Classes inside a container, wires with a free end or ending on
a text fall back to the general operations, as does a result that scores worse.

### erlayout (all levels)

A diagram detected (or forced) as an ER diagram is laid out from scratch, as
the class layout turned on its side:

- columns from the left: the table at the "one" end of a relation (ERone,
  ERmandOne, ERzeroToOne) left of the table at the "many" end (ERmany,
  ERoneToMany, ERzeroToMany), so foreign keys point left; one-to-one and
  many-to-many relations do not decide columns, and a table with neither sits
  in the column of its median neighbor;
- within a column, tables ordered top to bottom to reduce crossings, each
  level with the tables it hangs from as far as its neighbors allow.

draw.io routes relations drawn with edgeStyle=entityRelationEdgeStyle by
itself, out of the left or right side of the table at the row the relation is
attached to, and ignores waypoints; those relations only lose stored
waypoints. Other relations between adjacent columns run through the gap
between them as in classlayout, turned on its side: from the right side of
the left table, on a level of their own, into the left side of the right
table. Relations attached to a row, within a column or across several are
routed around the tables.

Tables keep their size and the diagram keeps its top-left corner. Tables inside
a container, wires with a free end or ending on a text fall back to the general
operations, as does a result that scores worse.

### commlayout (all levels)

A diagram detected (or forced) as a communication diagram is laid out from
scratch, left to right:

- the diagram starts at the actor of the link carrying the first message
  number (or either end of it, or the best connected object without
  numbers);
- each object one column right of the object it is first reached from,
  following links in message order (1, 1.1, 1.1.1, 1.2, 2, ...), so each
  column lists its objects in the order they get their first message, and
  the column is centered on the objects it is reached from;
- the objects of a column (not the actors) take its widest width, and a link
  meets each object at the middle of the side facing the other, so it stays
  clear of the rest of the column;
- each gap between columns fits the widest message label of the links
  crossing it, and objects in a column sit far enough apart that the labels
  of links fanning out to them do not touch;
- a self link loops above its object, with room for its label; a link that
  would still cross an object is routed around it.

A communication diagram's links keep their ends, so text arrows in labels such as "(-->)", which read
from the link's source to its target, stay true. The diagram keeps its
top-left corner. A container, a link with a free end or ending on a text
falls back to the general operations, as does a result that scores worse.

### deploylayout (all levels)

A diagram detected (or forced) as a deployment diagram is laid out like a
communication diagram, in columns breadth first, with these differences:

- it starts at the best connected node (the first in document order on a
  tie); when that node has more than three neighbors, its first half go in
  columns to its right and the rest to its left, each side growing outward;
- a node holding artifacts moves with them, and links are counted between
  top-level nodes;
- nodes keep their size.

### pkglayout (all levels)

A diagram detected (or forced) as a package diagram is laid out from
scratch, inside out:

- the top-level packages keep the rows they were drawn in, so layers stay in
  the author's order; packages standing alone in their row, such as layers,
  take the width of the widest of them;
- inside a package, what it holds goes in rows ranked by the realizations and
  generalizations between them (an interface above what realizes it);
  dependencies do not rank, so packages of one layer that use each other stay
  side by side; without such relations they keep the rows they were drawn in;
- each row is centered in its package and ordered to keep dependencies from
  crossing (sweeps over the average position of what each member, or
  anything inside it, depends on or is depended on by);
- every package shrinks or grows to fit what it holds, with room for its
  title below its tab; shapes holding nothing keep their size;
- dependencies are routed around the shapes afterwards; free texts move with
  the shape they belong to.

The diagram keeps its top-left corner. A shape inside a container that is
not part of the diagram, a dependency with a free end or ending on a text
falls back to the general operations, as does a result that scores worse.

### usecaselayout (all levels)

A diagram detected (or forced) as a use case diagram is laid out from
scratch:

- the use cases in one column inside the system boundary, all as wide as the
  widest, so their leftmost and rightmost points line up;
- each actor in a column on the side of the boundary (or, without one, of the
  use cases) it was drawn on, level with the use cases it is associated with;
- the order of use cases and actors chosen to reduce crossings (sweeps over
  the average height of what each is joined to);
- an association meets its use case at the middle of the side facing the
  actor, so it stays clear of the other use cases;
- two use cases joined by a labeled include or extend get room for the label
  between them; an include or extend between use cases that are not neighbors
  runs beside the column on the right, on a track of its own;
- includes and extends are weighted more than associations when ordering, so
  the use cases they join come near each other;
- the boundary is redrawn around the column and the tracks beside it, with
  room for its title;
- notes stand in a column right of the boundary, before the actors on that
  side, each level with the shape it stood nearest; a note's line to a wire
  follows the wire.

The diagram keeps its top-left corner. A container, a shape that is neither
an actor, a use case, a note nor a text, a second boundary, or a wire with a
free end falls back to the general operations, as does a result that scores
worse.

### Diagrams and decoration

Each page is split into diagrams: groups of shapes joined by wires or by sitting
in the same top-level container. A free wire end within 4px of another wire or
of a shape joins it, so arrows drawn between dashed lifelines belong to their
sequence diagram, and a hand-drawn lifeline joins the shape just above its
top (within 40px) even when its head was dragged off it. A free text or a note (shape=note) within 40px of a wired
shape, or of a container holding one (such as a lifeline with activation bars),
joins its diagram. A free end within 6px of a UML lifeline's center line
joins that lifeline, and a wire ending on another wire (a note's line to the
arrow it explains) joins that wire. A frame joins the diagram whose wires it encloses, or
whose lifelines it is drawn across. An unfilled rectangle with its title at the top
that encloses other shapes without holding them as children (the system
boundary of a use case diagram) is a frame too: it joins the diagram of every
wire it crosses, and never counts as a shape wires run through. Everything else (lone shapes, titles, legends, wires attached to
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

### align (normal)

Siblings whose centers lie within --align-tolerance on an axis line up on the
center of the largest of them. A wire with waypoints between two shapes that
end up level becomes a straight wire when nothing is in the way.

### resize (normal)

A shape whose text clearly overflows it (by more than 6px, measured at its
font size, narrowed for fonts other than Verdana) grows about its center:
diamonds hold text in their middle half, ellipses in seven tenths. Shapes are
never shrunk, and images, icons and shapes with outside labels are never
resized.

### samesize (normal)

Siblings with the same style and similar size (each at least two thirds of the
largest) take the largest size, about their centers.

### spacing (normal)

In each aligned row or column of three or more siblings, gaps that are already
similar (each within half of their median) but not even (more than one grid
step apart) become the median gap.

### compact (normal)

An empty band across the whole diagram wider than four --min-gap closes down
to two --min-gap. Only a container's edges block a band; its empty inside does
not. Other diagrams and decoration on the page block bands too; a band that
ends on one of them, or whose closing would land content on one, stays open,
and the bands after it still close.

### grid (normal)

Each shape's center moves onto the --grid; centers that were equal stay equal.
Lanes of stacked pools are left alone.

The normal operations repeat as a group until a pass moves nothing (at most
four passes), then wires are rerouted and labels moved.

### normalize (aggressive)

Rewrites structure before anything else, and lists every rewrite in the report
(normalized):

- a group with no label is dissolved: its cells move up to the group's parent,
  where they stay drawn, and the group is deleted;
- a text box laid over a shape becomes part of the shape's label, wires
  attached to the text box move to the shape, and the text box is deleted;
- a wire ending on another wire is attached to that wire's own end shape;
- a free text within 12px of a wire that has no label becomes its label, and
  the text box is deleted.

### relayout (aggressive)

An unknown diagram (not a sequence) is laid out from scratch by the engine as
a flowchart, or as a swimlane diagram when its shapes sit in swimlanes, with
the same rules and fallbacks as flowlayout. Wires without an arrowhead (or
with one at each end) run the way the flow goes; when no wire has a direction,
they run away from the best connected shape (the first in document order on a
tie), level by level, so the diagram lays out as a tree from its hub, with
branches in document order. Shapes without text keep their drawn size.
Diagrams in containers that are not lanes, such as grouped architecture tiers,
cannot be relaid out and get the normal operations.

### Passes

One pass over a page can make work for another: moving shapes can bring a
free wire end within snap reach, and a new attachment joins two diagrams. Each
page is processed again until a pass changes nothing (at most three passes;
normalize runs on the first only). The report shows the diagrams of the last
pass; a diagram whose members did not change keeps its metrics from before the
first pass, and the changes of all passes add up.

### Score and stepping down

Each diagram is scored before and after: wires through shapes and overlapping
shapes (including a shape sticking out of its container) weigh 100, overlapping
labels (including a label struck through by a UML lifeline's dashed line) 20, wires on top of wires 10, crossings 5, plus small weights for area
and wire length. A result is worse when its defects score higher, or, with
equal defects, when area or wire length grew by more than a tenth. A worse
result, or one that swaps two shapes' sides at safe or normal, is retried one
level lower, and the diagram keeps its original geometry if safe fails too. A
flowchart or swimlane whose flowlayout fails falls back to safe at most.
Snapped wire ends are kept in every case. --force keeps a worse result.

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

Each diagram is scored as flowchart, swimlane, sequence, class, state, er, usecase, communication, package and deployment by rules, never by
learning, so the report can say why (--verbose lists the signals):

- flowchart: three or more shapes, mostly one-way arrows going the same
  direction, decision diamonds or start and end shapes, a clear start and end
  (a start alone counts half: screens navigating back and forth have no
  end), no lanes;
- swimlane: the same, inside swimlane containers (a pool with lanes, or
  swimlanes side by side as bands); in lanes, the state machine's initial and
  final state shapes count as the activity's start and end shapes, not as
  another notation;
- sequence: two or more lifelines (the UML lifeline shape, or dashed vertical
  lines without arrowheads, free or hanging from a shape) with horizontal
  messages between them, attached or with free ends on the lifelines;
- class: two or more UML classes (the editor's class shape: a swimlane
  stacking its rows), most shapes being classes, with UML relation ends on the
  wires; when UML relations (diamonds, hollow triangles) join them, boxes
  standing for classifiers count as classes too: a rectangle or component with
  a classifier stereotype («interface», «subsystem», «component», «entity»,
  ...), a package's folder, and a plain box holding just a name;
- state: three or more shapes with the editor's initial or final state dots
  (shape=startState, shape=endState), mostly one-way transitions, no lanes,
  lifelines or containers, no shapes of other notations;
- deployment: two or more nodes (shape=cube, or a box with a deployment
  stereotype such as «device», «execution environment», «artifact» or
  «external system»), most shapes being nodes, most wires without arrowheads
  or dashed;
- package: two or more packages (shape=folder), at least one holding other
  shapes, most shapes being packages or classifiers, most wires dashed or
  with UML ends; nested packages push the class score down;
- communication: two or more objects (rectangles whose text has a line such
  as ":Class" or "name:Class", or is underlined) with links without
  arrowheads labeled with message numbers ("1.1: ..."), most shapes being
  objects or actors and most wires such links;
- usecase: one or more actors (shape=umlActor) and use cases (ellipses with
  text and no solid fill) with associations between them, most shapes being
  actors or use cases and most wires joining them;
- er: two or more of the editor's ER tables (shape=table, or the stacked
  shape the ER library shares with UML classes when ER relation ends outnumber
  UML ones), most shapes being tables, with ER relation ends on the wires.

For the flow kinds, shapes or arrows of other notations (UML classes, ER tables and relations,
state machine start and end states, network and cloud icons, mind map links,
UML associations and inheritance) and containers that are not lanes push the
flow scores down.

A kind is assigned only with high confidence: a score of at least 0.8 and 0.25
ahead of the next kind. Otherwise the diagram is unknown, reported with medium
or low confidence and, at medium, the kind it came closest to. Diagrams too
small to judge (fewer than three shapes or two directed wires) are unknown.

Flowcharts, swimlanes and state machines are laid out from scratch (see flowlayout below),
class diagrams by classlayout, ER diagrams by erlayout, sequence diagrams by
seqlayout, use case diagrams by usecaselayout, communication diagrams by
commlayout, package diagrams by pkglayout, deployment diagrams by deploylayout; unknown diagrams get the general operations. --type overrides the
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
