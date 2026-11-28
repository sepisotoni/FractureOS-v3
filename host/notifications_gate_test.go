package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeNotificationHub struct {
	webhook      atomic.Bool
	device       atomic.Int32 // TomTom "notifications" flag
	integrations atomic.Int32 // /api/integrations responses: 0 ok, else HTTP status
	status       atomic.Int32 // /api/command responses: 0 ok, else HTTP status
	badStatus    atomic.Bool  // /api/command returns text that is not JSON
}

func newFakeNotificationHub(t *testing.T) (Config, *fakeNotificationHub) {
	t.Helper()
	f := &fakeNotificationHub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/integrations", func(w http.ResponseWriter, r *http.Request) {
		if code := f.integrations.Load(); code != 0 {
			http.Error(w, "down", int(code))
			return
		}
		if f.webhook.Load() {
			w.Write([]byte(`{"operation":"get","notifications":{"enabled":true}}`))
		} else {
			w.Write([]byte(`{"operation":"get","notifications":{"enabled":false}}`))
		}
	})
	mux.HandleFunc("/api/command", func(w http.ResponseWriter, r *http.Request) {
		if code := f.status.Load(); code != 0 {
			http.Error(w, "down", int(code))
			return
		}
		if f.badStatus.Load() {
			w.Write([]byte("not json"))
			return
		}
		w.Write([]byte(`{"notifications":` + string(rune('0'+f.device.Load())) + `}`))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(srv.Certificate().Raw)
	resetSharedHubTransport()
	t.Cleanup(resetSharedHubTransport)
	return Config{Hub: srv.URL, Token: "tok", Fingerprint: hex.EncodeToString(sum[:])}, f
}

func TestNotificationGate(t *testing.T) {
	c, f := newFakeNotificationHub(t)

	f.webhook.Store(false)
	f.device.Store(0)
	if s, msg := notificationWorkerGate(context.Background(), c); s != gatePaused || msg != notificationPausedMessage {
		t.Fatalf("both off: %v %q", s, msg)
	}
	f.webhook.Store(true)
	if s, _ := notificationWorkerGate(context.Background(), c); s != gatePaused {
		t.Fatalf("webhook on, TomTom off must stay paused, got %v", s)
	}
	f.webhook.Store(false)
	f.device.Store(1)
	if s, _ := notificationWorkerGate(context.Background(), c); s != gatePaused {
		t.Fatalf("TomTom on, webhook off must stay paused, got %v", s)
	}
	f.webhook.Store(true)
	if s, msg := notificationWorkerGate(context.Background(), c); s != gateRun || msg != "" {
		t.Fatalf("both on: %v %q", s, msg)
	}
	// Turning the TomTom setting off wakes nothing and stops the next start.
	f.device.Store(0)
	if s, _ := notificationWorkerGate(context.Background(), c); s != gatePaused {
		t.Fatalf("settings change not seen: %v", s)
	}
}

func TestNotificationGateUnknownWhenHubOrDeviceFails(t *testing.T) {
	c, f := newFakeNotificationHub(t)
	f.webhook.Store(true)
	f.device.Store(1)

	f.integrations.Store(http.StatusBadGateway)
	if s, msg := notificationWorkerGate(context.Background(), c); s != gateUnknown || msg != "Cannot reach the hub; retrying" {
		t.Fatalf("hub error: %v %q", s, msg)
	}
	f.integrations.Store(0)

	f.status.Store(http.StatusBadGateway)
	if s, msg := notificationWorkerGate(context.Background(), c); s != gateUnknown || msg != "TomTom unavailable; retrying" {
		t.Fatalf("device error: %v %q", s, msg)
	}
	f.status.Store(0)

	f.badStatus.Store(true)
	if s, _ := notificationWorkerGate(context.Background(), c); s != gateUnknown {
		t.Fatalf("unparseable status must be unknown, got %v", s)
	}
	f.badStatus.Store(false)
	if s, _ := notificationWorkerGate(context.Background(), c); s != gateRun {
		t.Fatalf("recovery not seen: %v", s)
	}
}

func TestNotificationGateUnreachableHub(t *testing.T) {
	resetSharedHubTransport()
	c := Config{Hub: "https://127.0.0.1:1", Token: "t", Fingerprint: "00"}
	if s, _ := notificationWorkerGate(context.Background(), c); s != gateUnknown {
		t.Fatalf("closed port: %v", s)
	}
}

func TestPauseTimer(t *testing.T) {
	var p pauseTimer
	now := time.Unix(1000, 0)
	if p.update(true, now) {
		t.Fatal("must not exit on the first paused tick")
	}
	if p.update(true, now.Add(noticePauseExitAfter-time.Second)) {
		t.Fatal("exited before the pause limit")
	}
	if !p.update(true, now.Add(noticePauseExitAfter)) {
		t.Fatal("must exit once paused for the limit")
	}
	// Any active moment restarts the clock.
	p = pauseTimer{}
	p.update(true, now)
	p.update(false, now.Add(20*time.Second))
	if p.update(true, now.Add(35*time.Second)) {
		t.Fatal("pause clock was not reset by an active tick")
	}
	if !p.update(true, now.Add(35*time.Second+noticePauseExitAfter)) {
		t.Fatal("must exit a full limit after the new pause began")
	}
}
