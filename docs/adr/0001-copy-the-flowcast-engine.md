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

The copy is made at build step 7 of docs/design.md. The flowcast commit it is
taken from is recorded here at that time.

- Source commit: not copied yet.

## Consequences

- diakempt's engine can change shape without keeping Flow Table compatibility.
- The two engines drift apart. A fix made in flowcast after the source commit is
  not in diakempt until someone carries it over by hand; the source commit above
  is the starting point for finding such fixes.
- Right after the copy, tidying a flowcast-generated file gives the same geometry
  as building it from its table. That check is a starting test and is expected to
  stop holding as the engine changes.
- Whether flowcast keeps developing, freezes, or is replaced by diakempt is not
  decided here.
