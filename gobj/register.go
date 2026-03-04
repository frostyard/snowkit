// Package gobj provides helpers for registering GObject subtypes in Go with puregotk.
package gobj

import (
	"runtime"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
)

// TypeDef describes a GObject subtype to register.
type TypeDef struct {
	// ParentGLibType is the parent's GLib type (e.g., adw.ApplicationGLibType()).
	ParentGLibType func() gobject.Type
	// ClassName is the GObject class name (e.g., "FrostySetupApplication").
	ClassName string
	// ClassInit is called during class initialization to override virtual methods.
	ClassInit func(tc *gobject.TypeClass, registry *InstanceRegistry)
}

// InstanceRegistry maps GObject pointers to Go struct pointers.
type InstanceRegistry struct {
	instances map[uintptr]unsafe.Pointer
}

func newRegistry() *InstanceRegistry {
	return &InstanceRegistry{instances: make(map[uintptr]unsafe.Pointer)}
}

// Pin registers a Go struct pointer with the GObject, pins it to prevent GC,
// and sets up cleanup when the GObject is destroyed.
func (r *InstanceRegistry) Pin(o *gobject.Object, goInstance unsafe.Pointer) {
	var pinner runtime.Pinner
	pinner.Pin(goInstance)
	ptr := o.GoPointer()
	r.instances[ptr] = goInstance

	var cleanup glib.DestroyNotify = func(data uintptr) {
		delete(r.instances, ptr)
		pinner.Unpin()
	}
	o.SetDataFull("prevent_gc", 0, &cleanup)
}

// Get retrieves the Go struct pointer for a GObject pointer.
func (r *InstanceRegistry) Get(goPointer uintptr) unsafe.Pointer {
	return r.instances[goPointer]
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
