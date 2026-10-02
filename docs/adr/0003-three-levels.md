# ADR-0003: Three levels decide how far a layout may change

- Status: accepted
- Date: 2026-10-02

## Context

On a diagram whose kind is known, a full relayout gives the best result, because
the optimizer knows what a correct layout of that kind looks like. On an unknown
diagram, positions often carry meaning a general algorithm cannot infer: clients
on the left, servers on the right, related parts grouped in one region. A full
relayout there can do more harm than good. Users also differ in how much change
they accept.

## Decision

Three levels, chosen per run, with each operation switchable on its own:

- safe: snap wire ends, separate overlapping nodes and labels, reroute broken
  wires, grow containers to fit their children.
- normal, the default: safe plus aligning, resizing to fit text, even spacing,
  same size for nodes of the same style, removing excess space, snapping to a grid.
- aggressive: normalize the structure, then place freely: move, reorder, change
  the overall direction.

Every level keeps containment, connectivity (apart from snapped ends), styles,
text, wire style and decoration. safe and normal also keep relative order;
aggressive may break it.

Kinds with their own optimizer are fully laid out at every level. Other diagrams
get the operations of the chosen level.

## Consequences

- Relative order is the line between tidying and relayout. A user who wants it
  broken on an unknown diagram must choose aggressive or force a kind.
- Adding a kind with its own optimizer changes what happens to diagrams of that
  kind at safe and normal. The report always states the kind and level applied,
  so the change is visible.
- Each operation is a separate unit with its own switch and its own tests.
