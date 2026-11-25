package main

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestASCIIFallbackPreservesReadableSongText(t *testing.T) {
	tests := map[string]string{
		"Beyoncé — Déjà Vu": "Beyonce - Deja Vu",
		"e\u0301lan":        "elan",
		"東京 Nights":         "Nights",
		"東京":                "[text unavailable]",
	}
	for input, want := range tests {
		encoded := asciiHex(input)
		raw, err := hex.DecodeString(encoded)
		if err != nil {
			t.Fatalf("asciiHex(%q) returned invalid hex: %v", input, err)
		}
		if got := string(raw); got != want {
			t.Errorf("asciiHex(%q) = %q, want %q", input, got, want)
		}
		if strings.ContainsRune(string(raw), '?') {
			t.Errorf("asciiHex(%q) retained replacement question marks: %q", input, raw)
		}
		if !mediaTextValid(encoded) {
			t.Errorf("asciiHex(%q) returned text rejected by device validation", input)
		}
	}
}

func TestASCIIFallbackBoundsDeviceText(t *testing.T) {
	raw, err := hex.DecodeString(asciiHex(strings.Repeat("é", 100)))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 80 || string(raw) != strings.Repeat("e", 80) {
		t.Fatalf("translated text length/content = %d bytes, %q", len(raw), raw)
	}
}

func BenchmarkASCIIFallback(b *testing.B) {
	title := "Beyoncé — Déjà Vu (Live)"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = asciiHex(title)
	}
}
