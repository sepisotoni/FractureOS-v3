package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// Every hub call used to build a new http.Transport, do a full TLS 1.3 handshake
// (about 1.6 ms CPU and 95 KB of garbage) and close it. The supervisors make
// roughly 1.5 such calls per second while idle, so one transport is now kept per
// (hub, token, fingerprint). Certificate pinning is unchanged: hubTransport still
// verifies the fingerprint and validity on every new connection.
const (
	sharedHubIdleTimeout = 15 * time.Second // below the hub server's 30 s IdleTimeout
	// VerifyConnection only runs at handshake time, so a connection that never goes
	// idle could outlive the certificate. Recycling the transport bounds that.
	sharedHubMaxAge = 10 * time.Minute
)

type sharedHubEntry struct {
	key     string
	t       *http.Transport
	created time.Time
}

var (
	sharedHubMu    sync.Mutex
	sharedHubCur   *sharedHubEntry
	sharedHubClock = time.Now
)

func sharedHubKey(c Config) string {
	return c.Hub + "\x00" + c.Token + "\x00" + strings.ToLower(c.Fingerprint)
}

// sharedHubTransport returns the cached transport for c, replacing it (and
// closing the old one's idle connections) when the key changes or it is too old.
func sharedHubTransport(c Config) (*http.Transport, error) {
	sharedHubMu.Lock()
	defer sharedHubMu.Unlock()
	key, now := sharedHubKey(c), sharedHubClock()
	if cur := sharedHubCur; cur != nil && cur.key == key && now.Sub(cur.created) < sharedHubMaxAge {
		return cur.t, nil
	}
	t, err := hubTransport(c)
	if err != nil {
		return nil, err
	}
	t.IdleConnTimeout = sharedHubIdleTimeout
	t.MaxIdleConnsPerHost = 2
	t.TLSHandshakeTimeout = 10 * time.Second
	if sharedHubCur != nil {
		sharedHubCur.t.CloseIdleConnections()
	}
	sharedHubCur = &sharedHubEntry{key: key, t: t, created: now}
	return t, nil
}

// resetSharedHubTransport drops the cache (tests and benchmarks).
func resetSharedHubTransport() {
	sharedHubMu.Lock()
	defer sharedHubMu.Unlock()
	if sharedHubCur != nil {
		sharedHubCur.t.CloseIdleConnections()
		sharedHubCur = nil
	}
}
