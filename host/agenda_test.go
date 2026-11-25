package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseAgendaTodayAndRecurrences(t *testing.T) {
	now := time.Date(2026, 11, 25, 7, 0, 0, 0, time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	yesterday := today.AddDate(0, 0, -1)
	fixture := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:one-off
DTSTART:` + today.Add(9*time.Hour).Format("20060102T150405") + `
SUMMARY:Café morning
LOCATION:Office
END:VEVENT
BEGIN:VEVENT
UID:excluded
DTSTART:` + yesterday.Add(10*time.Hour).Format("20060102T150405") + `
RRULE:FREQ=DAILY
EXDATE:` + today.Add(10*time.Hour).Format("20060102T150405") + `
SUMMARY:Excluded
END:VEVENT
BEGIN:VEVENT
UID:repeat
DTSTART:` + yesterday.Add(11*time.Hour).Format("20060102T150405") + `
RRULE:FREQ=DAILY
RDATE:` + today.Add(13*time.Hour+30*time.Minute).Format("20060102T150405") + `
SUMMARY:Daily meeting
END:VEVENT
BEGIN:VEVENT
UID:repeat
RECURRENCE-ID:` + today.Add(11*time.Hour).Format("20060102T150405") + `
DTSTART:` + today.Add(12*time.Hour).Format("20060102T150405") + `
SUMMARY:Moved meeting
END:VEVENT
BEGIN:VEVENT
UID:all-day
DTSTART;VALUE=DATE:` + today.Format("20060102") + `
SUMMARY:Holiday
END:VEVENT
BEGIN:VEVENT
UID:cancelled
DTSTART:` + today.Add(5*time.Hour).Format("20060102T150405") + `
STATUS:CANCELLED
SUMMARY:Cancelled
END:VEVENT
END:VCALENDAR
`
	config := AgendaConfig{Enabled: true, FeedURL: "https://calendar.example/feed.ics", MaxEvents: 8}
	events, err := parseAgenda([]byte(fixture), config, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("got %d agenda entries, want 4: %+v", len(events), events)
	}
	if events[0].Title != "Holiday" || !events[0].AllDay {
		t.Fatalf("all-day event was not retained: %+v", events[0])
	}
	if events[1].Title != "Cafe morning" || events[1].Location != "" || events[1].At.Hour() != 9 {
		t.Fatalf("default summary or location policy is wrong: %+v", events[1])
	}
	if events[2].Title != "Moved meeting" || events[2].At.Hour() != 12 {
		t.Fatalf("recurrence override was not applied: %+v", events[2])
	}
	if events[3].At.Hour() != 13 || events[3].At.Minute() != 30 {
		t.Fatalf("RDATE was not expanded: %+v", events[3])
	}

	config.ShowLocation = true
	events, err = parseAgenda([]byte(fixture), config, now)
	if err != nil {
		t.Fatal(err)
	}
	if events[1].Location != "Office" {
		t.Fatalf("opt-in location missing: %+v", events[1])
	}
}

func TestParseAgendaBoundsAndUpcomingOnly(t *testing.T) {
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	fixture := `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:past
DTSTART:` + today.Add(10*time.Hour).Format("20060102T150405") + `
SUMMARY:Past
END:VEVENT
BEGIN:VEVENT
UID:future
DTSTART:` + today.Add(13*time.Hour).Format("20060102T150405") + `
SUMMARY:` + strings.Repeat("x", 80) + `
END:VEVENT
END:VCALENDAR
`
	events, err := parseAgenda([]byte(fixture), AgendaConfig{MaxEvents: 1}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Title != strings.Repeat("x", 48) {
		t.Fatalf("upcoming event limit/text bound failed: %+v", events)
	}
}

func TestValidateAgendaConfiguration(t *testing.T) {
	for _, invalid := range []AgendaConfig{
		{Enabled: true, MaxEvents: 5},
		{FeedURL: "http://calendar.example/feed.ics", MaxEvents: 5},
		{FeedURL: "https://user:secret@calendar.example/feed.ics", MaxEvents: 5},
		{FeedURL: "https://calendar.example/feed.ics#private", MaxEvents: 5},
		{FeedURL: "https://calendar.example/feed.ics", MaxEvents: 9},
	} {
		if err := validateAgenda(invalid); err == nil {
			t.Errorf("validateAgenda(%+v) succeeded", invalid)
		}
	}
	if err := validateAgenda(AgendaConfig{Enabled: true, FeedURL: "https://calendar.example/feed.ics", MaxEvents: 5}); err != nil {
		t.Fatalf("valid agenda configuration rejected: %v", err)
	}
}

func TestAgendaCommandValidation(t *testing.T) {
	if err := validateCommand("agenda-item 0,31343a3330,4d656574696e67,-"); err != nil {
		t.Fatalf("valid agenda command rejected: %v", err)
	}
	for _, command := range []string{
		"agenda-item 8,31343a3330,4d656574696e67,-",
		"agenda-item 0,-,4d656574696e67,-",
		"agenda-item 0,31343a3330,00,-",
		"agenda-item 0,31343a3330," + strings.Repeat("41", 49) + ",-",
	} {
		if err := validateCommand(command); err == nil {
			t.Errorf("invalid agenda command accepted: %q", command)
		}
	}
}
