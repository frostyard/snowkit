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

// RunOnMainThread schedules fn to run on the GTK main thread via glib.IdleAdd.
// Safe to call from any goroutine. fn runs exactly once, then the idle source is removed.
func RunOnMainThread(fn func()) {
	idleCallbackMu.Lock()
	idleCallbackID++
	id := idleCallbackID
	idleCallbacks[id] = fn
	idleCallbackMu.Unlock()

	cb := glib.SourceFunc(func(data uintptr) bool {
		idleCallbackMu.Lock()
		callback, ok := idleCallbacks[data]
		delete(idleCallbacks, data)
		idleCallbackMu.Unlock()

		if ok {
			callback()
		}
		return false
	})
	glib.IdleAdd(&cb, id)
}
