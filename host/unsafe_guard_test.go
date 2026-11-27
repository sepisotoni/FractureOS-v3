package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// A Go object's address stored in a uintptr variable is not kept alive and is not
// updated if the stack moves, so it can dangle before it is used. Convert at the
// call site instead. This scans the Windows sources, which cannot be run here.
func TestNoGoPointerStoredInUintptr(t *testing.T) {
	stored := regexp.MustCompile(`(?m)^\s*[A-Za-z_][A-Za-z0-9_]*\s*:?=\s*uintptr\(unsafe\.Pointer\(`)
	files, _ := filepath.Glob("*_windows.go")
	if len(files) == 0 {
		t.Skip("no windows sources")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, loc := range stored.FindAllIndex(src, -1) {
			// Allowed when a runtime.KeepAlive follows shortly: the object is then
			// pinned until after the call that uses the address.
			end := loc[1] + 500
			if end > len(src) {
				end = len(src)
			}
			if regexp.MustCompile(`runtime\.KeepAlive\(`).Match(src[loc[1]:end]) {
				continue
			}
			line := 1 + len(regexp.MustCompile("\n").FindAllIndex(src[:loc[0]], -1))
			t.Errorf("%s:%d stores uintptr(unsafe.Pointer(...)) in a variable with no runtime.KeepAlive after it; keep an unsafe.Pointer and convert at the call", f, line)
		}
	}
}
