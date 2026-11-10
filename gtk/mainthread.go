// Package gtk provides thread-safe utilities for GTK applications using puregotk.
package gtk

import (
	"sync"

	"codeberg.org/puregotk/puregotk/v4/glib"
)

var (
	idleCallbackMu sync.Mutex
	idleCallbacks  = make(map[uintptr]func())
	idleCallbackID uintptr
)

// idleSourceFunc is the single glib.SourceFunc this package gives to GLib.
//
// purego turns a Go function into a C callback with NewCallbackFnPtr, which
// caches 1 table slot per function-variable ADDRESS. A function value declared
// inside RunOnMainThread would have a new address on every call, so it would
// take 1 of purego's 2000 callback slots per call and never free it
// (frostyard/snowkit#10). This package-level value keeps 1 slot for the life
// of the process.
//
// The body holds idleCallbackMu only to read and delete the callback, never
// while it runs the user's function, so a callback can call RunOnMainThread
// again without a deadlock.
var idleSourceFunc glib.SourceFunc = func(data uintptr) bool {
	idleCallbackMu.Lock()
	callback, ok := idleCallbacks[data]
	delete(idleCallbacks, data)
	idleCallbackMu.Unlock()

	if ok {
		callback()
	}
	return false
}

// RunOnMainThread schedules fn to run on the GTK main thread via glib.IdleAdd.
// Safe to call from any goroutine. fn runs exactly once, then the idle source is removed.
//
// Every call uses the same package-level source function, so the call takes no
// new purego callback slot: the Go closure crosses the C boundary only as its
// uintptr ID in the data argument of glib.IdleAdd.
func RunOnMainThread(fn func()) {
	idleCallbackMu.Lock()
	idleCallbackID++
	id := idleCallbackID
	idleCallbacks[id] = fn
	idleCallbackMu.Unlock()

	glib.IdleAdd(&idleSourceFunc, id)
}
