# Org-wide decisions (frostyard/core ADRs)

Conventions this repository follows that are decided at the org level are
recorded as ADRs in
[frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr).
The ones that bind snowkit:

- [ADR-0002 — Agent-portable instruction surface](https://github.com/frostyard/core/blob/main/docs/adr/0002-agent-portable-instruction-surface.md) — one canonical [`AGENTS.md`](../AGENTS.md); `CLAUDE.md`, `GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are symlinks to it, and `.claude/skills` is a symlink to [`.agents/skills/`](../.agents/skills/)
- [ADR-0012 — svu-derived versions and make bump](https://github.com/frostyard/core/blob/main/docs/adr/0012-svu-versioning-and-rolling-dev-prerelease.md) — `make bump` tags `svu next`; snowkit has no release workflow and no rolling `dev` prerelease
- [ADR-0018 — Org-wide agent instruction and knowledge surfaces](https://github.com/frostyard/core/blob/main/docs/adr/0018-org-wide-agent-instruction-and-knowledge-surfaces.md) — `AGENTS.md` is canonical, the per-tool files are the symlinks above, and durable corrections live in the append-only [`.memory/corrections.jsonl`](../.memory/README.md) (5 fields: `date`, `scope`, `correction`, `evidence`, `promoted_to`)
- [ADR-0019 — Governance as machine-readable policy with risk tiers](https://github.com/frostyard/core/blob/main/docs/adr/0019-governance-as-code-and-risk-tiers.md) — the [PR template](../.github/pull_request_template.md) declares a risk tier
- [ADR-0021 — SHA-pinned actions and least-privilege CI workflows](https://github.com/frostyard/core/blob/main/docs/adr/0021-sha-pinned-actions-and-least-privilege-ci.md) — binds [`.github/workflows/`](../.github/workflows/): every action is pinned to a commit SHA with a `# vX.Y.Z` comment (checked by `internal/workflowpin`), `permissions: {}` at the top and `contents: read` per job, and checkout sets `persist-credentials: false`
- [ADR-0025 — One docs/ tree per repository, in core's four-category shape](https://github.com/frostyard/core/blob/main/docs/adr/0025-consolidate-repository-docs-into-docs.md) — this [`docs/`](README.md) tree in `adr/`, `design/`, `specs/` and `plans/`, formerly the `yeti/` tree, indexed in [docs/README.md](README.md)
- [ADR-0029 — ACMM conformance via canonical aliases](https://github.com/frostyard/core/blob/main/docs/adr/0029-acmm-conformance-via-canonical-aliases.md) — the alias symlinks above are committed, relative and resolve to `AGENTS.md`
- [ADR-0043 — Pin repository tools in mise and name the verify gate](https://github.com/frostyard/core/blob/main/docs/adr/0043-pin-repository-tools-in-mise-and-name-the-verify-gate.md) — [`mise.toml`](../mise.toml) pins `golangci-lint` and `node` with a committed [`mise.lock`](../mise.lock); `go.mod` is the only Go pin; the Makefile reads the lint pin so `make lint` and CI cannot drift
- [ADR-0044 — Expose the make gate triad in every repository](https://github.com/frostyard/core/blob/main/docs/adr/0044-expose-the-make-gate-triad-in-every-repository.md) — the [`Makefile`](../Makefile) exposes `make verify` (credential-free, non-mutating), `make check` (developer, may format) and `make ci` (the gate CI runs)
- [ADR-0051 — ADRs state facts only, not the process that produced them](https://github.com/frostyard/core/blob/main/docs/adr/0051-adrs-state-facts-not-process.md) — every ADR in [adr/](adr/) gives context, decision, consequences and alternatives only
- [ADR-0053 — Name the svu config file .svu.yml](https://github.com/frostyard/core/blob/main/docs/adr/0053-name-the-svu-config-svu-yml.md) — [`.svu.yml`](../.svu.yml), never `.svu.yaml`
- [ADR-0068 — Label pull requests that touch high-risk paths](https://github.com/frostyard/core/blob/main/docs/adr/0068-label-pull-requests-that-touch-high-risk-paths.md) — [`.github/risk-paths.json`](../.github/risk-paths.json) lists this repository's high-risk paths and the caller [`.github/workflows/risk-paths.yml`](../.github/workflows/risk-paths.yml) applies the `risk:high` label
- [ADR-0071 — Hold every code repository to one baseline](https://github.com/frostyard/core/blob/main/docs/adr/0071-hold-every-code-repository-to-one-baseline.md) — the [repository baseline](https://github.com/frostyard/core/blob/main/docs/specs/repository-baseline.md) rows B1 to B16; snowkit is a library, so B9 (tool docs site) does not apply: pkg.go.dev documents it

When changing behavior covered by one of these, update or supersede the ADR
in frostyard/core first, then change this repository in the same effort.
