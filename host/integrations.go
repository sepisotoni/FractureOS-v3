package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type NotificationConfig struct {
	Enabled  bool     `json:"enabled"`
	Excluded []string `json:"excluded"`
}
type Notification struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Body  string `json:"body"`
}
type IntegrationSettings struct {
	Operation     string             `json:"operation"`
	Weather       WeatherConfig      `json:"weather"`
	Notifications NotificationConfig `json:"notifications"`
	HubMetrics    bool               `json:"hub_metrics"`
}

func registerIntegrations(mux *http.ServeMux, b *Broker, token string) {
	var mu sync.Mutex
	var lastNotice time.Time
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return false
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return false
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		return true
	}
	mux.HandleFunc("/api/integrations", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var q IntegrationSettings
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&q) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		c, e := readConfig()
		if e != nil {
			http.Error(w, "cannot read settings", 500)
			return
		}
		switch q.Operation {
		case "get":
		case "set":
			c.Weather = q.Weather
			c.Notifications = q.Notifications
			c.HubMetrics = q.HubMetrics
			if e = saveConfig(c); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			if !c.Weather.Enabled {
				_, _ = b.call("weather-clear")
			}
		default:
			http.Error(w, "operation must be get or set", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(IntegrationSettings{Weather: c.Weather, Notifications: c.Notifications, HubMetrics: c.HubMetrics})
	})
	mux.HandleFunc("/api/notifications", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var q Notification
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&q) != nil || len(q.App) == 0 || len(q.App) > 160 || len(q.Title) > 256 || len(q.Body) > 1024 {
			http.Error(w, "invalid notification", 400)
			return
		}
		c, e := readConfig()
		if e != nil {
			http.Error(w, "settings unavailable", 500)
			return
		}
		if !c.Notifications.Enabled {
			fmt.Fprint(w, "disabled")
			return
		}
		for _, app := range c.Notifications.Excluded {
			if strings.EqualFold(strings.TrimSpace(app), strings.TrimSpace(q.App)) {
				fmt.Fprint(w, "excluded")
				return
			}
		}
		mu.Lock()
		defer mu.Unlock()
		if time.Since(lastNotice) < 2*time.Second {
			http.Error(w, "notification rate limit", 429)
			return
		}
		status, err := b.call("status")
		if err != nil {
			http.Error(w, "device unavailable", 502)
			return
		}
		if strings.Contains(status, `"notifications":0`) {
			fmt.Fprint(w, "disabled on device")
			return
		}
		text := q.App + ": " + q.Title
		if q.Body != "" {
			text += " / " + q.Body
		}
		out, e := b.call("notice " + asciiHex(text))
		if e != nil {
			http.Error(w, "notification delivery failed", 502)
			return
		}
		lastNotice = time.Now()
		fmt.Fprint(w, out)
	})
}
func integrationGet(c Config) (IntegrationSettings, error) {
	var q IntegrationSettings
	out, e := hubRequest(c, "/api/integrations", map[string]string{"operation": "get"})
	if e != nil {
		return q, e
	}
	e = json.Unmarshal([]byte(out), &q)
	return q, e
}
func integrationSave(c Config, q IntegrationSettings) error {
	if e := validateIntegrations(q.Weather, q.Notifications); e != nil {
		return e
	}
	q.Operation = "set"
	_, e := hubRequest(c, "/api/integrations", q)
	return e
}
