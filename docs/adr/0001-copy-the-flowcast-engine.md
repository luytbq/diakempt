# ADR-0001: Copy the flowcast layout engine instead of importing it

- Status: accepted
- Date: 2026-10-02

## Context

flowcast (github.com/luytbq/flowcast) has a layered layout and routing engine for
flowcharts and swimlane activity diagrams, with an optimizer, a geometry
self-check, goldens and invariants. diakempt needs that engine for three jobs:
relayout of flowcharts, relayout of swimlane diagrams, and the aggressive level on
unknown diagrams.

The engine's only entry point, layout.Lay, takes Flow Table rows: strings for id,
type, parent, content and metadata. It measures every element from its text with a
Verdana 12 width table, with no way to pass a fixed size or a font size. diakempt
works on a typed model of cells and needs both.

Options considered:

- Import flowcast as a module and add a typed entry point there. One engine, but
  every diakempt need becomes a flowcast API change kept compatible with the Flow
  Table path.
- Move the engine into a third module both tools import. Cleanest, but a large
  move before the needed API is known.
- Copy the engine into diakempt and change it freely.

## Decision

Copy the engine into diakempt and change it freely: a typed input of nodes, wires
and lanes; a fixed size per element; a font scale per element.

The packages layout, schema, model, num, validate and internal/unistr live in
engine/, and text and data at the top level, copied at build steps 5 and 7 of
docs/design.md.

- Source commit: flowcast 46e5239458d21022848a93c4721c29e68bf1f8e7.

## Consequences

- diakempt's engine can change shape without keeping Flow Table compatibility.
- The two engines drift apart. A fix made in flowcast after the source commit is
  not in diakempt until someone carries it over by hand; the source commit above
  is the starting point for finding such fixes.
- Right after the copy, 84 of 101 flowcast-generated diagrams relay out to
  exactly the geometry flowcast drew. The rest differ because the table's branch
  order cannot always be recovered from the drawing. The test is a starting
  point and is expected to drift as the engine changes.
- Whether flowcast keeps developing, freezes, or is replaced by diakempt is not
  decided here.
