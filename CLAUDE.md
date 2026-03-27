# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make check        # fmt + lint + test (the main dev loop command)
make test         # go test -v ./...
make lint         # golangci-lint run (skips if not installed)
make fmt          # gofmt -w on all .go files
make tidy         # go mod tidy
make test-cover   # generate coverage.out and coverage.html
make bump         # tag next semver via svu, push tag (requires clean tree)
```

There are no tests yet. The library depends on puregotk which loads GTK via dlopen at runtime, so tests that exercise GTK functionality need GTK libraries available on the host.

## Architecture

Snowkit is a Go utility library for building GTK/GNOME applications with [puregotk](https://codeberg.org/nicegotk/puregotk). It provides three independent packages:

- **`gtk`** — `RunOnMainThread(fn)` schedules a callback on the GTK main loop via `glib.IdleAdd`. Uses a mutex-protected map registry with incrementing IDs to safely queue work from any goroutine.

- **`gobj`** — `RegisterType(TypeDef)` registers custom GObject subtypes. `InstanceRegistry` maps GObject C pointers to Go struct pointers, using `runtime.Pinner` to prevent GC and GObject destroy notifications for cleanup. This is the most complex package — changes here must preserve the prevent-GC-via-Pin / cleanup-via-destroy-notify lifecycle.

- **`toast`** — `Notifier` interface (`ShowToast`, `ShowErrorToast`). Pure contract, no implementation — consuming applications provide their own.

The packages have no interdependencies. All three depend on puregotk's `gobject`/`glib` bindings.
