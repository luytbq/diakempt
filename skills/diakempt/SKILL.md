---
name: diakempt
description: Tidies the layout of existing draw.io (.drawio, .xml) files with the diakempt CLI - reattaches arrows dropped next to shapes, removes overlaps and wires through shapes, relays out flowcharts and swimlane diagrams, and reports what it changed per diagram. Use when the user wants to clean up, tidy, fix, straighten or re-layout a draw.io file they already have, not when creating a new diagram from scratch.
---

# diakempt

diakempt reads a draw.io file, finds the diagrams on each page, recognizes
their kind, and tidies their geometry. Styles, text and decorations are kept;
only positions, sizes, waypoints and wire anchors change (plus structure at the
aggressive level).

## Source of truth

Before choosing flags, read the feature list:
/Users/luytbq/projects/l/diakempt/docs/features.md

It lists every flag, level, operation, setting, detected kind, input format,
report field and exit code that exists today. Do not rely on memory or on this
skill for those; this skill only covers how to drive the tool and what to do
in common situations.

## Quick start

Build once (Go 1.25 or later):

    cd /Users/luytbq/projects/l/diakempt && go build -o local/bin/diakempt ./cmd/diakempt

Look before writing anything:

    local/bin/diakempt --dry-run --verbose path/to/file.drawio

Then run it; the result goes to file.tidy.drawio next to the input:

    local/bin/diakempt path/to/file.drawio

## Workflow

1. Dry run with --json and read the report: diagrams found, kind and
   confidence of each, operations planned, metrics before and after, issues.
2. Pick the level with the user if it is not obvious:
   - safe: fix defects only, keep the arrangement;
   - normal (default): also align, even out and tidy sizes;
   - aggressive: may rewrite structure and relay out unknown diagrams. It
     deletes and merges cells, so confirm with the user first.
3. Run without --dry-run. Write next to the input (the default). Use
   --in-place only when the user asked to overwrite; it keeps a numbered .bak.
4. Verify. If the drawio CLI is installed, export before and after to PNG and
   look at both:

       drawio -x -f png -o before.png file.drawio
       drawio -x -f png -o after.png file.tidy.drawio

5. Report to the user per diagram: its identifier, kind, level applied, the
   changes, the metrics before and after, and every warning with what it
   means for them.

## Playbook

What the report says, and what to do:

- snap.ambiguous warning: an arrow end is about as close to two shapes, so it
  was left unattached. Tell the user both candidates (named in the message) and
  let them attach it in draw.io; do not guess for them.
- "stepped down" with applied lower than asked, or applied none: the stronger
  result scored worse than the original, or broke the order of shapes, so a
  milder one (or none) was kept. Explain the reason given. Offer --force only
  if the user wants to see the worse result anyway.
- "classlayout not possible": the class diagram has classes inside a
  container (a package), a wire with a free end, or a wire ending on a text.
  The general tidy still ran; attaching the wire in draw.io lets the class
  layout run next time.
- "erlayout not possible": the ER diagram has tables inside a container, a
  relation with a free end, or a wire ending on a text. The general tidy
  still ran; attaching the relation to a table row in draw.io lets the ER
  layout run next time.
- "commlayout not possible": the communication diagram holds a container, or
  a link with a free end or ending on a text; the general tidy still ran.
  Attaching the link in draw.io lets the layout run next time.
- "deploylayout not possible": a link has a free end or ends on a text; the
  general tidy still ran. Attaching the link lets the layout run next time.
- "pkglayout not possible": the package diagram has a shape inside a container
  that is not part of it, or a dependency with a free end or ending on a
  text; the general tidy still ran.
- "usecaselayout not possible": the use case diagram holds a container, a
  second boundary, a shape that is neither an actor, a use case, a note nor a
  text, or a wire with a free end; the reason names it. The general tidy still
  ran; attaching the wire, or moving the extra shape out of the diagram, lets
  the use case layout run next time.
- "flowlayout not possible" or "relayout not possible": the engine cannot
  represent the diagram. Map the reason to advice:
  - a wire with a free end: attach it in draw.io, or accept the general tidy;
  - a wire ending on a container, or containers that are not lanes: the
    diagram is outside what the engine lays out; the general tidy still ran;
  - a shape outside every lane: move it into a lane in draw.io.
- Kind unknown but the user says it is a flowchart or swimlane: rerun with
  --type flowchart or --type swimlane.
- Kind detected wrongly and the user does not want a relayout: rerun with
  --type unknown.
- A page holds diagrams of different kinds and one is detected wrongly: force
  only that one with --type 'ID=KIND', copying ID from the report, for example
  --type 'Page-1 #2=sequence'; a plain --type would force every diagram.
- type.no_diagram warning: an ID given to --type matches no diagram of that
  file. Diagram indexes count from 1 per page, and a quoted name must match the
  report exactly; rerun with --dry-run to read the IDs, then fix the flag.
- "seqlayout not possible: fewer than two UML lifelines": the diagram mixes
  one UML lifeline with hand-drawn ones; only the general tidy ran. Drawing
  every participant the same way lets seqlayout run.
- seqlayout not possible on a hand-drawn sequence (boxes and dashed lines);
  only the general tidy ran. Map the reason to advice:
  - "lifeline ... hangs from no shape": a dashed line has no box within 40px
    above it; move its head box over it;
  - "lifelines ... hang from one shape": two dashed lines under one box;
  - "message ... is attached to a lifeline head" or "to a shape that is not a
    lifeline head": an arrow is connected to a box instead of ending on a
    dashed line; detach it in draw.io and drop its end on the line;
  - "message ... starts and ends at the same point": a self call drawn as a
    straight line; give it waypoints;
  - "container ... in a hand-drawn sequence": hand-drawn sequences inside
    containers are not laid out;
  - "fewer than two lifelines": fewer than two dashed lines were recognized.
- "seqlayout not possible: message ... does not join two lifelines" (an
  attached message on a hand-drawn sequence) or "message ... does not touch a
  lifeline" (UML lifelines): an arrow to a note, or a line that is neither a
  message nor an operand separator across a frame; the general tidy ran
  instead.
- "seqlayout not possible: message ... is attached to ..., which is not on a
  lifeline": a message connected to a container, or two messages connected
  to one dot; detach the extra end in draw.io.
- "seqlayout not possible: message ... has an end without a position": a
  free end with no stored point; reconnect it in draw.io.
- Kind unknown but the user says it is a class diagram drawn with plain boxes:
  classlayout only recognizes the editor's class shape; --type class forces
  the class layout on whatever shapes are there.
- input.embedded error (.drawio.svg or .drawio.png): convert first, for
  example with drawio -x -f xml -o file.drawio file.drawio.svg, then tidy the
  .drawio file.
- input.not_drawio, input.malformed, input.no_pages, input.page_decode: the
  file is not a readable draw.io document; tell the user which.
- Exit code 3 under --strict: there was a warning, or the final kept result is
  worse than the original under the never-worse rule (docs/adr/0004). --force
  keeps such a result, and several passes can also add up past the tolerance
  a single pass stayed within. Its "worse:" line names the defect score before
  and after, or the area and wire length when only those grew. Read the
  report rather than treating it as a crash.
- A result the user dislikes: try a lower level, switch single operations off
  (--no-NAME), or adjust a setting; the flags are in features.md.

## Keeping this skill true

Capabilities live in docs/features.md, which a test keeps in step with the
code. When a new kind of report message or failure appears, add its handling
to the playbook above.
