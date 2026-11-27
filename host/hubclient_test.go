package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newCountingHub(t *testing.T) (Config, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("OK")) }))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(srv.Certificate().Raw)
	resetSharedHubTransport()
	t.Cleanup(resetSharedHubTransport)
	return Config{Hub: srv.URL, Token: "tok", Fingerprint: hex.EncodeToString(sum[:])}, &conns
}

func TestHubRequestReusesOneConnection(t *testing.T) {
	c, conns := newCountingHub(t)
	for i := 0; i < 20; i++ {
		if _, err := hubRequest(c, "/api/command", map[string]string{"command": "status"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := conns.Load(); n != 1 {
		t.Fatalf("20 sequential requests used %d TLS connections, want 1", n)
	}
}

func TestSharedHubTransportKeyChangeReplacesTransport(t *testing.T) {
	c, conns := newCountingHub(t)
	a, err := sharedHubTransport(c)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := sharedHubTransport(c); b != a {
		t.Fatal("same key must return the cached transport")
	}
	if _, err := hubRequest(c, "/api/command", map[string]string{"command": "status"}); err != nil {
		t.Fatal(err)
	}
	c2 := c
	c2.Token = "other"
	b, _ := sharedHubTransport(c2)
	if b == a {
		t.Fatal("a new token must create a new transport")
	}
	if _, err := hubRequest(c2, "/api/command", map[string]string{"command": "status"}); err != nil {
		t.Fatal(err)
	}
	if n := conns.Load(); n != 2 {
		t.Fatalf("connections = %d, want 2 (one per key)", n)
	}
	c3 := c2
	c3.Fingerprint = "00" + c.Fingerprint[2:]
	if _, err := hubRequest(c3, "/api/command", map[string]string{"command": "status"}); err == nil {
		t.Fatal("a wrong fingerprint must be refused, not served from the cached connection")
	}
}

func TestSharedHubTransportExpires(t *testing.T) {
	c, _ := newCountingHub(t)
	now := time.Now()
	sharedHubClock = func() time.Time { return now }
	defer func() { sharedHubClock = time.Now }()
	a, _ := sharedHubTransport(c)
	now = now.Add(sharedHubMaxAge - time.Second)
	if b, _ := sharedHubTransport(c); b != a {
		t.Fatal("transport replaced too early")
	}
	now = now.Add(2 * time.Second)
	if b, _ := sharedHubTransport(c); b == a {
		t.Fatal("transport older than max age must be replaced")
	}
}

func TestSharedHubTransportRejectsUnconfigured(t *testing.T) {
	resetSharedHubTransport()
	if _, err := sharedHubTransport(Config{Hub: "http://x", Token: "t", Fingerprint: "f"}); err == nil {
		t.Fatal("plain http must be refused")
	}
	if _, err := sharedHubTransport(Config{Hub: "https://x"}); err == nil {
		t.Fatal("missing token/fingerprint must be refused")
	}
}

func TestHubRequestConcurrent(t *testing.T) {
	c, _ := newCountingHub(t)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if _, err := hubRequest(c, "/api/command", map[string]string{"command": "status"}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
