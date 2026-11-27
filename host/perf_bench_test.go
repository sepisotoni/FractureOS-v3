package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Benchmarks for the idle background cost of the host app. They run on any OS;
// the WinRT calls in the Windows notification poll (notificationAccess,
// windowsNotices) cannot be measured here.

func benchConfigDir(b *testing.B) {
	b.Helper()
	dir := b.TempDir()
	b.Setenv("XDG_CONFIG_HOME", dir)
	b.Setenv("AppData", dir)
	if err := os.MkdirAll(filepath.Join(dir, "fracture-v3"), 0o700); err != nil {
		b.Fatal(err)
	}
	raw := `{"device":"192.168.101.115","token":"t","windows_notifications":true}`
	if err := os.WriteFile(filepath.Join(dir, "fracture-v3", "config.json"), []byte(raw), 0o600); err != nil {
		b.Fatal(err)
	}
}

// Cost of one tick of the 250 ms notification loop that is measurable off Windows.
func BenchmarkReadConfig(b *testing.B) {
	benchConfigDir(b)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := readConfig(); err != nil {
			b.Fatal(err)
		}
	}
}

// Cost of one supervisor hub call: hubRequest builds a new Transport and does a
// full TLS 1.3 handshake every time.
func BenchmarkHubRequestFreshTLS(b *testing.B) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }))
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	c := Config{Hub: srv.URL, Token: "t", Fingerprint: hex.EncodeToString(sum[:])}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		resetSharedHubTransport() // forces a new transport and handshake, the old behaviour
		if _, err := hubRequest(c, "/api/command", map[string]string{"command": "status"}); err != nil {
			b.Fatal(err)
		}
	}
	resetSharedHubTransport()
}

// Same call with the shared keep-alive transport (current behaviour).
func BenchmarkHubRequestShared(b *testing.B) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("{}")) }))
	defer srv.Close()
	sum := sha256.Sum256(srv.Certificate().Raw)
	c := Config{Hub: srv.URL, Token: "t", Fingerprint: hex.EncodeToString(sum[:])}
	resetSharedHubTransport()
	defer resetSharedHubTransport()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := hubRequest(c, "/api/command", map[string]string{"command": "status"}); err != nil {
			b.Fatal(err)
		}
	}
}
