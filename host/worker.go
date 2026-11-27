package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Worker processes are the same executable started with a "<name>-worker"
// argument. Everything that spawns one goes through startWorker so that all of
// them get the same runtime tuning and are tied to the parent's lifetime.

const (
	// Worker live heaps are tiny. GOGC=50 trades a little CPU for a smaller
	// resident set; the memory limit is a soft guard only (the runtime caps GC CPU
	// when the limit is hit), so a worker is never killed or starved by it.
	workerGOGC        = "50"
	workerMemoryLimit = "48MiB"
)

// workerEnv returns the parent's environment plus the worker GC settings. A
// GOGC or GOMEMLIMIT the user already set always wins. The tray and the hub
// never use this: it applies to worker processes only.
func workerEnv() []string {
	env := os.Environ()
	hasGC, hasLimit := false, false
	for _, kv := range env {
		switch {
		case strings.HasPrefix(strings.ToUpper(kv), "GOGC="):
			hasGC = true
		case strings.HasPrefix(strings.ToUpper(kv), "GOMEMLIMIT="):
			hasLimit = true
		}
	}
	if !hasGC {
		env = append(env, "GOGC="+workerGOGC)
	}
	if !hasLimit {
		env = append(env, "GOMEMLIMIT="+workerMemoryLimit)
	}
	return env
}

// startWorker starts a worker command with the worker environment and, on
// Windows, assigns it to the kill-on-close job so it cannot outlive its parent.
func startWorker(cmd *exec.Cmd) error {
	if cmd.Env == nil {
		cmd.Env = workerEnv()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	trackChild(cmd)
	return nil
}

var spawnLog = log.Printf

// spawnBackoff throttles and reports worker start failures and crash loops. A
// failing supervisor used to retry every tick forever without saying anything.
type spawnBackoff struct {
	name    string
	base    time.Duration
	max     time.Duration
	fails   int
	next    time.Time
	begin   time.Time
	lastMsg string
}

func newSpawnBackoff(name string) *spawnBackoff {
	return &spawnBackoff{name: name, base: 3 * time.Second, max: time.Minute}
}

func (s *spawnBackoff) allowed(now time.Time) bool { return !now.Before(s.next) }

func (s *spawnBackoff) started(now time.Time) { s.begin = now }

// failed records a failure, schedules the next attempt (base, 2x, 4x ... max)
// and logs when the message changes and then every 10th repeat.
func (s *spawnBackoff) failed(now time.Time, err error) {
	s.fails++
	delay := s.base
	for i := 1; i < s.fails && delay < s.max; i++ {
		delay *= 2
	}
	if delay > s.max {
		delay = s.max
	}
	s.next = now.Add(delay)
	msg := fmt.Sprint(err)
	if msg != s.lastMsg || s.fails%10 == 0 {
		spawnLog("%s: attempt %d failed: %v (next try in %s)", s.name, s.fails, err, delay)
		s.lastMsg = msg
	}
}

// exited records that a running worker stopped. One that ran for a while is a
// normal restart and resets the backoff; one that died young counts as a failure.
func (s *spawnBackoff) exited(now time.Time, err error) {
	if ran := now.Sub(s.begin); ran < 10*time.Second {
		s.failed(now, fmt.Errorf("exited after %s: %v", ran.Round(time.Millisecond), err))
		return
	}
	s.fails, s.next, s.lastMsg = 0, time.Time{}, ""
}

// backgroundSet runs supervisor goroutines under one cancellable context and
// lets the owner wait for all of them. Supervisors reap their worker processes
// in a deferred stop(), so returning before they finish could let the process
// exit first and leave workers behind.
type backgroundSet struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func newBackgroundSet(parent context.Context) *backgroundSet {
	ctx, cancel := context.WithCancel(parent)
	return &backgroundSet{ctx: ctx, cancel: cancel}
}

func (s *backgroundSet) run(f func(context.Context)) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		f(s.ctx)
	}()
}

// stop cancels the context and blocks until every goroutine has returned.
func (s *backgroundSet) stop() {
	s.cancel()
	s.wg.Wait()
}
