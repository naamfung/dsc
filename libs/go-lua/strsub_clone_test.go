package lua

import (
	"runtime"
	"testing"
	"unsafe"
)

func strDataPtr(s string) uintptr {
	if len(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.StringData(s)))
}

// string.sub must return an independent copy, not a slice that aliases the
// source string's backing array.
func TestStrSubDoesNotAliasSource(t *testing.T) {
	L := NewState()
	defer L.Close()

	if err := L.DoString(`big = string.rep("x", 100000000); sub = string.sub(big, 50, 60)`); err != nil {
		t.Fatal(err)
	}
	big := string(L.GetGlobal("big").(LString))
	sub := string(L.GetGlobal("sub").(LString))

	bp, sp := strDataPtr(big), strDataPtr(sub)
	if sp >= bp && sp < bp+uintptr(len(big)) {
		t.Fatalf("string.sub result aliases the source backing array at offset %d", sp-bp)
	}
}

// A small string.sub result must not pin a large source string through GC.
func TestStrSubDoesNotPinSource(t *testing.T) {
	L := NewState()
	defer L.Close()

	var m0, m1 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m0)

	if err := L.DoString(`big = string.rep("y", 200000000); keep = string.sub(big, 1, 8); big = nil`); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&m1)

	heldMB := (float64(m1.HeapAlloc) - float64(m0.HeapAlloc)) / 1e6
	if heldMB > 100 {
		t.Fatalf("8-byte string.sub pinned %.0fMB of its source through GC", heldMB)
	}
	runtime.KeepAlive(L)
}
