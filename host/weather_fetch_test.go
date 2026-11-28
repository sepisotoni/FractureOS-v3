package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func weatherFixture(now time.Time, temp string) string {
	hours, days := make([]string, 3), make([]string, 3)
	for i := range hours {
		hours[i] = fmt.Sprint(now.Add(time.Duration(i) * time.Hour).Unix())
		days[i] = fmt.Sprint(now.Add(time.Duration(i) * 24 * time.Hour).Unix())
	}
	return fmt.Sprintf(`{"utc_offset_seconds":7200,
"current":{"time":%d,"temperature_2m":%s,"weather_code":3,"wind_gusts_10m":18.5},
"hourly":{"time":[%s],"temperature_2m":[20,21,22],"weather_code":[1,2,3],"precipitation_probability":[0,10,20]},
"daily":{"time":[%s],"temperature_2m_min":[10,11,12],"temperature_2m_max":[22,23,24],"weather_code":[1,2,3],"precipitation_probability_max":[5,10,15]}}`,
		now.Unix(), temp, strings.Join(hours, ","), strings.Join(days, ","))
}

func useWeatherServer(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	old := weatherForecastURL
	weatherForecastURL = srv.URL + "/v1/forecast"
	t.Cleanup(func() { weatherForecastURL = old; srv.Close() })
}

func TestFetchWeatherParsesAndSendsCoordinates(t *testing.T) {
	var mu sync.Mutex
	var gotQuery, gotUA string
	now := time.Now()
	useWeatherServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotQuery, gotUA = r.URL.RawQuery, r.Header.Get("User-Agent")
		mu.Unlock()
		w.Write([]byte(weatherFixture(now, "21.4")))
	})
	got, err := fetchWeather(context.Background(), WeatherConfig{Latitude: -26.2041, Longitude: 28.0473})
	if err != nil {
		t.Fatal(err)
	}
	if got.Temp == nil || *got.Temp != 21.4 || got.Code == nil || *got.Code != 3 {
		t.Fatalf("current not parsed: %+v", got)
	}
	if got.Forecast == nil || len(got.Forecast.Hours) != 3 || len(got.Forecast.Days) != 3 || got.Forecast.Offset != 7200 {
		t.Fatalf("forecast not built: %+v", got.Forecast)
	}
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(gotQuery, "latitude=-26.2041") || !strings.Contains(gotQuery, "longitude=28.0473") || !strings.Contains(gotQuery, "forecast_days=7") {
		t.Fatalf("query = %s", gotQuery)
	}
	if !strings.HasPrefix(gotUA, "FractureOS/") {
		t.Fatalf("User-Agent = %q", gotUA)
	}
}

func TestFetchWeatherRejectsBadResponses(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"http error", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 503) }},
		{"not json", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) }},
		{"missing temperature", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(weatherFixture(now, "null"))) }},
		{"implausible temperature", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(weatherFixture(now, "99"))) }},
		{"stale reading", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(weatherFixture(now.Add(-3*time.Hour), "20")))
		}},
		{"oversize", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(strings.Repeat(" ", 40000))) }},
		{"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://127.0.0.1:1/", 302) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useWeatherServer(t, tc.h)
			if _, err := fetchWeather(context.Background(), WeatherConfig{Latitude: 1, Longitude: 1}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestFetchWeatherHonoursContext(t *testing.T) {
	release := make(chan struct{})
	useWeatherServer(t, func(w http.ResponseWriter, r *http.Request) { <-release })
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := fetchWeather(ctx, WeatherConfig{Latitude: 1, Longitude: 1}); err == nil {
		t.Fatal("expected a context error")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("fetch did not stop when the context ended")
	}
}
