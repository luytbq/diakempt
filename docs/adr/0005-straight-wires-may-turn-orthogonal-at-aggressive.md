# ADR-0005: Straight wires may turn orthogonal at the aggressive level

- Status: accepted
- Date: 2026-10-31
- Supersedes: the "diakempt cannot restyle a diagram" consequence of ADR-0002,
  and "wire style" in the list of what every level keeps in ADR-0003, for the
  aggressive level only.

## Context

ADR-0002 limits diakempt to geometry, and ADR-0003 has every level keep wire
style. A straight wire that crosses a shape or runs on top of another wire is
rerouted with waypoints around the obstacles, keeping its style. When no such
route scores better, the wire stays broken, although a right-angle route
through side ports would clear it.

The aggressive level already changes more than geometry: it normalizes
structure and may break relative order. A user who asks for it accepts a
diagram that looks different from the drawing.

## Decision

At the aggressive level, an operation named wirestyle turns a straight wire
(no edge style, not curved) that is still broken after reroute into an
orthogonal one: it sets edgeStyle=orthogonalEdgeStyle and routes the wire at
right angles through fixed ports. The change is kept only if the diagram
scores better. It also runs after a kind layout or relayout accepted at
aggressive.

- Only straight to orthogonal. Curved wires are a presentation choice and keep
  their style; orthogonal wires never turn straight.
- Only wires reroute could not clear. A wire that waypoints fix keeps its style.
- Every change is reported per wire ("made wire ID orthogonal").
- safe and normal keep wire style, as before. --no-wirestyle switches it off
  at aggressive, and --with-wirestyle runs it at a lower level.

## Consequences

- The geometry-only rule now has three exceptions: snapped wire ends,
  reported normalizations at aggressive, and reported wire style changes at
  aggressive.
- The style key written is edgeStyle; other keys keep their order and values.
- Reroute fixes most broken straight wires with waypoints, so wirestyle
  rarely fires. Preferring a right-angle route over a straight one with
  waypoints, or making a diagram's wires consistent, would be a new decision.
