# diakempt

diakempt tidies the layout of draw.io diagrams you already have: wires that cut
through shapes, overlapping labels, misaligned and unevenly spaced shapes, arrows
that were dropped next to their target instead of on it. It recognizes common kinds
of diagram and lays them out with an optimizer made for that kind; anything else
gets a general tidy-up that keeps your arrangement. Colors, fonts, text and
decorations are left as they are.

Status: v1 complete. What works today is listed in
[docs/features.md](docs/features.md).

## Use

    go build -o diakempt ./cmd/diakempt
    ./diakempt --dry-run --verbose diagram.drawio   # see what would change
    ./diakempt diagram.drawio                       # writes diagram.tidy.drawio

--level safe fixes defects only, normal (the default) also shapes the layout
up, aggressive may rewrite structure and relay out any diagram. --in-place
overwrites the input and keeps a numbered .bak. --json prints a machine-readable
report.

## For AI agents

[skills/diakempt/SKILL.md](skills/diakempt/SKILL.md) is a Claude Code skill
that teaches an agent to run the tool and handle its reports. Install it by
linking the folder into ~/.claude/skills/.

## Documentation

| Read this | For |
|---|---|
| [docs/design.md](docs/design.md) | What the tool does, its rules, and the build order |
| [docs/features.md](docs/features.md) | What works today |
| [CONTEXT.md](CONTEXT.md) | The vocabulary used in code and docs |
| [docs/adr/](docs/adr/) | Settled decisions and their reasons |

## License

MIT
