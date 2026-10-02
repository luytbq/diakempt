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
| --strict | exit 3 when any file has a warning or a diagram scored worse |

Report fields per file: pages, diagrams, decoration, snap (snapped, ambiguous,
details), normalized, issues (code, severity, page, diagram, cells, message),
changed. Per diagram: id, page, index, name, kind, confidence, signals, forced,
level, applied, step_downs, operations, details, before, after. Metrics:
wires_through_nodes, node_overlaps, label_overlaps, wire_crossings, wire_overlaps,
area, wire_length, score.

Today the report fills pages and issues; diagrams stay empty because no
detection or optimization runs yet.

## Exit codes

- 0: success
- 1: a file could not be read, processed or written
- 2: bad flags
- 3: --strict found a warning or a worse result

## Levels and operations

| Flag | Effect |
|---|---|
| --level safe, normal, aggressive | how far the layout may change; default normal |
| --type flowchart, swimlane, sequence, unknown | treat every diagram as this kind |
| --with-NAME | run operation NAME whatever the level |
| --no-NAME | skip operation NAME |
| --force | keep results that score worse than the original |
| --seed N | seed for randomized optimizers, default 1 |

These flags are accepted and validated. No operation is implemented yet, so
none of them changes the output.

Operations, by the lowest level that runs them:

- safe: snap, separate, containers, reroute, labels
- normal: align, resize, samesize, spacing, compact, grid
- aggressive: normalize, relayout

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

None yet.

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
