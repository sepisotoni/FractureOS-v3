package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

type forecastHour struct {
	When             int64
	Temp, Code, Rain int
}
type forecastDay struct {
	When                  int64
	Low, High, Code, Rain int
}
type weatherForecast struct {
	Saved  int64
	Offset int
	Hours  []forecastHour
	Days   []forecastDay
}
type forecastArrays struct {
	Time      []int64    `json:"time"`
	Temp      []*float64 `json:"temperature_2m"`
	Code      []*int     `json:"weather_code"`
	Rain      []*int     `json:"precipitation_probability"`
	Low       []*float64 `json:"temperature_2m_min"`
	High      []*float64 `json:"temperature_2m_max"`
	DailyRain []*int     `json:"precipitation_probability_max"`
}

func buildForecast(hours, days forecastArrays, offset int) (*weatherForecast, error) {
	if offset < -43200 || offset > 50400 || len(hours.Time) > 24 || len(days.Time) > 7 {
		return nil, errors.New("invalid forecast bounds")
	}
	f := &weatherForecast{Saved: time.Now().Unix(), Offset: offset}
	probability := func(values []*int, i int) int {
		if i < len(values) && values[i] != nil && *values[i] >= 0 && *values[i] <= 100 {
			return *values[i]
		}
		return -1
	}
	validTime := func(t int64) bool { return t >= 1700000000 && t <= 2147483647 }
	for i, t := range hours.Time {
		if i >= len(hours.Temp) || hours.Temp[i] == nil || i >= len(hours.Code) || hours.Code[i] == nil || !validTime(t) {
			continue
		}
		temp, code := *hours.Temp[i], *hours.Code[i]
		if temp < -100 || temp > 70 || code < 0 || code > 99 {
			continue
		}
		f.Hours = append(f.Hours, forecastHour{t, int(math.Round(temp * 10)), code, probability(hours.Rain, i)})
	}
	for i, t := range days.Time {
		if i >= len(days.Low) || days.Low[i] == nil || i >= len(days.High) || days.High[i] == nil || i >= len(days.Code) || days.Code[i] == nil || !validTime(t) {
			continue
		}
		low, high, code := *days.Low[i], *days.High[i], *days.Code[i]
		if low < -100 || high > 70 || low > high || code < 0 || code > 99 {
			continue
		}
		f.Days = append(f.Days, forecastDay{t, int(math.Round(low * 10)), int(math.Round(high * 10)), code, probability(days.DailyRain, i)})
	}
	if len(f.Hours) == 0 || len(f.Days) == 0 {
		return nil, errors.New("forecast data unavailable")
	}
	return f, nil
}
func (f *weatherForecast) text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "FWE1 %d %d\n", f.Saved, f.Offset)
	for _, h := range f.Hours {
		fmt.Fprintf(&b, "H %d %d %d %d\n", h.When, h.Temp, h.Code, h.Rain)
	}
	for _, d := range f.Days {
		fmt.Fprintf(&b, "D %d %d %d %d %d\n", d.When, d.Low, d.High, d.Code, d.Rain)
	}
	return b.String()
}
func registerForecast(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/forecast", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		var f weatherForecast
		if json.NewDecoder(r.Body).Decode(&f) != nil || len(f.Hours) > 24 || len(f.Days) > 7 || len(f.Hours) == 0 || len(f.Days) == 0 || f.Offset < -43200 || f.Offset > 50400 || time.Since(time.Unix(f.Saved, 0)) > time.Hour || time.Until(time.Unix(f.Saved, 0)) > time.Minute {
			http.Error(w, "invalid forecast", 400)
			return
		}
		for _, h := range f.Hours {
			if h.When < 1700000000 || h.When > 2147483647 || h.Temp < -1000 || h.Temp > 700 || h.Code < 0 || h.Code > 99 || h.Rain < -1 || h.Rain > 100 {
				http.Error(w, "invalid hour", 400)
				return
			}
		}
		for _, d := range f.Days {
			if d.When < 1700000000 || d.When > 2147483647 || d.Low < -1000 || d.High > 700 || d.Low > d.High || d.Code < 0 || d.Code > 99 || d.Rain < -1 || d.Rain > 100 {
				http.Error(w, "invalid day", 400)
				return
			}
		}
		if e := uploadDevicePayload(b.device, []byte(f.text()), 2); e != nil {
			http.Error(w, "forecast transfer failed", 502)
			return
		}
		if _, e := b.call("forecast-reload"); e != nil {
			http.Error(w, "forecast activation failed", 502)
			return
		}
		fmt.Fprint(w, "OK")
	})
}
