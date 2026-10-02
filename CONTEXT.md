# diakempt glossary

This document defines the terms used throughout the code, tests and docs. A concept
has exactly one name. Seeing a different name in the code means the code needs
fixing, not that this document needs another synonym.

## Documents

**Document** - a parsed draw.io file: its pages, in order, each with the
compression it had on disk.

**Page** - one diagram element of a draw.io file, holding a tree of cells. A page
may hold several diagrams.

**Cell** - one mxCell of a page, possibly wrapped in an object or UserObject
element. A cell is a vertex, a wire or a container. Cells keep their original
attributes; diakempt edits geometry and leaves the rest alone.

**Style** - the style attribute of a cell: an ordered list of key=value pairs.
Editing one key never reorders the others.

## Reading the picture

**Wire** - a draw.io edge cell: a line or arrow between two cells, or with free
ends.

**Wire end** - the source or target end of a wire. An end is attached when it
names a cell, and dangling when it only has a point.

**Snap** - attaching a dangling wire end to the cell it was meant for, following
the rules in docs/design.md. An end that has two equally close candidates is
ambiguous and is left dangling with a warning.

**Logical node** - what a person sees as one shape, even when draw.io stores it as
several cells, for example a rectangle with a text box placed over it.

**Interpret** - building the read-only view of logical nodes and wire labels that
the later steps work on. It writes nothing to the file.

**Normalize** - rewriting a page's structure into a canonical form: merging
stacked shapes, dissolving convenience groups, joining wires connected to wires,
flattening nested containers. Only the aggressive level normalizes.

## Diagrams

**Diagram** - a connected group of logical nodes and wires on a page, joined with
the containers they sit in. Diagrams are tidied independently.

**Diagram identifier** - page, index on the page and a readable name, for example
Page-1 #2 "Login". It stays the same between runs on the same file.

**Decoration** - anything on a page that is not part of a diagram: titles,
legends, images, lone shapes, wires with both ends free. Decoration is never laid
out; it moves with its nearest diagram.

**Segment** - splitting a page into diagrams and decoration.

**Kind** - what sort of diagram something is: flowchart, swimlane, sequence, or
unknown.

**Detect** - choosing a diagram's kind by rule-based scoring, with a confidence. A
kind that leads to full relayout needs a high score and a clear margin; otherwise
the diagram is unknown.

## Changing the layout

**Level** - how far diakempt may change a layout: safe, normal or aggressive.

**Operation** - one kind of change a level performs, such as separating overlaps
or aligning nodes. Each operation can be switched on or off on its own.

**Relative order** - for every pair of nodes, which is left of the other and which
is above. safe and normal keep it; aggressive may break it.

**Relayout** - placing a diagram from scratch with the layout engine, as opposed to
tidying it from its current positions.

**Score** - a number summarizing a diagram's layout defects: wires through nodes,
overlapping nodes, overlapping labels, wire crossings, and with a low weight, area
and wire length. Lower is better.

**Step down** - retrying a diagram one level lower because its score got worse,
and keeping its original geometry when even safe is worse.

**Patch** - writing new geometry into the original cells, leaving every other
attribute as it was.

## Output

**Tidy file** - the output file, named with .tidy before the original extension.

**Report** - what a run tells the caller: per file, the diagrams found, their
kinds, the level applied, the operations performed, the score before and after,
and warnings. Printed as text or JSON by the CLI; returned as data by the core.

## Testing

**Corpus** - the synthetic files the tests run on. Real user files are never part
of it.

**Messifier** - a tool that damages clean diagrams in controlled, seeded ways, so
that the result of tidying can be compared with a known ground truth.

**Ground truth** - the clean diagram a messified file was made from, including
which wire ends belong to which cells and what kind the diagram is.
