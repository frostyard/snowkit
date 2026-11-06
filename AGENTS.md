# snowkit

A Go library (`github.com/frostyard/snowkit`) for GTK4 and GNOME applications
built on [puregotk](https://codeberg.org/puregotk/puregotk): main-thread
scheduling (`gtk`), GObject subtype registration (`gobj`) and a toast
notification contract (`toast`). Its consumer in the fleet is
frostyard/chairlift. Start at [docs/README.md](docs/README.md); the
architecture is in [docs/design/overview.md](docs/design/overview.md).

This file (`AGENTS.md`) is the CANONICAL agent instructions — `CLAUDE.md`,
`GEMINI.md`, `.cursorrules`, and `.github/copilot-instructions.md` are symlinks
to it, and `.claude/skills` symlinks to `.agents/skills/`
([ADR-0002](docs/adr/0002-agent-portable-instruction-surface.md)). Edit only
the canonical paths; keep content tool-agnostic.

## Skills (follow these for common tasks)

Step-by-step procedures live in [.agents/skills/](.agents/skills/); follow
them rather than improvising, whichever agent you are:

- *(none yet — add the first one when a procedure repeats)*

## Build and test commands

```bash
make verify       # the credential-free, non-mutating gate: tidy diff, vet, gofmt -l, pinned lint, template gate, tests
make check        # developer gate: fmt (gofmt -w), the static checks of verify, then go test -v
make ci           # verify, the race tests, and with BASE set the corrections append-only check
make ci BASE=origin/main   # what CI job Verify runs, against the PR base
make test         # go test -v ./...
make test-race    # go test -race -short ./...
make lint         # golangci-lint run; fails when golangci-lint is missing or is not the mise.toml pin
make fmt          # gofmt -w on all .go files
make tidy         # go mod tidy
make test-cover   # coverage.out and coverage.html
make bump         # owner only: tag the next version with svu and push the tag
```

`mise install` provisions the pinned `golangci-lint` and `node`
([mise.toml](mise.toml), checksums in [mise.lock](mise.lock)). The gate needs
Node.js 22 or later for the template gate.

## Code conventions (live — the code exists)

- **3 independent packages.** `gtk`, `gobj` and `toast` never import each
  other. `gtk` imports puregotk's `glib`; `gobj` imports `glib` and `gobject`;
  `toast` imports nothing. Keep it so: a consumer imports only the package it
  needs.
- **`gtk.RunOnMainThread` keeps the lock short.** `gtk/mainthread.go` holds
  `idleCallbackMu` only to read or write `idleCallbacks` and the ID counter,
  never while it runs the user's callback, so a callback can call
  `RunOnMainThread` again without a deadlock. The Go closure crosses the C
  boundary only as its `uintptr` ID (the `data` argument of `glib.IdleAdd`).
- **`gobj` lifecycle.** `InstanceRegistry.Pin` (`gobj/register.go`) pins the Go
  pointer with a `runtime.Pinner` and stores it in `instances` under
  `o.GoPointer()`. The intended cleanup (a `glib.DestroyNotify` that deletes
  the entry and unpins at finalization) never runs: `Pin` passes data `0` to
  `SetDataFull("prevent_gc", 0, …)`, and GLib calls no destroy callback for
  NULL data. So every `Pin` leaks its entry and its pin
  ([frostyard/snowkit#12](https://github.com/frostyard/snowkit/issues/12)). A
  fix must make the pin-then-unpin-on-finalize lifecycle real, with a test.
  `InstanceRegistry` has no lock: call `Pin` and `Get` on the GTK main thread.
- **Check a GLib lifecycle claim against the binding.** Before a doc or a
  comment says when GLib calls a callback, read the puregotk doc comment of the
  call (in the module cache) and run it once with GLib on the host. A claim
  from the GLib function name alone was wrong once (#11, `SetDataFull` with
  NULL data).
- **Each `RunOnMainThread` call and each `Pin` uses 1 purego callback slot that
  is never freed** (limit 2000 per process on Linux and macOS). Do not add a
  new per-call `glib.NewCallback` path; the fix is tracked in
  [frostyard/snowkit#10](https://github.com/frostyard/snowkit/issues/10).
- **`toast.Notifier` is a contract only.** It has `ShowToast(message string)`
  and `ShowErrorToast(message string)`; the consuming application implements
  it. Do not add an implementation to this module.
- **puregotk, not cgo.** puregotk loads GTK with `dlopen` through purego. The
  module builds with no C compiler, but GTK and GLib shared libraries must be
  present at run time. A test that calls GTK or GLib needs them on the host;
  CI's `ubuntu-latest` runners do not install them.
- **Actions are pinned.** Every `uses:` in `.github/workflows/` is a full
  40-character commit SHA with a `# vX.Y.Z` comment (frostyard/core ADR-0021).
  A reusable workflow from a repository with no release tags names the pull
  request of the pinned commit instead (`# core#194` in `risk-paths.yml`).
  `internal/workflowpin/workflowpin_test.go` fails otherwise; it is the only
  Go test in the module.
- **Go comes from `go.mod` only.** CI reads the `go` line (`go-version-file:
  go.mod`) and runs `govulncheck` v1.6.0. When govulncheck reports a reachable
  standard-library vulnerability, raise the `go` line to the lowest patch that
  fixes it.
- Run `make verify` before you call any change done. CI job `Verify` runs
  `make ci`, which also checks that `.memory/corrections.jsonl` is append-only
  against the PR base; run `make ci BASE=origin/main` for the same check
  locally.

**Test reliability.** No sleeps for timing (wait on a condition or inject a
clock). No network and no wall clock in unit tests (inject them). A flaky test
gets an issue, then a fix or a quarantine (skip with the issue link) in the
same PR. Never retry a job or a test until it is green.

**Blocker to guard.** When a review (agent cross-review, owner review, or a
human) finds a blocking point, the PR that fixes it also adds a guard: a
targeted test that fails without the fix (preferred), or an `AGENTS.md` rule
when no test can catch it. The same PR appends 1 line to
`.memory/corrections.jsonl` (`evidence`: the PR and the review;
`promoted_to`: the test or the rule). See `.memory/README.md`.

**Risk paths.** `.github/risk-paths.json` lists the high-risk paths; a PR that
touches one gets `risk:high` from `.github/workflows/risk-paths.yml` and
declares tier 3. Add a path when you add a file of that kind (signing, auth,
publish, secrets, destructive operations, CI).

**Vendored files.** `scripts/lib/risk-paths.mjs` and
`scripts/check-corrections.mjs` are copies from frostyard/core; never edit them
by hand (`make verify` checks their hashes).

## Repository boundary

What does not belong here: application code (windows, pages, a toast
implementation), secrets, and generated files such as `coverage.out` and
`coverage.html`.

Releases and dependencies:

- **Tag format:** `v0.x.y` (the first tag is `v0.1.0`). There is no release
  workflow: a tag is the release, and consumers get it through the Go module
  proxy.
- **Version rule:** `svu next`, configured by [.svu.yml](.svu.yml): `v0: true`
  keeps the version below v1.0.0, and `always: true` makes a patch bump when
  no commit since the last tag asks for a bump.
- **Who tags:** the repository owner. Agents never run `make bump`, never tag,
  push a tag or publish a release.
- **Dependencies:** `codeberg.org/puregotk/puregotk` (direct) and
  `codeberg.org/puregotk/purego` (indirect). Dependabot updates `gomod` and
  `github-actions` weekly.
- **Who uses this module:** frostyard/chairlift (`gobj` and `gtk`). An exported
  API change needs a matching chairlift change.

## Documentation rules (enforced)

Docs live in `docs/` in four categories. **Every new doc starts from its
category's `TEMPLATE.md`** and follows its structure:

- `docs/adr/` — why we decided. Immutable once Accepted; reversals are new
  ADRs that mark the old one Superseded.
- `docs/design/` — how it fits together. Living; updated in place to match
  reality.
- `docs/specs/` — exact contracts. Change only alongside implementing code.
- `docs/plans/` — order of work. Phases with "Done when" outcomes.

After a source change, update [docs/design/overview.md](docs/design/overview.md),
[README.md](README.md) and this file. A task is not complete until the docs
match the code.

A decision local to this repository gets an ADR in `docs/adr/`. A decision that
binds more than this repository goes to
[frostyard/core](https://github.com/frostyard/core/tree/main/docs/adr) and gets
a line in [docs/org-adrs.md](docs/org-adrs.md).

### Cross-linking is mandatory

A doc without its required links is incomplete — do not finish a docs change
until they exist, in both directions:

- **ADR** → links every design doc/spec it shapes, and prior ADRs it builds on.
- **Design doc** → links the ADR(s) providing its rationale, the spec(s)
  pinning its contracts, and the roadmap phase that builds it.
- **Spec** → links its motivating ADR(s) and the design doc showing where it
  fits.
- **Plan** → every phase links the design docs/specs it implements; resolved
  open questions become ADRs.

When you touch a doc, verify its links still hold (targets exist, section
anchors valid) and add the back-links on the targets. Use relative paths.

### Housekeeping

- New doc ⇒ add a line to the index in [docs/README.md](docs/README.md).
- New significant decision ⇒ new ADR *first*, then update the affected design
  docs/specs in the same change.
- Convert relative dates ("next weekend") to absolute dates in all docs.

## Issues

Bugs and feature requests live in GitHub Issues; a fix arrives as a pull
request that closes its issue. Labels in use: `ready-for-human` (a PR that is
reviewed, has green CI and is ready for the owner), `sweep` (a finding of the
conformance sweep), `risk:high` (applied by the risk-paths workflow).
