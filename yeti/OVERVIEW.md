# Snowkit Overview

## Purpose

Snowkit is a Go utility library for building GTK/GNOME applications with [puregotk](https://codeberg.org/nicegotk/puregotk). It solves three recurring problems in puregotk applications: scheduling work on GTK's main thread from goroutines, registering custom GObject subtypes with proper Go/C lifecycle management, and providing a decoupled toast notification contract. The module path is `github.com/frostyard/snowkit`.

## Architecture

Three independent packages with zero interdependencies. All depend on puregotk's `gobject`/`glib` bindings.

```
snowkit/
├── gtk/          # Thread-safe main-loop scheduling
│   └── mainthread.go
├── gobj/         # GObject subtype registration + Go↔C pointer lifecycle
│   └── register.go
├── toast/        # Toast notification interface (pure contract)
│   └── notifier.go
├── go.mod        # Module: github.com/frostyard/snowkit (Go 1.24.2)
├── Makefile      # Build/lint/test/release targets
├── CLAUDE.md     # AI assistant instructions
└── yeti/         # AI-optimized documentation (this directory)
```

### Package: `gtk` — Main-Thread Scheduling

**File:** `gtk/mainthread.go`

**Problem:** GTK is not thread-safe. Any GTK API call must happen on the main thread, but Go applications use goroutines freely.

**Solution:** `RunOnMainThread(fn func())` schedules a closure on the GTK main loop via `glib.IdleAdd`. The callback runs exactly once, then the idle source is removed (returns `false`).

**How it works:**
1. A package-level map (`idleCallbacks`) stores pending callbacks keyed by incrementing `uintptr` IDs.
2. A `sync.Mutex` protects both the map and the ID counter.
3. `RunOnMainThread` assigns an ID, stores the callback, then calls `glib.IdleAdd` with a `SourceFunc` wrapper that looks up and deletes the callback by ID.
4. The `SourceFunc` receives the ID as its `data uintptr` parameter — this is how the GTK callback finds the right Go closure.

**Key invariant:** The mutex is held only briefly (to read/write the map), never while executing the user's callback. This prevents deadlocks if the callback itself calls `RunOnMainThread`.

**Typical usage by consumers:**
```go
go func() {
    result := expensiveWork()
    gtk.RunOnMainThread(func() {
        label.SetText(result) // safe: runs on main thread
    })
}()
```

### Package: `gobj` — GObject Type Registration

**File:** `gobj/register.go`

**Problem:** puregotk lets you use existing GObject types from Go, but registering *new* GObject subtypes (needed for custom application classes, widgets, etc.) requires manual C-level registration and careful lifecycle management to prevent Go's GC from collecting objects the C side still references.

**Solution:** `RegisterType(TypeDef)` registers a GObject subtype and returns its GLib type ID plus an `InstanceRegistry` for managing Go↔C pointer associations.

**Core types:**

- **`TypeDef`** — Describes a subtype to register:
  - `ParentGLibType func() gobject.Type` — parent type accessor (e.g., `adw.ApplicationGLibType`)
  - `ClassName string` — GObject class name (e.g., `"FrostySetupApplication"`)
  - `ClassInit func(tc *gobject.TypeClass, registry *InstanceRegistry)` — called during class initialization, used to override virtual methods

- **`InstanceRegistry`** — Maps GObject C pointers (`uintptr`) to Go struct pointers (`unsafe.Pointer`):
  - `Pin(o *gobject.Object, goInstance unsafe.Pointer)` — registers the association, pins the Go pointer to prevent GC, and sets up destroy-notify cleanup
  - `Get(goPointer uintptr) unsafe.Pointer` — retrieves the Go struct for a GObject pointer

**Lifecycle (critical to understand for changes):**

1. **Registration:** `RegisterType` calls `gobject.TypeRegisterStaticSimple` with the parent type's class/instance sizes and the class init callback.
2. **Pinning:** When a new instance is created, the consumer calls `registry.Pin(obj, goPtr)`. This:
   - Creates a `runtime.Pinner` and pins the Go pointer (prevents GC relocation/collection)
   - Stores the mapping in `instances[obj.GoPointer()] = goPtr`
   - Attaches a `glib.DestroyNotify` via `SetDataFull("prevent_gc", ...)` that will clean up when the GObject is destroyed
3. **Cleanup:** When the GObject's ref count hits zero and it's finalized, GLib calls the destroy notify, which:
   - Deletes the entry from the instances map
   - Calls `pinner.Unpin()` to release the Go pointer back to GC

**Key invariant:** Every `Pin` must eventually be balanced by a GObject destruction. Leaking a GObject leaks both the C object and the pinned Go struct. This is by design — the Go struct's lifetime is tied to the GObject's lifetime.

**Note:** The `InstanceRegistry` is NOT thread-safe. Registration and pinning are expected to happen on the GTK main thread. If multi-threaded access becomes necessary, a mutex would need to be added.

### Package: `toast` — Notification Interface

**File:** `toast/notifier.go`

**Problem:** Toast notifications are tied to a specific window/overlay widget, but business logic should not depend on UI details.

**Solution:** A pure interface with no implementation:
```go
type Notifier interface {
    ShowToast(message string)
    ShowErrorToast(message string)
}
```

Consuming applications provide their own implementation (typically wrapping `adw.ToastOverlay`). This package exists to define the contract so it can be passed across package boundaries without importing UI code.

## Key Patterns

### No Interdependencies Between Packages
Each package can be imported independently. This is intentional — consuming applications may only need main-thread scheduling without GObject registration, or vice versa.

### C Pointer ↔ Go Struct Bridging (gobj)
The pattern of using `runtime.Pinner` + GObject destroy notifications is the canonical way to prevent Go GC from collecting objects that are still referenced by C code. This replaces older patterns like `cgo.Handle` or manual prevent-gc maps.

### Callback Registry Pattern (gtk)
Rather than using `cgo.Handle` or `unsafe.Pointer` to pass Go closures through C callbacks, snowkit uses a map-based registry with integer IDs. The ID is passed as the `uintptr data` parameter through the C callback boundary, then used to look up the actual Go closure. This avoids unsafe pointer conversions entirely.

### puregotk (Not cgo)
puregotk uses `purego` to call C functions via `dlopen`/`dlsym` at runtime — there is no cgo involved. This means:
- No C compiler needed to build
- Cross-compilation works normally
- But GTK shared libraries must be present at runtime
- Tests that exercise GTK functionality need GTK libraries on the host

## Dependencies

| Dependency | Purpose |
|---|---|
| `codeberg.org/puregotk/puregotk` | Go bindings for GTK4/GLib/GObject via purego |
| `codeberg.org/puregotk/purego` | (indirect) dlopen-based C function calling |

## Build & Development

Primary dev loop: `make check` (formats, lints, tests).

There are no tests yet. The library depends on puregotk which loads GTK via dlopen at runtime, so tests exercising GTK functionality need GTK libraries on the host.

Versioning uses `svu` (semantic version utility): `make bump` runs checks, tags next semver, and pushes the tag. Requires a clean working tree. Configured via `.svu.yaml` with `v0: true` (stays in 0.x range) and `always: true` (always bumps even without conventional commits).

## CI Pipeline

GitHub Actions (`.github/workflows/ci.yml`) runs on push/PR to `main` with four parallel jobs:

| Job | What it does |
|---|---|
| **Lint** | `golangci-lint` via the official action |
| **Unit Tests** | `go test -v ./...` |
| **Race Detection** | `go test -race -short ./...` |
| **Verify** | `go mod tidy` (checks for drift), `go vet`, `gofmt -l` (checks formatting) |

All jobs use Go 1.24 on `ubuntu-latest`. Note: CI tests will pass vacuously until actual test files are added, since there are no `_test.go` files yet.

Dependabot is configured (`.github/dependabot.yml`) to monitor `gomod` and `github-actions` updates.

## Configuration

No configuration files or environment variables. The library is stateless and configured entirely through its API at call sites.
