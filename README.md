# snowkit

Go utilities for GTK4 and GNOME applications built with
[puregotk](https://codeberg.org/puregotk/puregotk).

```sh
go get github.com/frostyard/snowkit
```

| Package | What it gives you |
| --- | --- |
| `gtk` | `RunOnMainThread(fn)`: run a closure on the GTK main loop from any goroutine |
| `gobj` | `RegisterType(TypeDef)`: register a GObject subtype, with an `InstanceRegistry` that keeps the Go struct alive as long as the GObject |
| `toast` | `Notifier`: a toast interface that the application implements |

API reference: [pkg.go.dev/github.com/frostyard/snowkit](https://pkg.go.dev/github.com/frostyard/snowkit).
Design and invariants: [docs/design/overview.md](docs/design/overview.md).
Contributor and agent rules: [AGENTS.md](AGENTS.md).

puregotk loads GTK with `dlopen` at run time, so building needs no C compiler,
but the GTK and GLib shared libraries must be installed where the application
runs.

## Known limit

Each `RunOnMainThread` call and each `InstanceRegistry.Pin` uses 1 of purego's
2000 callback slots and does not free it, so a process panics after about 2000
calls. See [#10](https://github.com/frostyard/snowkit/issues/10).

## License

[MIT](LICENSE)
