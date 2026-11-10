// Package gobj provides helpers for registering GObject subtypes in Go with puregotk.
package gobj

import (
	"runtime"
	"sync"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
)

// pinDataKey is the GObject data key that carries the ID of a pin entry.
const pinDataKey = "prevent_gc"

// TypeDef describes a GObject subtype to register.
type TypeDef struct {
	// ParentGLibType is the parent's GLib type (e.g., adw.ApplicationGLibType()).
	ParentGLibType func() gobject.Type
	// ClassName is the GObject class name (e.g., "FrostySetupApplication").
	ClassName string
	// ClassInit is called during class initialization to override virtual methods.
	ClassInit func(tc *gobject.TypeClass, registry *InstanceRegistry)
}

// pinEntry is the Go side of 1 pinned Go struct: the pin itself, and the
// registry entry that must go away when the GObject is finalized.
type pinEntry struct {
	registry *InstanceRegistry
	// key is the GObject pointer, also the key in registry.instances.
	key    uintptr
	ptr    unsafe.Pointer
	pinner runtime.Pinner
}

var (
	// pinsMu guards pins and pinID. It is never held across a GLib call.
	pinsMu sync.Mutex
	// pins holds the live pin entries by their non-zero ID. The ID is the data
	// argument of the GObject data key, so the destroy notify finds its entry.
	pins  = make(map[uintptr]*pinEntry)
	pinID uintptr
)

// pinDestroyNotify is the single glib.DestroyNotify this package gives to
// GLib. purego caches 1 callback slot per function-variable ADDRESS, so a
// function value declared inside Pin would take 1 of purego's 2000 slots per
// call and never free it (frostyard/snowkit#10). It runs when GLib finalizes
// the object, or when Pin sets the data key again on the same object, which
// releases the previous entry (frostyard/snowkit#12).
var pinDestroyNotify glib.DestroyNotify = func(data uintptr) {
	releasePin(data)
}

// releasePin drops the pin entry with the given ID: the pins entry, the
// registry entry if it is still this same entry, and the pin itself. It takes
// no GLib call, so it may run on the thread that finalizes the GObject.
func releasePin(id uintptr) {
	pinsMu.Lock()
	entry, ok := pins[id]
	if ok {
		delete(pins, id)
	}
	pinsMu.Unlock()
	if !ok {
		return
	}

	entry.registry.mu.Lock()
	// A second Pin on the same object replaced this entry before GLib called
	// this notify; that newer entry owns the registry key now.
	if entry.registry.instances[entry.key] == entry {
		delete(entry.registry.instances, entry.key)
	}
	entry.registry.mu.Unlock()

	entry.pinner.Unpin()
}

// InstanceRegistry maps GObject pointers to Go struct pointers. Its mutex
// guards the map: finalization, and so the destroy notify, can run on a
// thread other than the GTK main thread.
type InstanceRegistry struct {
	mu        sync.Mutex
	instances map[uintptr]*pinEntry
}

func newRegistry() *InstanceRegistry {
	return &InstanceRegistry{instances: make(map[uintptr]*pinEntry)}
}

// Pin registers a Go struct pointer with the GObject and pins it so the
// collector neither moves nor frees it while C holds its pointer.
//
// The pin lives as long as the GObject: Pin stores a non-zero ID as the data
// of the "prevent_gc" key with 1 package-level destroy notify, and GLib calls
// that notify with the ID when it finalizes the object. The notify then
// deletes the registry entry, drops the pin entry and unpins the Go pointer. A
// second Pin on the same object releases the previous Go pointer the same way:
// GLib calls the old notify when the key is set again.
func (r *InstanceRegistry) Pin(o *gobject.Object, goInstance unsafe.Pointer) {
	entry := &pinEntry{registry: r, key: o.GoPointer(), ptr: goInstance}
	entry.pinner.Pin(goInstance)

	pinsMu.Lock()
	pinID++
	id := pinID
	pins[id] = entry
	pinsMu.Unlock()

	r.mu.Lock()
	r.instances[entry.key] = entry
	r.mu.Unlock()

	// SetDataFull can call the previous destroy notify of this key
	// synchronously, so it runs after both locks are released.
	o.SetDataFull(pinDataKey, id, &pinDestroyNotify)
}

// Get retrieves the Go struct pointer for a GObject pointer. It returns nil
// once the GObject is finalized, or after a later Pin replaced the entry.
func (r *InstanceRegistry) Get(goPointer uintptr) unsafe.Pointer {
	r.mu.Lock()
	entry, ok := r.instances[goPointer]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	return entry.ptr
}

// RegisterType registers a new GObject subtype and returns its GLib type and instance registry.
func RegisterType(def TypeDef) (gobject.Type, *InstanceRegistry) {
	registry := newRegistry()

	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		def.ClassInit(tc, registry)
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(def.ParentGLibType(), &parentQuery)

	gType := gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		def.ClassName,
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)

	return gType, registry
}
