package main

import "unsafe"

// comPointer turns an address returned by a COM or WinRT call into an
// unsafe.Pointer. That memory belongs to the OS (an audio engine buffer, a vtable,
// an HSTRING buffer, a CoTaskMem block), not to the Go heap, so the garbage
// collector neither moves nor frees it; the caller must only use the result while
// the owning COM object or string is still alive. Loading the address through a
// pointer-sized slot is the usual way to say this without go vet reporting a
// "possible misuse" for every such call, which would hide real misuse.
//
// Never store the address of a Go object in a uintptr to use later: convert it at
// the call site, e.g. comCall(p, slot, uintptr(unsafe.Pointer(&x))).
func comPointer(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}
