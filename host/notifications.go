package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Notifications remain in memory only. The first snapshot establishes a
// baseline, so starting the tray never replays the user's notification history.
type windowsNotice struct {
	ID      uint32
	Created int64
	AppID   string
	Notification
}

const (
	maxObservedNotices = 256
	maxPendingNotices  = 16
	noticeLifetime     = 30 * time.Second
)

func noticeKey(n windowsNotice) string {
	h := sha256.Sum256([]byte(n.AppID + "\x00" + n.App + "\x00" + n.Title + "\x00" + n.Body))
	return fmt.Sprintf("%d:%d:%s", n.ID, n.Created, hex.EncodeToString(h[:12]))
}

func noticeText(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
	if len(s) <= limit {
		return strings.Clone(s)
	}
	s = s[:limit]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return strings.Clone(s)
}

func notificationAppMatches(app string, candidates []string) bool {
	app = strings.TrimSpace(app)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && strings.EqualFold(candidate, app) {
			return true
		}
	}
	return false
}

func notificationPermitted(n windowsNotice, config NotificationConfig) bool {
	if notificationAppMatches(n.App, config.Excluded) || notificationAppMatches(n.AppID, config.Excluded) {
		return false
	}
	return !config.AllowListOnly || notificationAppMatches(n.App, config.Allowed) || notificationAppMatches(n.AppID, config.Allowed)
}

func clockMinutes(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	for i := range value {
		if i != 2 && (value[i] < '0' || value[i] > '9') {
			return 0, false
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func quietHoursActive(enabled bool, start, end string, now time.Time) bool {
	if !enabled {
		return false
	}
	from, fromOK := clockMinutes(start)
	until, untilOK := clockMinutes(end)
	if !fromOK || !untilOK || from == until {
		return false
	}
	current := now.Hour()*60 + now.Minute()
	if from < until {
		return current >= from && current < until
	}
	return current >= from || current < until
}

func notificationTestForward() error {
	c, err := readConfig()
	if err != nil {
		return err
	}
	settings, err := integrationGet(c)
	if err != nil {
		return fmt.Errorf("cannot check notification settings on the hub: %w", err)
	}
	if !settings.Notifications.Enabled {
		return fmt.Errorf("enable the authenticated notification webhook before testing")
	}
	out, err := hubRequest(c, "/api/notifications", Notification{
		App:   "FractureOS",
		Title: "Notification test",
		Body:  "The hub and TomTom notification path accepted this test.",
	})
	if err != nil {
		return err
	}
	if out != "OK" {
		return fmt.Errorf("notification test was not delivered: %s", out)
	}
	return nil
}

type pendingNotice struct {
	notice windowsNotice
	queued time.Time
}

type noticeQueue struct {
	ready   bool
	seen    map[string]bool
	pending []pendingNotice
}

func (q *noticeQueue) reset() {
	for i := range q.pending {
		q.pending[i] = pendingNotice{}
	}
	*q = noticeQueue{}
}

func (q *noticeQueue) observe(notices []windowsNotice, config NotificationConfig, now time.Time) {
	if len(notices) > maxObservedNotices {
		notices = notices[:maxObservedNotices]
	}
	next := make(map[string]bool, len(notices))
	for _, n := range notices {
		n.App = noticeText(n.App, 160)
		n.AppID = noticeText(n.AppID, 512)
		n.Title = noticeText(n.Title, 256)
		n.Body = noticeText(n.Body, 1024)
		n.Artwork = nil
		key := noticeKey(n)
		if next[key] {
			continue
		}
		next[key] = true
		if q.ready && !q.seen[key] && notificationPermitted(n, config) && (n.Title != "" || n.Body != "") {
			if len(q.pending) == maxPendingNotices {
				q.pending[0] = pendingNotice{}
				q.pending = q.pending[1:]
			}
			q.pending = append(q.pending, pendingNotice{n, now})
		}
	}
	q.seen, q.ready = next, true
	kept := q.pending[:0]
	for _, p := range q.pending {
		// Dismissed, expired or newly excluded messages should not be relayed later.
		if next[noticeKey(p.notice)] && now.Sub(p.queued) < noticeLifetime && notificationPermitted(p.notice, config) {
			kept = append(kept, p)
		}
	}
	for i := len(kept); i < len(q.pending); i++ {
		q.pending[i] = pendingNotice{}
	}
	q.pending = kept
}
