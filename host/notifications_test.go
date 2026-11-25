package main

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestNoticeTextBoundedUTF8(t *testing.T) {
	got := noticeText(" \x00hello 🌍 ", 8)
	if !utf8.ValidString(got) || got != "hello " {
		t.Fatalf("noticeText returned %q", got)
	}
	if got := noticeText("message", 0); got != "" {
		t.Fatalf("zero limit returned %q", got)
	}
}

func TestNoticeQueueBaselinesAndDeduplicates(t *testing.T) {
	now := time.Now()
	initial := windowsNotice{ID: 1, Notification: Notification{App: "WhatsApp", Title: "old"}}
	fresh := windowsNotice{ID: 2, Notification: Notification{App: "WhatsApp", Title: "new"}}
	q := noticeQueue{}

	q.observe([]windowsNotice{initial}, NotificationConfig{}, now)
	if len(q.pending) != 0 {
		t.Fatal("baseline notifications must not be queued")
	}
	q.observe([]windowsNotice{initial, fresh, fresh}, NotificationConfig{}, now.Add(time.Second))
	if len(q.pending) != 1 || q.pending[0].notice.ID != fresh.ID {
		t.Fatalf("expected one new notification, got %#v", q.pending)
	}
	q.observe([]windowsNotice{initial, fresh}, NotificationConfig{}, now.Add(2*time.Second))
	if len(q.pending) != 1 {
		t.Fatalf("duplicate snapshot queued again: %d pending", len(q.pending))
	}
}

func TestNoticeQueueDropsDismissedExpiredAndExcluded(t *testing.T) {
	now := time.Now()
	q := noticeQueue{}
	q.observe(nil, NotificationConfig{}, now)
	excluded := windowsNotice{ID: 1, Notification: Notification{App: "PrivateApp", Title: "hidden"}}
	q.observe([]windowsNotice{excluded}, NotificationConfig{Excluded: []string{"privateapp"}}, now.Add(time.Second))
	if len(q.pending) != 0 {
		t.Fatal("excluded notification was queued")
	}

	fresh := windowsNotice{ID: 2, Notification: Notification{App: "Chat", Title: "hello"}}
	q.observe([]windowsNotice{excluded, fresh}, NotificationConfig{}, now.Add(2*time.Second))
	q.observe([]windowsNotice{excluded}, NotificationConfig{}, now.Add(3*time.Second))
	if len(q.pending) != 0 {
		t.Fatal("dismissed notification remained queued")
	}

	expiring := windowsNotice{ID: 3, Notification: Notification{App: "Chat", Title: "short-lived"}}
	q.observe([]windowsNotice{expiring}, NotificationConfig{}, now.Add(4*time.Second))
	q.observe([]windowsNotice{expiring}, NotificationConfig{}, now.Add(noticeLifetime+5*time.Second))
	if len(q.pending) != 0 {
		t.Fatal("expired notification remained queued")
	}
}

func TestNoticeQueueIsBoundedAndReset(t *testing.T) {
	now := time.Now()
	q := noticeQueue{}
	q.observe(nil, NotificationConfig{}, now)
	notices := make([]windowsNotice, maxObservedNotices+32)
	for i := range notices {
		notices[i] = windowsNotice{
			ID:           uint32(i + 1),
			Notification: Notification{Title: strings.Repeat("x", 12)},
		}
	}
	q.observe(notices, NotificationConfig{}, now.Add(time.Second))
	if len(q.seen) > maxObservedNotices || len(q.pending) != maxPendingNotices {
		t.Fatalf("queue exceeded bounds: seen=%d pending=%d", len(q.seen), len(q.pending))
	}
	if got := q.pending[0].notice.ID; got != uint32(maxObservedNotices-maxPendingNotices+1) {
		t.Fatalf("oldest queued id = %d, want most recent bounded window", got)
	}

	large := windowsNotice{
		ID: 1000, AppID: strings.Repeat("i", 1024),
		Notification: Notification{App: strings.Repeat("a", 256), Title: strings.Repeat("t", 512), Body: strings.Repeat("b", 2048), Artwork: []byte("art")},
	}
	q.reset()
	q.observe(nil, NotificationConfig{}, now)
	q.observe([]windowsNotice{large}, NotificationConfig{}, now.Add(time.Second))
	got := q.pending[0].notice
	if len(got.AppID) > 512 || len(got.App) > 160 || len(got.Title) > 256 || len(got.Body) > 1024 || len(got.Artwork) != 0 {
		t.Fatalf("queued notification was not bounded: %#v", got)
	}

	q.reset()
	if q.ready || len(q.seen) != 0 || len(q.pending) != 0 {
		t.Fatalf("reset left queue state: %#v", q)
	}
}

func TestNotificationAllowAndBlockLists(t *testing.T) {
	notice := windowsNotice{AppID: "com.example.chat", Notification: Notification{App: "Example Chat"}}
	if !notificationPermitted(notice, NotificationConfig{}) {
		t.Fatal("default notification policy should allow apps")
	}
	config := NotificationConfig{AllowListOnly: true, Allowed: []string{"com.example.chat"}}
	if !notificationPermitted(notice, config) {
		t.Fatal("allow-list should match the app ID")
	}
	config.Excluded = []string{"EXAMPLE CHAT"}
	if notificationPermitted(notice, config) {
		t.Fatal("block list must take precedence over allow list")
	}
	config = NotificationConfig{AllowListOnly: true, Allowed: []string{"Calendar"}}
	if notificationPermitted(notice, config) {
		t.Fatal("allow-list-only mode must reject unlisted apps")
	}
}

func TestQuietHoursAcrossMidnight(t *testing.T) {
	start, end := "22:00", "07:00"
	cases := []struct {
		at   string
		want bool
	}{
		{"21:59", false},
		{"22:00", true},
		{"23:59", true},
		{"00:00", true},
		{"06:59", true},
		{"07:00", false},
	}
	for _, tc := range cases {
		at, err := time.Parse("15:04", tc.at)
		if err != nil {
			t.Fatal(err)
		}
		if got := quietHoursActive(true, start, end, at); got != tc.want {
			t.Errorf("quietHoursActive(%s) = %t, want %t", tc.at, got, tc.want)
		}
	}
	if quietHoursActive(false, start, end, time.Date(2026, 1, 1, 23, 0, 0, 0, time.Local)) {
		t.Fatal("disabled quiet hours were active")
	}
}

func TestNotificationQuietHoursValidation(t *testing.T) {
	valid := NotificationConfig{QuietHoursEnabled: true, QuietStart: "22:00", QuietEnd: "07:00"}
	if err := validateIntegrations(defaultWeather(), valid); err != nil {
		t.Fatalf("valid quiet hours rejected: %v", err)
	}
	for _, invalid := range []NotificationConfig{
		{QuietHoursEnabled: true, QuietStart: "22:00", QuietEnd: "22:00"},
		{QuietHoursEnabled: true, QuietStart: "25:00", QuietEnd: "07:00"},
		{QuietHoursEnabled: true, QuietStart: "9pm", QuietEnd: "07:00"},
		{QuietHoursEnabled: true, QuietStart: "+1:00", QuietEnd: "07:00"},
	} {
		if err := validateIntegrations(defaultWeather(), invalid); err == nil {
			t.Errorf("invalid quiet hours accepted: %#v", invalid)
		}
	}
}
