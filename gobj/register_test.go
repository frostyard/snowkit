package gobj

import (
	"sync"
	"testing"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
)

// newTestObject returns a plain GObject: 1 reference, no properties.
func newTestObject(t *testing.T) *gobject.Object {
	t.Helper()
	obj := gobject.NewObjectWithProperties(gobject.ObjectGLibType(), 0, nil, nil)
	if obj == nil {
		t.Fatal("gobject.NewObjectWithProperties returned nil")
	}
	return obj
}

// pinCount reports the number of live pin entries.
func pinCount() int {
	pinsMu.Lock()
	defer pinsMu.Unlock()
	return len(pins)
}

// TestPinDestroyNotifySlotMadeAtInit checks that init took the callback slot of
// pinDestroyNotify before any concurrent use: a later call must return the same
// address. purego is not atomic between its cache check and its slot
// allocation, so the first call must not come from 2 goroutines.
func TestPinDestroyNotifySlotMadeAtInit(t *testing.T) {
	if pinDestroyNotifyAddr == 0 {
		t.Fatal("pinDestroyNotifyAddr is 0: init did not take the callback slot")
	}
	if got := glib.NewCallback(&pinDestroyNotify); got != pinDestroyNotifyAddr {
		t.Fatalf("glib.NewCallback returned %#x, want the init slot %#x", got, pinDestroyNotifyAddr)
	}
}

// TestPinCleanupOnFinalize fails when the destroy notify never runs, which is
// what a NULL data argument to SetDataFull causes (frostyard/snowkit#12).
func TestPinCleanupOnFinalize(t *testing.T) {
	registry := newRegistry()
	obj := newTestObject(t)
	key := obj.GoPointer()
	instance := new(struct{ n int })
	instance.n = 7

	registry.Pin(obj, unsafe.Pointer(instance))
	if got := registry.Get(key); got != unsafe.Pointer(instance) {
		t.Fatalf("Get after Pin returned %v, want the pinned struct pointer", got)
	}

	obj.Unref()

	if got := registry.Get(key); got != nil {
		t.Fatalf("Get after Unref returned %v, want nil", got)
	}
	if n := pinCount(); n != 0 {
		t.Fatalf("%d pin entries are left after finalization, want 0", n)
	}
}

// TestPinTwiceSameObject checks that a second Pin on the same object releases
// the first Go struct: GLib calls the old destroy notify when the same key is
// set again.
func TestPinTwiceSameObject(t *testing.T) {
	registry := newRegistry()
	obj := newTestObject(t)
	key := obj.GoPointer()
	first := new(struct{ n int })
	second := new(struct{ n int })

	registry.Pin(obj, unsafe.Pointer(first))
	registry.Pin(obj, unsafe.Pointer(second))

	if got := registry.Get(key); got != unsafe.Pointer(second) {
		t.Fatalf("Get returned %v, want the second pinned struct pointer", got)
	}
	if n := pinCount(); n != 1 {
		t.Fatalf("%d pin entries are live after 2 Pins on 1 object, want 1", n)
	}

	obj.Unref()

	if got := registry.Get(key); got != nil {
		t.Fatalf("Get after Unref returned %v, want nil", got)
	}
	if n := pinCount(); n != 0 {
		t.Fatalf("%d pin entries are left after finalization, want 0", n)
	}
}

// TestPinManyObjects fails when each Pin takes a new purego callback slot:
// after about 2000 Pins purego panics (frostyard/snowkit#10).
func TestPinManyObjects(t *testing.T) {
	registry := newRegistry()

	const objects = 2500
	for i := 0; i < objects; i++ {
		obj := newTestObject(t)
		registry.Pin(obj, unsafe.Pointer(new(struct{ n int })))
		obj.Unref()
	}

	if n := pinCount(); n != 0 {
		t.Fatalf("%d pin entries are left after %d finalized objects, want 0", n, objects)
	}
}

// GLib registers a type name once per process, so the test type is
// registered once, also when the tests run with -count greater than 1.
var (
	testTypeOnce     sync.Once
	testType         gobject.Type
	testTypeRegistry *InstanceRegistry
)

// TestRegisterTypePin checks the same lifecycle through a registered subtype.
func TestRegisterTypePin(t *testing.T) {
	testTypeOnce.Do(func() {
		testType, testTypeRegistry = RegisterType(TypeDef{
			ParentGLibType: gobject.ObjectGLibType,
			ClassName:      "SnowkitRegisterTestObject",
			ClassInit:      func(tc *gobject.TypeClass, r *InstanceRegistry) {},
		})
	})
	gType, registry := testType, testTypeRegistry
	if gType == 0 {
		t.Fatal("RegisterType returned the invalid type 0")
	}

	obj := gobject.NewObjectWithProperties(gType, 0, nil, nil)
	if obj == nil {
		t.Fatal("gobject.NewObjectWithProperties returned nil for the registered type")
	}
	key := obj.GoPointer()
	instance := new(struct{ n int })

	registry.Pin(obj, unsafe.Pointer(instance))
	if got := registry.Get(key); got != unsafe.Pointer(instance) {
		t.Fatalf("Get after Pin returned %v, want the pinned struct pointer", got)
	}

	obj.Unref()

	if got := registry.Get(key); got != nil {
		t.Fatalf("Get after Unref returned %v, want nil", got)
	}
	if n := pinCount(); n != 0 {
		t.Fatalf("%d pin entries are left after finalization, want 0", n)
	}
}
