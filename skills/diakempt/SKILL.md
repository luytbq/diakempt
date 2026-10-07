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
- "seqlayout not possible: fewer than two UML lifelines": the sequence diagram
  is hand-drawn (boxes and dashed lines), which seqlayout does not handle; only
  the general tidy ran. Redrawing it with the UML lifeline shapes lets
  seqlayout run.
- "seqlayout not possible: message ... does not join two lifelines": a found
  or lost message, or an arrow to a note; the general tidy ran instead.
- Kind unknown but the user says it is a class diagram drawn with plain boxes:
  classlayout only recognizes the editor's class shape; --type class forces
  the class layout on whatever shapes are there.
- input.embedded error (.drawio.svg or .drawio.png): convert first, for
  example with drawio -x -f xml -o file.drawio file.drawio.svg, then tidy the
  .drawio file.
- input.not_drawio, input.malformed, input.no_pages, input.page_decode: the
  file is not a readable draw.io document; tell the user which.
- Exit code 3 under --strict: there was a warning or a worse metric; read the
  report rather than treating it as a crash.
- A result the user dislikes: try a lower level, switch single operations off
  (--no-NAME), or adjust a setting; the flags are in features.md.

## Keeping this skill true

Capabilities live in docs/features.md, which a test keeps in step with the
code. When a new kind of report message or failure appears, add its handling
to the playbook above.
