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
  pointer with the entry's `runtime.Pinner`, stores the `*pinEntry` in
  `instances` under `o.GoPointer()`, and gives GLib a non-zero ID as the data of
  `SetDataFull("prevent_gc", id, …)` with 1 package-level destroy notify. GLib
  calls that notify with the ID when it finalizes the object, and also when
  `Pin` sets the same key again on the object (then the previous Go pointer is
  released). The notify deletes `pins[id]`, deletes the registry entry only if
  it is still that same entry, and unpins. `Get` returns nil after
  finalization, or after a later `Pin` replaced the entry
  ([frostyard/snowkit#12](https://github.com/frostyard/snowkit/issues/12) is
  fixed). `InstanceRegistry` and the `pins` table each have a mutex, because
  finalization can run on another thread. Never call into GLib while you hold a
  lock: `SetDataFull` can call the old destroy notify synchronously.
- **Check a GLib lifecycle claim against the binding.** Before a doc or a
  comment says when GLib calls a callback, read the puregotk doc comment of the
  call (in the module cache) and run it once with GLib on the host. A claim
  from the GLib function name alone was wrong once (#11, `SetDataFull` with
  NULL data).
- **Callback slots are counted per function address.** purego keeps 1 callback
  slot per function-variable ADDRESS and has 2000 per process on Linux and
  macOS. `gtk` passes the package-level `idleSourceFunc` to every
  `glib.IdleAdd`, and `gobj` passes the package-level `pinDestroyNotify` to
  every `SetDataFull`, so each of them uses 1 slot for the whole process
  ([frostyard/snowkit#10](https://github.com/frostyard/snowkit/issues/10) is
  fixed). Do not add a per-call `glib.NewCallback` path, and never pass a
  function variable declared inside a function by address to puregotk: its new
  address takes a new slot that is never freed.
- **`toast.Notifier` is a contract only.** It has `ShowToast(message string)`
  and `ShowErrorToast(message string)`; the consuming application implements
  it. Do not add an implementation to this module.
- **puregotk, not cgo.** puregotk loads GTK with `dlopen` through purego. The
  module builds with no C compiler, but GTK and GLib shared libraries must be
  present at run time. A test that calls GTK or GLib needs those libraries on
  the host: CI's `ubuntu-latest` runners have GLib and GObject
  (`libglib-2.0.so.0`, `libgobject-2.0.so.0`) and no GTK library.
- **Actions are pinned.** Every `uses:` in `.github/workflows/` is a full
  40-character commit SHA with a `# vX.Y.Z` comment (frostyard/core ADR-0021).
  A reusable workflow from a repository with no release tags names the pull
  request of the pinned commit instead (`# core#225` in `risk-paths.yml`).
  `internal/workflowpin/workflowpin_test.go` fails otherwise. The `gtk` and
  `gobj` tests need only `libglib-2.0.so.0` and `libgobject-2.0.so.0` on the
  host (see the puregotk rule); `internal/workflowpin` needs nothing.
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

**Risk paths.** `.github/risk-paths.json` lists the risk paths, each at a level
from L1 to L4, with a `default_level` of L3 for a path that no glob matches. A
PR gets 1 level label (`level:L1` to `level:L4`) and, at L1 or L2, `risk:high`,
from the `.github/workflows/risk-paths.yml` caller of core's `pr-triage.yml`
(core ADR-0078). The caller reads the list at the base and keeps the strictest
level; it also removes `ready-for-human` when the head moves after the label,
or when CI at the head is not green when the label comes (core ADR-0077). A PR
with `risk:high` declares tier 3. Add a path when you add a file of that kind
(signing, auth, publish, secrets, destructive operations, CI); for an L1 or L2
entry, its `why` starts with a category of core's risk-paths spec.

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
reviewed at its current head, has green CI and is ready for the owner; the PR
triage caller removes it when the head moves after the label, or when the
label comes while CI at the head is not green),
`sweep` (a finding of the conformance sweep), `level:L1` to `level:L4` (the
review level of the changed paths) and `risk:high` (the alias of L1 and L2);
the last 2 come from the `.github/workflows/risk-paths.yml` caller of core's
`pr-triage.yml`.
