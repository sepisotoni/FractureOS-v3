package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type WeatherConfig struct {
	Enabled   bool    `json:"enabled"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Hot       float64 `json:"hot"`
	Cold      float64 `json:"cold"`
	Wind      float64 `json:"wind"`
}

func defaultWeather() WeatherConfig {
	return WeatherConfig{Latitude: -26.2041, Longitude: 28.0473, Hot: 35, Cold: 0, Wind: 60}
}
func validateIntegrations(w WeatherConfig, n NotificationConfig) error {
	for _, v := range []float64{w.Latitude, w.Longitude, w.Hot, w.Cold, w.Wind} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return errors.New("settings must be finite")
		}
	}
	if w.Latitude < -90 || w.Latitude > 90 || w.Longitude < -180 || w.Longitude > 180 || w.Cold < -100 || w.Hot > 70 || w.Cold >= w.Hot || w.Wind < 1 || w.Wind > 400 {
		return errors.New("invalid weather location or thresholds")
	}
	if len(n.Excluded) > 64 {
		return errors.New("at most 64 excluded apps")
	}
	for _, app := range n.Excluded {
		if len(app) > 160 {
			return errors.New("application name too long")
		}
	}
	return nil
}

// Lifecycle management stays in the hub; the HTTP client and decoder live only
// in the separate enabled/visible worker. Reaped before any replacement starts.
func weatherSupervisor(ctx context.Context, b *Broker) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done <-chan error
	var active WeatherConfig
	stop := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
			done = nil
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
		case <-ticker.C:
			c, e := readConfig()
			needed := e == nil && c.Weather.Enabled
			if needed {
				out, err := b.call("status")
				var s struct {
					Menu    int `json:"menu"`
					Weather int `json:"weather"`
				}
				needed = err == nil && json.Unmarshal([]byte(out), &s) == nil && s.Menu == 0 && s.Weather > 0
			}
			if !needed {
				stop()
				continue
			}
			if cancel != nil && active != c.Weather {
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
			cmd := exec.CommandContext(child, exe, "weather-worker")
			cmd.Stderr = os.Stderr
			if err = cmd.Start(); err != nil {
				stopChild()
				continue
			}
			ch := make(chan error, 1)
			go func() { ch <- cmd.Wait() }()
			cancel = stopChild
			done = ch
			active = c.Weather
		}
	}
}

type weatherCurrent struct {
	Time int64    `json:"time"`
	Temp *float64 `json:"temperature_2m"`
	Code *int     `json:"weather_code"`
	Gust *float64 `json:"wind_gusts_10m"`
}

func fetchWeather(ctx context.Context, w WeatherConfig) (weatherCurrent, error) {
	var body struct {
		Current weatherCurrent `json:"current"`
	}
	q := url.Values{"latitude": {strconv.FormatFloat(w.Latitude, 'f', 4, 64)}, "longitude": {strconv.FormatFloat(w.Longitude, 'f', 4, 64)}, "current": {"temperature_2m,weather_code,wind_gusts_10m"}, "timeformat": {"unixtime"}, "forecast_days": {"1"}}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.open-meteo.com/v1/forecast?"+q.Encode(), nil)
	if e != nil {
		return body.Current, e
	}
	req.Header.Set("User-Agent", "FractureOS/"+version)
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("weather redirect refused") }}
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return body.Current, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return body.Current, fmt.Errorf("weather service: HTTP %d", res.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 32769))
	if e != nil {
		return body.Current, e
	}
	if len(data) > 32768 {
		return body.Current, errors.New("weather response too large")
	}
	if e = json.Unmarshal(data, &body); e != nil {
		return body.Current, e
	}
	v := body.Current
	if v.Temp == nil || v.Code == nil || v.Gust == nil || *v.Temp < -100 || *v.Temp > 70 || *v.Code < 0 || *v.Code > 99 || *v.Gust < 0 || *v.Gust > 500 || time.Since(time.Unix(v.Time, 0)) > 90*time.Minute || time.Until(time.Unix(v.Time, 0)) > 10*time.Minute {
		return v, errors.New("missing, invalid or stale weather")
	}
	return v, nil
}
func weatherWorker() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var current weatherCurrent
	var fetched time.Time
	var lastError time.Time
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		c, e := readConfig()
		if e != nil {
			return e
		}
		if !c.Weather.Enabled {
			return nil
		}
		// Disk cache prevents rapid menu open/close cycles from refetching the provider.
		if fetched.IsZero() {
			var cache struct {
				At      time.Time
				Config  WeatherConfig
				Current weatherCurrent
			}
			if data, err := os.ReadFile(filepathCache()); err == nil && len(data) < 4096 && json.Unmarshal(data, &cache) == nil && cache.Config == c.Weather && time.Since(cache.At) < 15*time.Minute && time.Since(cache.At) >= 0 && cache.Current.Temp != nil && cache.Current.Code != nil && cache.Current.Gust != nil {
				current = cache.Current
				fetched = cache.At
			}
		}
		if time.Since(fetched) >= 15*time.Minute && time.Since(lastError) >= 2*time.Minute {
			v, err := fetchWeather(ctx, c.Weather)
			if err == nil {
				current = v
				fetched = time.Now()
				data, _ := json.Marshal(struct {
					At      time.Time
					Config  WeatherConfig
					Current weatherCurrent
				}{fetched, c.Weather, current})
				_ = os.WriteFile(filepathCache(), data, 0600)
			} else {
				lastError = time.Now()
				fmt.Fprintln(os.Stderr, "weather:", err)
			}
		}
		if current.Temp != nil && time.Since(fetched) < 30*time.Minute {
			code := *current.Code
			unusual := code >= 95 || code == 65 || code == 66 || code == 67 || code == 75 || code == 82 || code == 86 || *current.Temp >= c.Weather.Hot || *current.Temp <= c.Weather.Cold || *current.Gust >= c.Weather.Wind
			flag := 0
			if unusual {
				flag = 1
			}
			_, _ = hubCall(c, fmt.Sprintf("weather-state %d,%d,%d", code, int(math.Round(*current.Temp*10)), flag))
		} else {
			_, _ = hubCall(c, "weather-clear")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func filepathCache() string { return configDir() + string(os.PathSeparator) + "weather-cache.json" }
