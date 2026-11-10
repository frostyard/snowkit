# snowkit

Go utilities for GTK4 and GNOME applications built with
[puregotk](https://codeberg.org/puregotk/puregotk).

```sh
go get github.com/frostyard/snowkit
```

| Package | What it gives you |
| --- | --- |
| `gtk` | `RunOnMainThread(fn)`: run a closure on the GTK main loop from any goroutine |
| `gobj` | `RegisterType(TypeDef)`: register a GObject subtype, with an `InstanceRegistry` that maps GObject pointers to pinned Go structs |
| `toast` | `Notifier`: a toast interface that the application implements |

API reference: [pkg.go.dev/github.com/frostyard/snowkit](https://pkg.go.dev/github.com/frostyard/snowkit).
Design and invariants: [docs/design/overview.md](docs/design/overview.md).
Contributor and agent rules: [AGENTS.md](AGENTS.md).

puregotk loads GTK with `dlopen` at run time, so building needs no C compiler,
but the GTK and GLib shared libraries must be installed where the application
runs.

## Callback slots and the Go lifecycle

purego keeps 1 callback slot per function address and has 2000 per process on
Linux and macOS. Both packages hand puregotk 1 package-level function, so
`gtk` and `gobj` use 1 slot each for the whole process, however many
`RunOnMainThread` calls or `Pin` calls a program makes.

`InstanceRegistry.Pin` pins the Go struct and the GObject releases it: GLib
calls the package destroy notify when it finalizes the object, or when `Pin`
sets the same key again, and the notify deletes the registry entry, unpins, and
then `Get` returns nil. History: [#10](https://github.com/frostyard/snowkit/issues/10)
(the slot limit) and [#12](https://github.com/frostyard/snowkit/issues/12) (the
cleanup that never ran) are both fixed.

## License

[MIT](LICENSE)
