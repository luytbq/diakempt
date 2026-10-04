# diakempt

diakempt tidies the layout of existing draw.io diagrams of any kind, keeping
everything that is not geometry. Read docs/design.md before changing anything, and
CONTEXT.md for the vocabulary.

## Language

Everything in this repository is in English: code, comments, test names, CLI
output, reports, error messages, documentation and commit messages.

## Feature list

docs/features.md lists every capability the tool has today: commands, flags,
levels, detected kinds, accepted formats, report fields. The agent skill points
agents to it, so it must stay true.

- Any change that adds, removes or changes a capability updates docs/features.md
  in the same commit.
- Planned work does not go into features.md; it belongs in docs/design.md.
- The agent skill in skills/diakempt/SKILL.md points to features.md instead of
  repeating it. A change that adds a report message, an issue code or a
  failure reason also adds its handling to the skill's playbook.

## Core boundary

The root package and everything it imports form the core. The core takes bytes
and returns bytes plus a report. It does not read or write files, call external
processes or print. Files, flags, exit codes and the drawio CLI belong to cmd and
tools.

## Correctness

- Geometry only: an operation that changes a style, a text value, a property or a
  connection (other than a snapped wire end, or a reported normalization at the
  aggressive level) is a bug.
- Never worse: a diagram whose quality score gets worse steps down a level, then
  keeps its original geometry.
- Real user files never enter the repository. Test files are synthetic; real files
  go in the git-ignored local/ folder.
- Randomized algorithms take a seed; optimizers stop on an iteration budget, never
  on wall-clock time.

## Decisions

Settled decisions live in docs/adr/. A change that contradicts one needs a new ADR
that supersedes it.

## Commands

    go build ./...
    go vet ./...
    go test ./...

go test also writes showcase/NAME.before.drawio and NAME.after.drawio for a
fixed set of examples (TestShowcase), for judging results in draw.io.
