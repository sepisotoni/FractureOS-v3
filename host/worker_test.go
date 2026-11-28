package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), key+"=") {
			return kv[len(key)+1:], true
		}
	}
	return "", false
}

func TestWorkerEnvDefaults(t *testing.T) {
	t.Setenv("GOGC", "")
	t.Setenv("GOMEMLIMIT", "")
	os.Unsetenv("GOGC")
	os.Unsetenv("GOMEMLIMIT")
	env := workerEnv()
	if v, ok := envValue(env, "GOGC"); !ok || v != workerGOGC {
		t.Fatalf("GOGC = %q, %v", v, ok)
	}
	if v, ok := envValue(env, "GOMEMLIMIT"); !ok || v != workerMemoryLimit {
		t.Fatalf("GOMEMLIMIT = %q, %v", v, ok)
	}
}

func TestWorkerEnvKeepsUserSettings(t *testing.T) {
	t.Setenv("GOGC", "200")
	t.Setenv("GOMEMLIMIT", "1GiB")
	env := workerEnv()
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "GOGC=") || strings.HasPrefix(kv, "GOMEMLIMIT=") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("user GOGC/GOMEMLIMIT duplicated or overridden: %v", env)
	}
	if v, _ := envValue(env, "GOGC"); v != "200" {
		t.Fatalf("GOGC = %q", v)
	}
}

func TestStartWorkerSetsEnvAndStarts(t *testing.T) {
	os.Unsetenv("GOGC")
	os.Unsetenv("GOMEMLIMIT")
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperEnvPrinter")
	cmd.Env = nil
	var out strings.Builder
	cmd.Stdout = &out
	if err := startWorker(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "GOGC="+workerGOGC) {
		t.Fatalf("child did not get GOGC: %q", out.String())
	}
}

func TestHelperEnvPrinter(t *testing.T) {
	if os.Getenv("GOGC") == "" {
		t.Skip()
	}
	fmt.Println("GOGC=" + os.Getenv("GOGC"))
}

func TestStartWorkerReportsStartError(t *testing.T) {
	if err := startWorker(exec.Command("/nonexistent/fracture-worker")); err == nil {
		t.Fatal("expected start error")
	}
}

func TestSpawnBackoffGrowsAndCaps(t *testing.T) {
	var logged []string
	spawnLog = func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }
	defer func() { spawnLog = nil }()
	s := newSpawnBackoff("x-worker")
	now := time.Unix(1000, 0)
	want := []time.Duration{3, 6, 12, 24, 48, 60, 60}
	for i, w := range want {
		if !s.allowed(now) {
			t.Fatalf("attempt %d should be allowed at %v", i, now)
		}
		s.failed(now, errors.New("boom"))
		if got := s.next.Sub(now); got != w*time.Second {
			t.Fatalf("attempt %d delay = %v, want %v", i, got, w*time.Second)
		}
		if s.allowed(now.Add(w*time.Second - time.Millisecond)) {
			t.Fatalf("attempt %d allowed before delay elapsed", i)
		}
		now = now.Add(w * time.Second)
	}
	if len(logged) != 1 {
		t.Fatalf("identical failures should log once, got %d: %v", len(logged), logged)
	}
	s.failed(now, errors.New("different"))
	if len(logged) != 2 {
		t.Fatalf("a new message should log, got %d", len(logged))
	}
}

func TestSpawnBackoffCrashLoopVersusNormalExit(t *testing.T) {
	spawnLog = func(string, ...any) {}
	defer func() { spawnLog = nil }()
	s := newSpawnBackoff("x-worker")
	now := time.Unix(1000, 0)
	s.started(now)
	s.exited(now.Add(time.Second), errors.New("exit 1"))
	if s.fails != 1 || s.allowed(now.Add(time.Second)) {
		t.Fatalf("worker that died after 1s must back off: fails=%d", s.fails)
	}
	later := now.Add(time.Minute)
	s.started(later)
	s.exited(later.Add(time.Minute), nil)
	if s.fails != 0 || !s.allowed(later.Add(time.Minute)) {
		t.Fatalf("a long-running worker that exits must reset backoff: fails=%d", s.fails)
	}
}

// The Win32 job object API rejects a wrong structure size; these are the 64-bit
// sizes for amd64 and arm64.
func TestJobStructSizes(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("64-bit layout only")
	}
	if got := unsafe.Sizeof(jobBasicLimit{}); got != 64 {
		t.Fatalf("JOBOBJECT_BASIC_LIMIT_INFORMATION = %d, want 64", got)
	}
	if got := unsafe.Sizeof(jobIOCounters{}); got != 48 {
		t.Fatalf("IO_COUNTERS = %d, want 48", got)
	}
	if got := unsafe.Sizeof(jobExtendedLimit{}); got != 144 {
		t.Fatalf("JOBOBJECT_EXTENDED_LIMIT_INFORMATION = %d, want 144", got)
	}
}

func TestBackgroundSetStopWaitsForGoroutines(t *testing.T) {
	set := newBackgroundSet(context.Background())
	var finished atomic.Int32
	for i := 0; i < 5; i++ {
		set.run(func(ctx context.Context) {
			<-ctx.Done()
			time.Sleep(50 * time.Millisecond) // a supervisor reaping its worker
			finished.Add(1)
		})
	}
	set.stop()
	if n := finished.Load(); n != 5 {
		t.Fatalf("stop returned with %d/5 goroutines finished", n)
	}
}

func TestBackgroundSetFollowsParentContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	set := newBackgroundSet(parent)
	ran := make(chan struct{})
	set.run(func(ctx context.Context) { <-ctx.Done(); close(ran) })
	cancel()
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("goroutine did not see parent cancellation")
	}
	set.stop()
}
