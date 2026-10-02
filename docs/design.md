# diakempt design

Status: design agreed, nothing implemented yet.

This document is for the person who builds diakempt v1. It records what the tool
does, the rules it follows, and the order in which v1 is built. Decisions that were
deliberately postponed are listed at the end, so they are not mistaken for gaps.

## What is diakempt?

diakempt reads draw.io files that people already have, finds the diagrams in them,
works out what kind of diagram each one is, and makes the layout tidy: wires that do
not cut through shapes, labels that do not overlap, shapes that are aligned and
evenly spaced, arrows that are really attached to what they point at.

It works on any kind of diagram. When it recognizes a known kind (flowchart,
swimlane activity diagram, ...) it applies an optimizer made for that kind. When it
does not, it applies a general tidy-up that respects the user's existing
arrangement.

diakempt changes geometry, not content. Colors, fonts, text, icons, custom
properties, decorations and extra pages survive untouched. The only content-level
change it makes by default is attaching arrows whose ends were meant to be attached
but were dropped slightly off target.

## Pipeline

```
file.drawio
  --> read        every page, compressed or not: cell tree, styles, absolute geometry
  --> snap        attach dangling wire ends (all levels)
  --> interpret   read-only view: logical nodes, wire labels kept as text boxes (all levels)
  --> normalize   rewrite structure into a canonical form (aggressive only)
  --> segment     split each page into independent diagrams plus decoration
  --> detect      classify each diagram, with a confidence
  --> optimize    per diagram, chosen by kind and level
  --> score       compare quality before and after; step down a level if worse
  --> patch       write the new geometry into the original cells
  --> report      text or JSON
```

### snap: dangling wire ends

Users often release the mouse slightly off a shape, so a wire end floats next to its
target. These ends are attached before anything else, because a broken wire splits
the graph and misleads segmentation and detection.

Rules, for each unattached wire end:

1. The end lies inside a shape: attach to it.
2. The end lies outside but near a shape's outline: attach when the distance is at
   most a threshold relative to the shape's size, for example min(20px, 25% of the
   shorter side). The threshold is a parameter.
3. Two or more candidates are about equally close (the nearest is not at least 1.5
   times closer than the second): do not attach. Report an ambiguity warning naming
   both candidates. A wrong guess is worse than no guess.
4. The end is far from every shape: do not attach. A wire with both ends far from
   everything is decoration (an annotation arrow), not a graph edge.
5. The end falls inside a container (pool, lane, group) rather than on a node in it:
   prefer the nearest leaf node within the threshold; attach to the container only
   when no node qualifies.

Among equal candidates, prefer the one that gives a more plausible graph: no
self-loop, no duplicate of an existing wire.

A snapped end is written to the output file at every level: it is a fix the user
wants, not a layout choice.

### interpret: read-only view

Detection must see what a person sees, not the raw cell list. This step builds a
read-only view used by segment, detect and optimize, at every level, and writes
nothing to the file:

- shapes stacked on top of each other (a rectangle with a text box over it) form one
  logical node;
- a text box sitting on or next to a wire is that wire's label;
- groups made only for dragging convenience are seen through.

Snap runs on the logical view, so an end dropped on a text box that covers a
rectangle attaches to the logical node.

### normalize: canonical structure (aggressive only)

At the aggressive level only, diakempt may rewrite structure so that more diagrams
fit a known kind and the layout engine can handle them. Steps, in order:

1. Geometry: absolute coordinates, rotated shapes treated as unrotated.
2. Logical nodes: merge stacked shapes into one node; dissolve convenience groups;
   a group surrounding a titled region becomes a container.
3. Wires connected to wires, and chains of wires through empty waypoint shapes,
   become single wires to the real target. This runs after snap, since an end left
   floating next to another wire is the same situation.
4. Text boxes next to wires become real wire labels.
5. Deeply nested containers are flattened to pool and lane where possible.

Every normalization is listed in the report.

### segment: diagrams on a page

- A diagram is a connected component of the graph of logical nodes and wires,
  joined with container membership: everything in one pool belongs to one diagram.
- A note or free text close to a node, with no wire, is attached to that node.
- Everything else (page titles, legends, images, lone shapes, dangling wires) is
  decoration. Decoration is not laid out; it moves with its nearest diagram so it is
  not covered.

Each diagram gets a stable identifier made of the page, its index on the page, and a
readable name taken from its container title or first node label, for example
Page-1 #2 "Login". The identifier must stay the same between runs on the same file.

### detect: what kind of diagram

Detection is rule-based scoring, not machine learning: deterministic and
explainable. Signals:

- styles: shape=umlLifeline, swimlane, rhombus, ellipse, cylinder, BPMN and UML
  library shapes;
- graph structure: clear sources and sinks, most wires going one way, rhombi with
  two or more outgoing wires;
- geometry: parallel vertical lines joined by horizontal wires (a hand-drawn
  sequence diagram), containers arranged as bands (lanes).

Misclassification costs are asymmetric. Calling an architecture diagram a flowchart
means a full relayout that destroys the user's arrangement; missing a real flowchart
only means it gets the general tidy-up. So a kind that leads to full relayout needs a
high score and a clear margin over the runner-up. Otherwise the diagram is unknown.

The detection catalogue is separate from the optimizer catalogue. A kind can be
recognized before it has its own optimizer; it then gets the general tidy-up and the
report says so.

v1 catalogue:

| Kind | Detected | Own optimizer |
|---|---|---|
| flowchart / activity without lanes | yes | yes, the flowcast engine |
| swimlane activity | yes | yes, the flowcast engine |
| sequence | yes | no, general tidy-up |
| anything else | unknown | general tidy-up |

Class, ER, state, component, BPMN, mind map and network diagrams are not detected in
v1; they are unknown.

The user can force the kind for the whole file with a flag, for example
--type swimlane.

### optimize: levels

The level sets how far diakempt may change the user's layout.

| Level | Operations |
|---|---|
| safe | snap wire ends, separate overlapping nodes, separate overlapping labels, reroute broken wires (through a node or on top of another wire), grow containers to fit their children |
| normal (default) | safe, plus: align nearly aligned nodes, resize nodes to fit their text, even out spacing, give nodes of the same style the same size, remove excess empty space, snap to a grid |
| aggressive | normalize, then free placement: move, reorder, change the overall direction, few constraints |

Each operation can be switched on or off on its own, overriding the level, for
example --level normal --no-resize.

Numeric parameters (alignment threshold, minimum spacing, snap threshold, ...) are
declared once, with a default and a valid range. The CLI builds its flags from that
declaration, and any later front end does the same.

Invariants per level:

- All levels keep: containment (a node stays in its container), connectivity (no
  wire added or removed, no source or target changed, except snapped ends),
  styles, text, wire style (straight, curved or orthogonal), decoration.
  At aggressive, normalize may change structure as listed above.
- safe and normal also keep relative order: if A is left of B, or above B, it stays
  so. This is what separates tidying from relayout.
- aggressive may break relative order.

A broken straight wire is fixed by adding waypoints around the obstacle, never by
switching it to orthogonal.

Kinds with their own optimizer (flowchart, swimlane) are fully laid out by the
engine at every level, because the engine understands what a correct layout is for
them. Unknown and sequence diagrams use the general operations above.

At aggressive, an unknown diagram is laid out by the flowcast engine in flowchart
mode: every node is a task, wire direction follows the arrows, cycles become back
edges. Two limits apply:

- nested containers or groups that are not lane-shaped: the engine cannot place
  them, so the diagram steps down to normal and the report says why;
- wires without arrowheads (network diagrams, mind maps): the direction is inferred
  from the current positions, top to bottom or left to right, and the report says it
  was inferred.

### score: never make it worse

After optimizing a diagram, diakempt computes a quality score from:

- heavy defects: wires through nodes, overlapping nodes;
- medium defects: overlapping labels;
- light defects: wire crossings;
- with a low weight: bounding box area and total wire length, so a much more compact
  result at aggressive is not judged only by its crossings.

If the score after is worse than before, diakempt retries one level lower
(aggressive --> normal --> safe). If safe is still worse, it keeps the original
geometry of that diagram. Snapped wire ends are kept in every case. Each diagram
steps down on its own; other diagrams in the file are unaffected.

--force keeps the result even when it is worse.

### patch: writing geometry back

The original file is the base. Patch replaces only positions, sizes, waypoints,
wire anchors and label positions, plus snapped wire ends, plus normalizations at
aggressive. Coordinates are written relative to each cell's parent. Each page keeps
the compression it had in the input, so diffs in version control stay readable.

## Determinism

The tool aims to be deterministic for people: running it again gives a result that
looks the same. Byte-level determinism is not promised, so that advanced optimizers
remain possible. To stay as close as possible:

- randomized algorithms use a fixed seed, changeable with --seed;
- optimizers stop on an iteration budget, never on wall-clock time, so the result
  does not depend on the speed of the machine.

Where results can still vary, tests compare metrics within a tolerance instead of
bytes.

## Input formats

v1 accepts:

- .drawio with uncompressed pages;
- .drawio with compressed pages (deflate and base64 inside the diagram element);
- .xml with the same content.

The format is detected from the content, not the extension. A file that is not a
draw.io document is rejected with a clear error.

.drawio.svg and .drawio.png (diagrams embedded in images) are rejected in v1 with
"embedded diagrams are not supported yet, export to .drawio first". .vsdx is not
supported; users open it in draw.io and save as .drawio.

## Output files

- Default: a new file next to the input, with the suffix .tidy before the original
  extension: diagram.drawio --> diagram.tidy.drawio, diagram.xml --> diagram.tidy.xml.
- Suffixes do not stack: running on diagram.tidy.drawio writes diagram.tidy.drawio.
- An existing output file is overwritten, and the report says so.
- -o path chooses the output path; -o - writes to stdout.
- --in-place overwrites the input after copying it to diagram.drawio.bak. If that
  exists, the copy is numbered: .bak.1, .bak.2, ... An existing backup is never
  overwritten.
- --dry-run writes no file and prints the report of what would change.
- Several input files may be given in one command. Each is processed on its own,
  with its own report.

## Report

Per file:

- overview: number of pages, diagrams found, decoration elements;
- per diagram:
  - its identifier (see segment);
  - the detected kind and confidence; with --verbose, the signals that matched;
  - the level applied; if it stepped down, the reason;
  - the operations performed, as counts, for example "snapped 3 wire ends,
    separated 2 overlaps, rerouted 5 wires"; with --verbose, every operation with
    its cell ids, and for each snapped end: the wire, which end, the target, the
    distance;
  - quality metrics before and after: wires through nodes, wire crossings,
    overlapping nodes, overlapping labels;
- warnings: ambiguous wire ends, parts that could not be mapped, diagrams that
  stepped down.

Even without --verbose the report has a summary line for snapping, for example
"snapped 7 wire ends, skipped 2 ambiguous", and always shows ambiguity warnings.

Form:

- human-readable text on stderr by default, keeping stdout free for -o -;
- --json prints a machine-readable report;
- exit codes: 0 success, 1 a file could not be read or written, 2 bad flags;
  warnings exit 0 unless --strict, which exits non-zero on any warning or on a
  metric that got worse.

All output, code comments and documentation are in English.

## Interfaces

- CLI: diakempt [flags] file.drawio ...
- Go library. The core takes bytes and returns bytes plus a report. It does not read
  or write files, call external processes or print. The CLI owns files, flags and
  exit codes. This keeps the core ready for a web service later without changes.
- An agent skill that teaches AI agents how to run diakempt. It references
  docs/features.md and contains a playbook for common situations: ambiguous wire
  ends, a diagram that stepped down, a result rolled back because it was worse, an
  unknown kind, an unsupported embedded format, warnings under --strict.

docs/features.md lists every current capability: commands, flags, levels, detected
kinds, accepted formats, report fields. Two mechanisms keep it true:

- CLAUDE.md states that any feature change must check and update docs/features.md;
- a static test reads the flags, levels and kinds from the code and fails when one
  is missing from docs/features.md.

## Relationship to flowcast

The layout engine is copied from flowcast (github.com/luytbq/flowcast), not
imported, and diakempt changes it freely. The first ADR records the flowcast commit
the copy was taken from, so fixes made later in flowcast can be traced and carried
over by hand.

Changes planned for the copied engine:

- a typed input (nodes, wires, lanes) instead of Flow Table rows;
- a fixed size per element, for icons, images and shapes the user sized on purpose;
- a font scale per element, for text whose font size is not 12.

Text measurement keeps the Verdana width table: HTML values are stripped to plain
lines for measuring, the original value is kept when patching, sizes are scaled by
fontSize / 12, and patched cells get whiteSpace=wrap so draw.io wraps inside the
computed box. Verdana is wider than most fonts, so boxes come out slightly large
rather than too small.

Right after the copy, a file generated by flowcast, tidied by diakempt, must have the
same geometry as building it from its table. This check holds at the copy commit and
is expected to drift as the engine changes; it is a starting test, not a lasting
invariant.

## Testing

Invariants, on every corpus file at every level:

- determinism as described above;
- fixed point: running on a .tidy output changes nothing;
- preservation: everything except geometry is identical between input and output
  (styles, text, UserObject properties, cell count, connections), apart from
  snapped ends and, at aggressive, the reported normalizations;
- relative order at safe and normal;
- never worse: the score after is not worse than before;
- round trip: reading and writing without edits yields an equivalent file.

Golden files: for each corpus file, the .tidy output and the JSON report. Changing a
golden requires reviewing the image diff. Rendering through the drawio CLI is
optional, as in flowcast: export PNGs before and after, and compare the wires draw.io
actually draws with the computed coordinates.

### Corpus

No real user files go into the repository. The corpus is synthetic, in three layers:

1. Minimal hand-written cases, one situation each: an end inside a shape, an end
   near two shapes, a text box over a rectangle, a wire to a wire, a compressed page,
   several pages, several diagrams on a page, nested groups, a rotated shape.
2. A messifier with ground truth. It takes clean diagrams and damages them under a
   seed: jitter positions, detach wire ends by a few pixels, overlap nodes, resize
   against the text, add random groups, compress pages. Since the clean original is
   known, it measures snap accuracy, wrong snaps, detection accuracy and layout
   quality against the clean version. This turns the heuristics into numbers.
   Clean sources: flowchart and swimlane from the flowcast conformance suite;
   sequence, architecture, network and mind map diagrams built by script.
3. Hand-drawn style: files built to mimic real habits, with shapes from several
   draw.io libraries, HTML rich text, varied fonts, page titles, legends, images and
   free notes. This layer tests interpret and decoration.

The messifier is a tool in the repository. Layer 2 is regenerated from seeds and not
committed; layers 1 and 3 and the goldens are.

A git-ignored local folder lets the user run the invariants on real files without
committing them.

## Build order for v1

Each step ends with a tool that runs.

1. Repository skeleton: go.mod (Go 1.25), CLAUDE.md (English only, the
   features.md rule), CONTEXT.md (glossary), docs/features.md, and the first ADRs:
   engine copied from flowcast at a named commit; patch geometry instead of
   regenerating; the three-level model; never make it worse.
2. doc package: read and write every page, compressed or not, with the round-trip
   invariant and the basic layer 1 cases.
3. Minimal CLI: .tidy output path, --in-place with numbered .bak, several files,
   --dry-run, text and JSON reports. The tool runs end to end without changing
   anything yet.
4. interpret and snap, with the first messifier measuring snap accuracy. The first
   feature users find useful.
5. The general safe level, the quality score, stepping down when worse.
6. segment and detect (flowchart, swimlane, sequence), with detection accuracy
   measured by the messifier.
7. Copy the flowcast engine, switch it to typed input, add fixed sizes and font
   scale. Full relayout for flowchart and swimlane.
8. The normal level.
9. The aggressive level: normalize and general layout through the engine.
10. The agent skill, and a final review of docs/features.md.

## Later phases

Postponed on purpose:

- forcing the kind per diagram by identifier, for example
  --type "Page-1 #2"=sequence, for pages holding diagrams of different kinds;
- changing wire style (for example straight to orthogonal) as an optimization;
- a configuration file in the project folder, for example .diakempt.yaml; v1 uses
  flags only;
- .drawio.svg and .drawio.png input and output, which needs the drawio CLI to
  regenerate the image part;
- a web service on top of the core;
- an own optimizer for sequence diagrams (lifelines, message order, activation
  boxes);
- detection of more kinds: class, ER, state, component, BPMN, mind map, network;
- a general layout engine that handles nested containers and undirected graphs
  (force-directed), replacing the flowchart-mode fallback at aggressive;
- the future of flowcast relative to diakempt, including whether table, mermaid and
  PlantUML input move here.
