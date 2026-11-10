package gtk

import (
	"runtime"
	"testing"

	"codeberg.org/puregotk/puregotk/v4/glib"
)

// TestRunOnMainThreadManyCalls fails when each RunOnMainThread call takes a new
// purego callback slot: purego has 2000 slots on Linux, so call 2000 panics
// (frostyard/snowkit#10). 2500 calls must all run instead.
func TestRunOnMainThreadManyCalls(t *testing.T) {
	// The default main context is iterated on this thread, so the callbacks run
	// here, as they do on the GTK main thread in an application.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const calls = 2500
	var ran int
	for i := 0; i < calls; i++ {
		RunOnMainThread(func() { ran++ })
	}

	context := glib.MainContextDefault()
	// A bounded loop: 1 idle source is dispatched per call at most, and no
	// sleeps or wall clock are needed.
	for i := 0; i < 10*calls && ran < calls; i++ {
		context.Iteration(false)
	}

	if ran != calls {
		t.Fatalf("%d callbacks ran, want %d", ran, calls)
	}

	idleCallbackMu.Lock()
	pending := len(idleCallbacks)
	idleCallbackMu.Unlock()
	if pending != 0 {
		t.Fatalf("%d callbacks are still in idleCallbacks, want 0", pending)
	}
}

// TestRunOnMainThreadNested checks that a callback can schedule another
// callback: the source function holds no lock while the user's function runs.
func TestRunOnMainThreadNested(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var order []string
	RunOnMainThread(func() {
		order = append(order, "outer")
		RunOnMainThread(func() {
			order = append(order, "inner")
		})
	})

	context := glib.MainContextDefault()
	for i := 0; i < 100 && len(order) < 2; i++ {
		context.Iteration(false)
	}

	if len(order) != 2 || order[0] != "outer" || order[1] != "inner" {
		t.Fatalf("callbacks ran in order %v, want [outer inner]", order)
	}
}
