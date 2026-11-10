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
├── internal/workflowpin/       workflowpin_test.go: the action-pinning test (ADR-0021)
├── gtk/, gobj/                 mainthread_test.go, register_test.go: the GLib tests (need libglib and libgobject on the host)
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
   `glib.IdleAdd(&idleSourceFunc, id)`. `idleSourceFunc` is a package-level
   `glib.SourceFunc` that looks up the callback by its `data` argument, deletes
   it from the map, releases the lock and then calls it.

Invariant: the mutex is held only to read or write the map, never while the
user's callback runs, so a callback can call `RunOnMainThread` again without a
deadlock.

Invariant: the package has exactly 1 `glib.SourceFunc`, so every call shares 1
purego callback slot. purego keeps a slot per function-variable ADDRESS, and a
function value declared inside `RunOnMainThread` had a new address on every
call: 1 slot of purego's 2000 per call, never freed, so a process panicked at
call 2000 ([#10](https://github.com/frostyard/snowkit/issues/10), fixed).

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
  (`unsafe.Pointer`) through a `*pinEntry`:
  - `Pin(o *gobject.Object, goInstance unsafe.Pointer)` stores the pair, pins
    the Go pointer, and gives GLib an ID that releases it when the GObject is
    finalized (see below).
  - `Get(goPointer uintptr) unsafe.Pointer` returns the Go struct for a GObject
    pointer, or `nil` after finalization or after a later `Pin` replaced the
    entry.

Lifecycle: "pin while the GObject lives, unpin when it is finalized". The
cleanup did not run before this change
([#12](https://github.com/frostyard/snowkit/issues/12), fixed):

1. **Registration.** `RegisterType` reads the parent's class and instance sizes
   with `gobject.NewTypeQuery`, then calls `gobject.TypeRegisterStaticSimple`
   with the class-init callback and a no-op instance-init callback.
2. **Pinning.** The consumer calls `registry.Pin(obj, goPtr)` when it builds an
   instance (chairlift does it in its `constructed` override). `Pin`:
   - builds a `*pinEntry` (registry, object key, Go pointer) and pins `goPtr`
     with the entry's `runtime.Pinner`, so the collector does not move or free
     it;
   - stores the entry in the package table `pins` under a new non-zero ID, and
     in `instances[obj.GoPointer()]`;
   - calls `SetDataFull("prevent_gc", id, &pinDestroyNotify)`, where
     `pinDestroyNotify` is the package's only `glib.DestroyNotify`. The data is
     non-NULL, so GLib stores it and calls the notify. Both locks are released
     before this call: `SetDataFull` can call the previous notify of the key
     synchronously.
3. **Cleanup.** GLib calls `pinDestroyNotify(id)` when it finalizes the object,
   and also when `Pin` sets the same key again on the object (then the new data
   is stored first). The notify deletes `pins[id]`, deletes
   `instances[entry.key]` only if that value is still this same entry, and
   calls `entry.pinner.Unpin()`.

Behavior now (checked with the tests in `gobj/register_test.go`, which need
GLib on the host):

- After the GObject is finalized, `Get` returns nil, the entry leaves both
  maps, and the Go struct is unpinned.
- A second `Pin` on the same object releases the first Go pointer at once; `Get`
  returns the second one and the table holds 1 entry.
- A new GObject at a reused address gets a new ID: the old entry is already
  gone, so the key cannot answer for the old struct.
- `InstanceRegistry` and `pins` each have a mutex, because finalization can run
  on another thread. `Pin` and `Get` stay safe to call on the GTK main thread;
  a notify on its own thread takes the locks for a map operation only.

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

`runtime.Pinner` keeps the Go struct in place while C holds its pointer. A
GObject destroy notify releases it, so that the Go struct lives as long as the
GObject: `Pin` sets a non-zero ID as the data of the `"prevent_gc"` key with 1
package-level notify, and the notify unpins the entry when GLib finalizes the
object or when a second `Pin` replaces the key
([#12](https://github.com/frostyard/snowkit/issues/12), fixed).

### ID registry for callbacks (`gtk`)

The Go closure does not cross the C boundary. Its integer ID goes through the
`uintptr data` argument of `glib.IdleAdd`, and the package-level source
function finds the closure in the map. No `unsafe.Pointer` conversion or
`cgo.Handle` is needed, and the ID keeps 1 purego callback slot for the whole
process.

### puregotk, not cgo

puregotk calls C functions through purego (`dlopen`/`dlsym` at run time); the
build has no cgo:

- no C compiler is needed to build;
- cross-compilation works as for any pure Go module;
- GTK and GLib shared libraries must be present at run time;
- a test that calls GTK or GLib needs those libraries on the host. The `gtk`
  and `gobj` tests import only `glib` and `gobject`, so they load
  `libglib-2.0.so.0` and `libgobject-2.0.so.0` (CI's `ubuntu-latest` has both)
  and no GTK library.

## Operational notes

- **Callback slots.** puregotk's `glib.IdleAdd` and `Object.SetDataFull` turn a
  Go function pointer into a C callback with `purego.NewCallbackFnPtr`. purego
  has a fixed table of 2000 callbacks on Linux and macOS (`maxCB`), and it
  caches by the ADDRESS of the function variable. `gtk` passes the
  package-level `idleSourceFunc` to every `glib.IdleAdd`, and `gobj` the
  package-level `pinDestroyNotify` to every `SetDataFull`, so each package uses
  1 slot for the life of the process, whatever the number of calls. A function
  value declared inside the call had a new address per call, 1 slot each, and
  purego panicked at call 2000 with `purego: the maximum number of callbacks
  has been reached`; reproduced at `4f8bd38` and fixed
  ([frostyard/snowkit#10](https://github.com/frostyard/snowkit/issues/10)).
  `gtk/mainthread_test.go` (2500 calls) and `gobj/register_test.go` (2500
  pinned objects) guard it.
- **`Pin` cleanup.** `Pin` no longer leaks: the entry and the pin go away when
  the GObject is finalized, or when a second `Pin` replaces the entry
  ([#12](https://github.com/frostyard/snowkit/issues/12), fixed;
  `gobj/register_test.go`).
- **Thread rules.** `RunOnMainThread` is safe from any goroutine. `gobj` is
  safe from any thread: `InstanceRegistry` and the `pins` table have a mutex,
  and no lock is held across a GLib call.

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

The Go tests are `internal/workflowpin/workflowpin_test.go`, `gtk/mainthread_test.go`
and `gobj/register_test.go`.

`internal/workflowpin` walks
`.github/workflows` and `.github/actions` and fails on any `uses:` that is not
a 40-character commit SHA with a `# v…` comment (local `./` and `docker://`
references are skipped). A reusable workflow (`…/.github/workflows/…@SHA`) can
name the pull request of the pinned commit instead, because frostyard/core has
no release tags: `risk-paths.yml` uses `# core#225`. It fails when it finds no workflow file.

`gtk/mainthread_test.go` adds 2500 idle sources through `RunOnMainThread` and
iterates the default main context; `gobj/register_test.go` pins and finalizes
plain `GObject`s. Both load GLib and GObject only (no GTK), so they run
wherever `libglib-2.0.so.0` and `libgobject-2.0.so.0` exist, CI included. They
are the guards of [#10](https://github.com/frostyard/snowkit/issues/10) and
[#12](https://github.com/frostyard/snowkit/issues/12).

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
