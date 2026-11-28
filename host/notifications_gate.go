package main

import (
	"context"
	"encoding/json"
	"time"
)

// A notifications-worker is a whole Go process (about 9 MB on Windows). It is
// only worth having while forwarding can actually deliver something, so the
// supervisor asks this gate before starting one, and the worker leaves by itself
// after it has been paused for a while.

type notificationGateState int

const (
	// gateUnknown: the hub or the TomTom could not be asked. A running worker is
	// left alone (restarting it could drop a notice that arrived meanwhile); a
	// stopped one is not started.
	gateUnknown notificationGateState = iota
	gatePaused                        // forwarding is switched off on the hub or the TomTom
	gateRun
)

const (
	notificationPausedMessage = "Paused: enable the authenticated notification webhook and TomTom notifications"
	// How long the worker stays paused before it exits and frees its process.
	noticePauseExitAfter = 30 * time.Second
)

// notificationWorkerGate decides whether a notifications worker should run, and
// returns the status text to show when it should not.
func notificationWorkerGate(ctx context.Context, c Config) (notificationGateState, string) {
	settings, err := integrationGetContext(ctx, c)
	if err != nil {
		return gateUnknown, "Cannot reach the hub; retrying"
	}
	raw, err := hubCallContext(ctx, c, "status")
	var ds struct {
		Notifications int `json:"notifications"`
	}
	if err != nil || json.Unmarshal([]byte(raw), &ds) != nil {
		return gateUnknown, "TomTom unavailable; retrying"
	}
	if !settings.Notifications.Enabled || ds.Notifications == 0 {
		return gatePaused, notificationPausedMessage
	}
	return gateRun, ""
}

// pauseTimer tells the worker when it has been paused long enough to exit.
type pauseTimer struct{ since time.Time }

func (p *pauseTimer) update(paused bool, now time.Time) (exit bool) {
	if !paused {
		p.since = time.Time{}
		return false
	}
	if p.since.IsZero() {
		p.since = now
	}
	return now.Sub(p.since) >= noticePauseExitAfter
}
