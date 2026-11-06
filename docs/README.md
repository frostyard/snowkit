# Documentation

Docs are split by the question they answer:

| Directory | Question | Contents |
| --- | --- | --- |
| [adr/](adr/) | **Why** did we choose this? | Architecture Decision Records — immutable once accepted; superseded, never edited. Org-wide decisions in [org-adrs.md](org-adrs.md) |
| [design/](design/) | **How** does it fit together? | Living documents describing the current architecture |
| [specs/](specs/) | **What exactly** is the contract? | Precise, testable interface definitions |
| [plans/](plans/) | **When/in what order** do we build? | Roadmaps and phase plans; updated as work lands |

The exported Go API is documented on
[pkg.go.dev](https://pkg.go.dev/github.com/frostyard/snowkit).

## Index

### Decisions (ADRs)

- [0001 — Record architecture decisions](adr/0001-record-architecture-decisions.md)
- [0002 — Agent-portable instruction surface](adr/0002-agent-portable-instruction-surface.md)
- [0003 — Verify gate, risk paths and corrections](adr/0003-verify-gate-risk-paths-and-corrections.md)
- [Org-wide decisions (frostyard/core ADRs)](org-adrs.md)

### Design

- [overview.md](design/overview.md) — purpose, the 3 packages (`gtk`, `gobj`,
  `toast`) and their invariants, key patterns, the callback slot limit, build,
  test, release and CI

### Specs

*(none yet)*

### Plans

*(none yet)*

## Conventions

- **New docs start from their category's `TEMPLATE.md`** (in each directory).
- New decision → new ADR with the next number; if it reverses an old one, mark
  the old one `Superseded by NNNN` rather than editing it.
- Design docs are updated in place to always reflect reality.
- Specs change only alongside the code that implements them.
- Cross-links between categories are mandatory in both directions — see the
  documentation rules in [AGENTS.md](../AGENTS.md) (CLAUDE.md, GEMINI.md,
  .cursorrules and .github/copilot-instructions.md are symlinks to it,
  ADR-0002).
- Adding a doc means adding it to the index above.
