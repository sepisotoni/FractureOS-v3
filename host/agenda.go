package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

const (
	maxAgendaEvents = 8
	maxAgendaFeed   = 256 * 1024
	maxAgendaItems  = 4096
)

type AgendaConfig struct {
	Enabled      bool   `json:"enabled"`
	FeedURL      string `json:"feed_url"`
	MaxEvents    int    `json:"max_events"`
	ShowLocation bool   `json:"show_location"`
}

type agendaEvent struct {
	At       time.Time `json:"at"`
	Title    string    `json:"title"`
	Location string    `json:"location,omitempty"`
	AllDay   bool      `json:"all_day,omitempty"`
}

type agendaUpdate struct {
	Events []agendaEvent `json:"events"`
}

func defaultAgenda() AgendaConfig {
	return AgendaConfig{MaxEvents: 5}
}

func nAgendaIndex(value string) bool {
	index, err := strconv.Atoi(value)
	return err == nil && index >= 0 && index < maxAgendaEvents
}

func validateAgenda(a AgendaConfig) error {
	if a.MaxEvents < 1 || a.MaxEvents > maxAgendaEvents {
		return errors.New("agenda event limit must be between 1 and 8")
	}
	if len(a.FeedURL) > 2048 {
		return errors.New("agenda feed URL is too long")
	}
	if a.FeedURL == "" {
		if a.Enabled {
			return errors.New("configure an ICS feed URL before enabling the agenda")
		}
		return nil
	}
	u, err := url.Parse(a.FeedURL)
	if err != nil || !u.IsAbs() || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("agenda feed URL must be an HTTPS URL without embedded credentials or a fragment")
	}
	return nil
}

func fetchAgenda(ctx context.Context, config AgendaConfig, now time.Time) ([]agendaEvent, error) {
	if err := validateAgenda(config); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.FeedURL, nil)
	if err != nil {
		return nil, errors.New("invalid agenda feed request")
	}
	req.Header.Set("Accept", "text/calendar, text/plain;q=0.9")
	req.Header.Set("User-Agent", "FractureOS/"+version)
	client := http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("agenda redirects are not followed")
		},
	}
	defer client.CloseIdleConnections()
	res, err := client.Do(req)
	if err != nil {
		return nil, errors.New("agenda feed request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agenda feed returned HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxAgendaFeed+1))
	if err != nil {
		return nil, errors.New("agenda feed could not be read")
	}
	if len(data) > maxAgendaFeed {
		return nil, errors.New("agenda feed exceeds 256 KiB")
	}
	events, err := parseAgenda(data, config, now)
	if err != nil {
		return nil, errors.New("agenda feed contains invalid calendar data")
	}
	return events, nil
}

func parseAgenda(data []byte, config AgendaConfig, now time.Time) ([]agendaEvent, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	localNow := now.In(time.Local)
	startOfDay := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	endOfDay := startOfDay.AddDate(0, 0, 1)

	events := cal.Events()
	masters := make(map[string]*ics.VEvent)
	overrides := make(map[string]map[int64]*ics.VEvent)
	for _, event := range events {
		uid := event.Id()
		if uid == "" {
			continue
		}
		recurrenceID, recurrenceErr := event.GetRecurrenceID()
		if recurrenceErr == nil {
			if overrides[uid] == nil {
				overrides[uid] = make(map[int64]*ics.VEvent)
			}
			overrides[uid][recurrenceID.Unix()] = event
			continue
		}
		masters[uid] = event
	}

	type occurrence struct {
		event *ics.VEvent
		at    time.Time
	}
	var result []occurrence
	for _, event := range masters {
		if strings.EqualFold(propertyValue(event, ics.ComponentPropertyStatus), "CANCELLED") {
			continue
		}
		start, err := event.GetStartAt()
		if err != nil {
			continue
		}
		allDay := strings.EqualFold(propertyParameter(event.GetProperty(ics.ComponentPropertyDtStart), "VALUE"), "DATE")
		rules := event.GetProperties(ics.ComponentPropertyRrule)
		if len(rules) > 1 {
			return nil, errors.New("multiple recurrence rules are not supported")
		}
		if len(event.GetProperties(ics.ComponentPropertyExrule)) != 0 {
			return nil, errors.New("EXRULE recurrence exclusions are not supported")
		}
		set := &rrule.Set{}
		set.DTStart(start)
		if len(rules) == 1 {
			rule, err := rrule.StrToRRule("RRULE:" + rules[0].Value)
			if err != nil {
				return nil, err
			}
			rule.DTStart(start)
			set.RRule(rule)
		} else {
			set.RDate(start)
		}
		rdates, err := event.GetRDates()
		if err != nil {
			return nil, err
		}
		for _, date := range rdates {
			set.RDate(date)
		}
		exdates, err := event.GetExDates()
		if err != nil {
			return nil, err
		}
		for _, date := range exdates {
			set.ExDate(date)
		}
		after := localNow
		if allDay {
			after = startOfDay
		}
		if len(rules) == 0 && !start.Before(startOfDay) && start.Before(endOfDay) && (allDay || !start.Before(localNow)) {
			result = append(result, occurrence{event, start})
		}
		if len(result) > maxAgendaItems {
			return nil, errors.New("agenda has too many daily event instances")
		}
		next := set.After(after.Add(-time.Second), true)
		occurrenceCount := 0
		for !next.IsZero() && next.Before(endOfDay) {
			occurrenceCount++
			if occurrenceCount > 128 {
				return nil, errors.New("agenda recurrence has too many daily occurrences")
			}
			if (allDay && !next.Before(startOfDay)) || (!allDay && !next.Before(localNow)) {
				result = append(result, occurrence{event, next.In(time.Local)})
				if len(result) > maxAgendaItems {
					return nil, errors.New("agenda has too many daily event instances")
				}
			}
			next = set.After(next, false)
		}
	}

	filtered := result[:0]
	for _, item := range result {
		uid := item.event.Id()
		override := overrides[uid][item.at.Unix()]
		if override == nil {
			filtered = append(filtered, item)
			continue
		}
		if strings.EqualFold(propertyValue(override, ics.ComponentPropertyStatus), "CANCELLED") {
			continue
		}
		at, err := override.GetStartAt()
		if err != nil {
			continue
		}
		at = at.In(time.Local)
		if !at.Before(localNow) && at.Before(endOfDay) {
			filtered = append(filtered, occurrence{override, at})
		}
	}
	for uid, instances := range overrides {
		master := masters[uid]
		if master == nil {
			continue
		}
		for recurrenceID, override := range instances {
			alreadyIncluded := false
			for _, item := range filtered {
				if item.event == override {
					alreadyIncluded = true
					break
				}
			}
			if alreadyIncluded || strings.EqualFold(propertyValue(override, ics.ComponentPropertyStatus), "CANCELLED") {
				continue
			}
			at, err := override.GetStartAt()
			if err != nil {
				continue
			}
			at = at.In(time.Local)
			if !at.Before(localNow) && at.Before(endOfDay) && recurrenceID > 0 {
				filtered = append(filtered, occurrence{override, at})
			}
		}
	}

	sort.Slice(filtered, func(i, j int) bool { return filtered[i].at.Before(filtered[j].at) })
	seen := make(map[string]struct{})
	out := make([]agendaEvent, 0, min(config.MaxEvents, len(filtered)))
	for _, item := range filtered {
		summary := propertyValue(item.event, ics.ComponentPropertySummary)
		title := agendaASCII(summary, 48)
		if title == "" {
			title = "Untitled event"
		}
		location := ""
		if config.ShowLocation {
			location = agendaASCII(propertyValue(item.event, ics.ComponentPropertyLocation), 32)
		}
		key := item.event.Id() + ":" + item.at.Format(time.RFC3339)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, agendaEvent{
			At:       item.at,
			Title:    title,
			Location: location,
			AllDay:   strings.EqualFold(propertyParameter(item.event.GetProperty(ics.ComponentPropertyDtStart), "VALUE"), "DATE"),
		})
		if len(out) == config.MaxEvents {
			break
		}
	}
	return out, nil
}

func propertyValue(event *ics.VEvent, name ics.ComponentProperty) string {
	property := event.GetProperty(name)
	if property == nil {
		return ""
	}
	return property.Value
}

func propertyParameter(property *ics.IANAProperty, name string) string {
	if property == nil {
		return ""
	}
	for key, values := range property.ICalParameters {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func agendaASCII(input string, limit int) string {
	raw, err := hex.DecodeString(asciiHex(strings.ReplaceAll(input, "\n", " ")))
	if err != nil || len(raw) == 0 {
		return ""
	}
	if len(raw) > limit {
		raw = raw[:limit]
	}
	return strings.TrimSpace(string(bytes.Trim(raw, " \t\r\n")))
}

func agendaDeviceCommand(index int, event agendaEvent) string {
	title := asciiHex(event.Title)
	location := asciiHex(event.Location)
	if location == "" {
		location = "-"
	}
	timeLabel := "All day"
	if !event.AllDay {
		timeLabel = event.At.In(time.Local).Format("15:04")
	}
	return fmt.Sprintf("agenda-item %d,%s,%s,%s", index, asciiHex(timeLabel), title, location)
}

func registerAgenda(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/agenda", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var update agendaUpdate
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if decoder.Decode(&update) != nil || len(update.Events) > maxAgendaEvents {
			http.Error(w, "invalid agenda update", http.StatusBadRequest)
			return
		}
		commands := []string{"agenda-clear"}
		for i, event := range update.Events {
			if event.Title == "" || len(event.Title) > 48 || len(event.Location) > 32 || event.At.IsZero() {
				http.Error(w, "invalid agenda event", http.StatusBadRequest)
				return
			}
			command := agendaDeviceCommand(i, event)
			if err := validateCommand(command); err != nil {
				http.Error(w, "invalid agenda event", http.StatusBadRequest)
				return
			}
			commands = append(commands, command)
		}
		if err := b.batch(commands); err != nil {
			http.Error(w, "agenda delivery failed", http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, "OK")
	})
}

func agendaWorker() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var fetched time.Time
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var config AgendaConfig
	for {
		c, err := readConfig()
		if err != nil {
			return err
		}
		if !c.Agenda.Enabled {
			return nil
		}
		if c.Agenda != config {
			config = c.Agenda
			fetched = time.Time{}
		}
		if time.Since(fetched) >= 15*time.Minute {
			events, fetchErr := fetchAgenda(ctx, config, time.Now())
			if fetchErr != nil {
				_, _ = hubRequest(c, "/api/agenda", agendaUpdate{})
				fmt.Fprintln(os.Stderr, "agenda:", fetchErr)
				fetched = time.Now()
			} else if _, err = hubRequest(c, "/api/agenda", agendaUpdate{Events: events}); err != nil {
				fmt.Fprintln(os.Stderr, "agenda: device update failed")
				fetched = time.Now()
			} else {
				fetched = time.Now()
			}
		}
		status, err := hubCall(c, "status")
		var state struct {
			AgendaView bool `json:"agenda_view"`
		}
		if err != nil || json.Unmarshal([]byte(status), &state) != nil || !state.AgendaView {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func agendaSupervisor(ctx context.Context, b *Broker) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done <-chan error
	var active AgendaConfig
	clear := func() {
		if _, err := b.call("agenda-clear"); err != nil {
			fmt.Fprintln(os.Stderr, "agenda: could not clear device summary:", err)
		}
	}
	stop := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
			done = nil
			clear()
		}
	}
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			cancel()
			cancel = nil
			done = nil
			clear()
		case <-ticker.C:
			c, err := readConfig()
			needed := err == nil && c.Agenda.Enabled
			if needed {
				out, callErr := b.call("status")
				var status struct {
					AgendaView bool `json:"agenda_view"`
				}
				needed = callErr == nil && json.Unmarshal([]byte(out), &status) == nil && status.AgendaView
			}
			if !needed {
				stop()
				continue
			}
			if cancel != nil && active != c.Agenda {
				stop()
			}
			if cancel != nil {
				continue
			}
			exe, err := os.Executable()
			if err != nil {
				continue
			}
			child, stopChild := context.WithCancel(ctx)
			cmd := exec.CommandContext(child, exe, "agenda-worker")
			cmd.Stderr = os.Stderr
			if err = cmd.Start(); err != nil {
				stopChild()
				continue
			}
			ch := make(chan error, 1)
			go func() { ch <- cmd.Wait() }()
			cancel = stopChild
			done = ch
			active = c.Agenda
		}
	}
}
