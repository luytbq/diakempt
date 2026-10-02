# ADR-0004: Never make a diagram worse

- Status: accepted
- Date: 2026-10-02

## Context

diakempt rewrites files people drew by hand, which have no source to regenerate
from. Its operations are heuristics: thresholds, detection scores, weights. Some
inputs will defeat them, and a tool that sometimes leaves a diagram worse than it
found it is a tool people stop trusting with --in-place.

## Decision

After optimizing a diagram, diakempt scores it before and after. Heavy defects
(wires through nodes, overlapping nodes) weigh most, then overlapping labels, then
wire crossings; area and total wire length count with a low weight.

If the score got worse, the diagram is retried one level lower, aggressive -->
normal --> safe. If safe is still worse, the diagram keeps its original geometry.
Snapped wire ends are kept in every case. Each diagram steps down on its own.

--force keeps the result even when it is worse. The report states every step down
and its reason.

## Consequences

- "Not worse" is an invariant over the whole corpus, at every level.
- The score defines what better means. Changing its weights changes behavior and
  needs the corpus goldens reviewed.
- A run costs up to three optimizations per diagram in the worst case.
- Improvements the score cannot see, such as a more readable arrangement with the
  same defect counts, are not protected. That is accepted.
