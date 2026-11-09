# snowkit overview

Living document. Rationale: [ADR-0002](../adr/0002-agent-portable-instruction-surface.md)
and [ADR-0003](../adr/0003-verify-gate-risk-paths-and-corrections.md) for the
repository surfaces and the gate; the library code has no ADR yet. Org-wide
decisions: [org-adrs.md](../org-adrs.md).
Contracts: the exported Go API, documented on
[pkg.go.dev](https://pkg.go.dev/github.com/frostyard/snowkit) (no spec in
`docs/specs/` yet). Agent rules: [AGENTS.md](../../AGENTS.md).

## Purpose

snowkit is a Go utility library for GTK4 and GNOME applications built with
[puregotk](https://codeberg.org/puregotk/puregotk). It solves 3 recurring
problems in puregotk applications: running work on GTK's main thread from
goroutines, registering custom GObject subtypes with a safe Go/C lifecycle, and
a toast notification contract that business logic can use without UI imports.
The module path is `github.com/frostyard/snowkit`; frostyard/chairlift imports
`gobj` and `gtk`.

## Architecture

3 independent packages. No package imports another.

```
snowkit/
├── gtk/mainthread.go           RunOnMainThread: schedule a closure on the GTK main loop (imports glib)
├── gobj/register.go            RegisterType, TypeDef, InstanceRegistry (imports glib, gobject)
├── toast/notifier.go           Notifier interface (no imports)
├── internal/workflowpin/       workflowpin_test.go: the action-pinning test (ADR-0021), the only Go test
├── go.mod                      module github.com/frostyard/snowkit, go 1.24.6
├── Makefile                    verify / check / ci gate, lint, test, bump
├── mise.toml, mise.lock        golangci-lint and node pins
├── scripts/, test/             template gate (check-template.mjs) and its Node tests
├── .github/workflows/          ci.yml, risk-paths.yml
├── AGENTS.md                   agent rules (CLAUDE.md, GEMINI.md, .cursorrules, copilot-instructions.md link to it)
└── docs/                       this tree
```

### Package `gtk`: main-thread scheduling

File: `gtk/mainthread.go`.

GTK is not thread-safe: every GTK call must happen on the main thread, but Go
code uses goroutines freely. `RunOnMainThread(fn func())` schedules `fn` on the
GTK main loop with `glib.IdleAdd`. `fn` runs once; the source function returns
`false`, so GLib removes the idle source.

How it works:

1. A package-level map `idleCallbacks` holds pending callbacks under
   incrementing `uintptr` IDs (`idleCallbackID`).
2. `idleCallbackMu` (a `sync.Mutex`) protects the map and the counter.
3. `RunOnMainThread` takes the next ID, stores `fn`, then calls
   `glib.IdleAdd(&cb, id)`. `cb` is a `glib.SourceFunc` that looks up the
   callback by its `data` argument, deletes it from the map, releases the lock
   and then calls it.

Invariant: the mutex is held only to read or write the map, never while the
user's callback runs, so a callback can call `RunOnMainThread` again without a
deadlock.

Typical use:

```go
go func() {
    result := expensiveWork()
    gtk.RunOnMainThread(func() {
        label.SetText(result) // runs on the main thread
    })
}()
```

### Package `gobj`: GObject type registration

File: `gobj/register.go`.

puregotk can use existing GObject types from Go, but a new GObject subtype (a
custom application class or widget) needs registration at the C level and a
lifecycle that keeps Go's garbage collector away from objects that C still
references. `RegisterType(TypeDef)` registers the subtype and returns its GLib
type and an `InstanceRegistry` for the Go↔C pointer map.

Types:

- `TypeDef` describes the subtype:
  - `ParentGLibType func() gobject.Type`: the parent type accessor (for example
    `adw.ApplicationGLibType`).
  - `ClassName string`: the GObject class name (chairlift uses
    `"ChairLiftApplication"`).
  - `ClassInit func(tc *gobject.TypeClass, registry *InstanceRegistry)`: called
    at class initialization to override virtual methods. It is required:
    `RegisterType` calls it with no nil check.
- `InstanceRegistry` maps GObject C pointers (`uintptr`) to Go struct pointers
  (`unsafe.Pointer`):
  - `Pin(o *gobject.Object, goInstance unsafe.Pointer)` stores the pair and
    pins the Go pointer. Its destroy-notify cleanup never runs (see below).
  - `Get(goPointer uintptr) unsafe.Pointer` returns the Go struct for a GObject
    pointer, or `nil`.

Lifecycle. The design intent is "pin while the GObject lives, unpin when it is
finalized"; the current code does only the first half
([#12](https://github.com/frostyard/snowkit/issues/12)):

1. **Registration.** `RegisterType` reads the parent's class and instance sizes
   with `gobject.NewTypeQuery`, then calls `gobject.TypeRegisterStaticSimple`
   with the class-init callback and a no-op instance-init callback.
2. **Pinning.** The consumer calls `registry.Pin(obj, goPtr)` when it builds an
   instance (chairlift does it in its `constructed` override). `Pin`:
   - pins `goPtr` with a new `runtime.Pinner`, so the collector does not move
     or free it;
   - stores `instances[obj.GoPointer()] = goPtr`;
   - calls `SetDataFull("prevent_gc", 0, &cleanup)`, where `cleanup` would
     delete the map entry and call `pinner.Unpin()`.
3. **Cleanup (does not happen).** The data argument of `SetDataFull` is `0`
   (NULL), and GLib does not call the destroy callback when the data is NULL
   (puregotk's doc comment on `Object.SetDataFull`). With NULL data the call
   stores nothing (`GetData("prevent_gc")` returns `0`). So `cleanup` never
   runs: the map entry and the pin stay for the life of the process, also after
   the GObject is finalized.

Current behavior (checked at `4f8bd38` with GLib on the host: a subtype of
`GObject`, `Pin`, then `Unref`; a control `SetDataFull` with data `1` on the
same object is called at finalization):

- After the GObject is finalized, `Get` still returns the Go struct, and the
  struct stays pinned. Each pinned instance leaks 1 Go struct and 1 map entry.
  chairlift pins 1 application object, so it leaks 1 object for the process
  life.
- A second `Pin` on the same object overwrites the map entry; the first pinner
  stays pinned.
- A new GObject at a reused address has the same key: `Get` returns the old
  struct until `Pin` overwrites it.
- `InstanceRegistry` has no lock. Call `Pin` and `Get` on the GTK main thread.

### Package `toast`: notification contract

File: `toast/notifier.go`.

A toast belongs to a window's overlay widget, but business logic must not
depend on UI details. The package is 1 interface and no implementation:

```go
type Notifier interface {
    ShowToast(message string)
    ShowErrorToast(message string)
}
```

The consuming application implements it (typically around
`adw.ToastOverlay`) and passes it across package boundaries.

## Key patterns

### No dependencies between packages

Each package can be imported alone: an application can use main-thread
scheduling without GObject registration, or the reverse.

### Pinner plus destroy notify (`gobj`)

`runtime.Pinner` keeps the Go struct in place while C holds its pointer. The
intent is that a GObject destroy notify releases it, so that the Go struct
lives as long as the GObject; today the notify is never set
([#12](https://github.com/frostyard/snowkit/issues/12)), so the struct stays
pinned until the process ends.

### ID registry for callbacks (`gtk`)

The Go closure does not cross the C boundary. Its integer ID goes through the
`uintptr data` argument of `glib.IdleAdd`, and the source function finds the
closure in the map. No `unsafe.Pointer` conversion or `cgo.Handle` is needed.

### puregotk, not cgo

puregotk calls C functions through purego (`dlopen`/`dlsym` at run time); the
build has no cgo:

- no C compiler is needed to build;
- cross-compilation works as for any pure Go module;
- GTK and GLib shared libraries must be present at run time;
- a test that calls GTK or GLib needs those libraries on the host. CI's
  `ubuntu-latest` runners do not install them, and no such test exists today.

## Operational notes

- **Callback slot limit.** puregotk's `glib.IdleAdd` and `SetDataFull` turn
  each Go function pointer into a C callback with `purego.NewCallbackFnPtr`.
  purego has a fixed table of 2000 callbacks on Linux and macOS (`maxCB`), and
  it caches by function pointer. `RunOnMainThread` makes a new `SourceFunc`
  value on every call, and `Pin` a new `DestroyNotify` (made even though GLib
  never stores it), and neither calls
  `glib.UnrefCallback`. So each call uses 1 slot for the life of the process,
  and after about 2000 calls (fewer, because other callbacks share the table)
  purego panics with `purego: the maximum number of callbacks has been
  reached`. Reproduced at `4f8bd38` with 2500 calls and no main loop: the panic
  came at call 2000. Tracked in
  [frostyard/snowkit#10](https://github.com/frostyard/snowkit/issues/10).
- **`Pin` leaks.** Each pinned instance stays pinned and in the map after
  finalization ([#12](https://github.com/frostyard/snowkit/issues/12)).
- **Thread rules.** `RunOnMainThread` is safe from any goroutine. Everything in
  `gobj` runs on the main thread.

## Dependencies

| Dependency | Purpose |
| --- | --- |
| `codeberg.org/puregotk/puregotk` | GTK4, GLib and GObject bindings through purego; snowkit imports `v4/glib` and `v4/gobject` |
| `codeberg.org/puregotk/purego` | (indirect) `dlopen`-based C calls and the callback table |

## Build, test and release

The gate is the Makefile triad ([ADR-0003](../adr/0003-verify-gate-risk-paths-and-corrections.md)):

- `make verify`: credential-free and non-mutating. `go mod tidy -diff`,
  `go vet ./...`, a `gofmt -l` check, the pinned `golangci-lint` (exact
  `mise.toml` version, built with a Go no older than `go.mod`'s), the template
  gate (`node scripts/check-template.mjs` and `node --test test/*.test.mjs`),
  then `go test -count=1 ./...`.
- `make check`: the developer gate. `gofmt -w`, the static checks of `verify`,
  then `go test -v ./...`.
- `make ci`: `verify`, then with `BASE` set the append-only check of
  `.memory/corrections.jsonl` (`scripts/check-corrections.mjs --base`), then
  `go test -race -short ./...`.

The only Go test is `internal/workflowpin/workflowpin_test.go`. It walks
`.github/workflows` and `.github/actions` and fails on any `uses:` that is not
a 40-character commit SHA with a `# v…` comment (local `./` and `docker://`
references are skipped). A reusable workflow (`…/.github/workflows/…@SHA`) can
name the pull request of the pinned commit instead, because frostyard/core has
no release tags: `risk-paths.yml` uses `# core#225`. It fails when it finds no workflow file.

Versions come from `svu` with [.svu.yml](../../.svu.yml) (`v0: true`: no bump
to v1 while the version is v0; `always: true`: a patch bump when no commit
asks for a bump). `make bump` runs `make check`, refuses a dirty tree, tags
`svu next` and pushes the tag. Only the owner tags. There is no release
workflow: the tag is the release, served by the Go module proxy. The first tag
is `v0.1.0`.

## CI

`.github/workflows/ci.yml` runs on pushes and pull requests to `main` and on
`merge_group`. Top-level `permissions: {}`; each job has `contents: read`,
checkout sets `persist-credentials: false`, and each action is SHA-pinned
(ADR-0021).

| Job | What it runs |
| --- | --- |
| Lint | `make lint`, with golangci-lint from `mise.toml`/`mise.lock` (jdx/mise-action) |
| Unit Tests | `make test` |
| Race Detection | `make test-race` |
| Verify | `make ci BASE=<PR or merge-group base SHA>`, with golangci-lint and Node from mise; full clone for the corrections check |
| Vulnerability Check | `govulncheck ./...` (v1.6.0, installed with `go install`) |

Every job reads Go from `go.mod` (`go-version-file: go.mod`), now 1.24.6: the
lowest Go 1.24 patch that fixes GO-2025-3956 (`os/exec`) and GO-2025-3750
(`os`), which govulncheck reported as reachable at 1.24.2. `setup-go` sets
`GOTOOLCHAIN=local`. govulncheck v1.6.0 needs Go 1.25 to build, so its install
step sets `GOTOOLCHAIN=auto` and `go install` can fetch a newer toolchain for
the build; the scan step keeps `GOTOOLCHAIN=local`, so it checks the standard
library of the `go.mod` version.

`.github/workflows/risk-paths.yml` is the caller of frostyard/core's PR triage
workflow (`pr-triage.yml`, pinned by SHA) on `pull_request_target`. It adds 1
level label, `level:L1` to `level:L4`, from the strictest level of the changed
paths in `.github/risk-paths.json` at the base, and `risk:high` at L1 or L2.
It also removes `ready-for-human` when the head moves after the label, or when
the label comes while CI at the head is not green, and writes a comment that
says why (core ADR-0077, ADR-0078; ADR-0068 keeps `risk:high` as the alias of
L1 and L2). Its job id is `pr-triage`, and it is not a required check.

Dependabot (`.github/dependabot.yml`) updates `gomod` and `github-actions`
weekly.

## Configuration

None. The library has no configuration files and reads no environment
variables; the caller configures everything through the API.

## References

- Rationale: [ADR-0001](../adr/0001-record-architecture-decisions.md),
  [ADR-0002](../adr/0002-agent-portable-instruction-surface.md),
  [ADR-0003](../adr/0003-verify-gate-risk-paths-and-corrections.md),
  [org-wide decisions](../org-adrs.md)
- Contracts: the exported API on
  [pkg.go.dev](https://pkg.go.dev/github.com/frostyard/snowkit)
- Built in: no plan yet; the code predates `docs/plans/`
- Index: [docs/README.md](../README.md)
