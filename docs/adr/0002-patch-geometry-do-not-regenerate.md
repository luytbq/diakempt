# ADR-0002: Patch geometry into the original file instead of regenerating it

- Status: accepted
- Date: 2026-10-02

## Context

A user's draw.io file carries far more than a layout: styles, HTML rich text,
images, properties on UserObject cells, decorations, extra pages, the compression
of each page. A generator that builds a file from a model derives every style from
the model, so a round trip through a model loses whatever the model cannot hold.

## Decision

The original file is the base of the output. diakempt replaces only positions,
sizes, waypoints, wire anchors and label positions. On top of that it writes:

- snapped wire ends, at every level;
- the reported normalizations, at the aggressive level only.

Every other attribute, cell and page is written back as it was read, including the
order of style keys and the compression of each page.

## Consequences

- Reading a file and writing it back without edits must yield an equivalent file.
  This is the first test of the doc package.
- Preservation becomes testable: comparing input and output, everything except
  geometry must be identical, apart from snapped ends and reported normalizations.
- Shapes diakempt does not understand are still kept intact; at worst they are
  laid out as plain boxes.
- diakempt cannot restyle a diagram. Changing wire style, for example straight to
  orthogonal, would need a new decision.
