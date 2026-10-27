package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type lyricLine struct {
	At   float64 `json:"at"`
	Text string  `json:"text"`
}
type lyricsRecord struct {
	Title        string `json:"trackName"`
	Artist       string `json:"artistName"`
	Plain        string `json:"plainLyrics"`
	Synced       string `json:"syncedLyrics"`
	Instrumental bool   `json:"instrumental"`
}
type lyricsSession struct {
	PositionMS int64         `json:"position_ms"`
	Title      string        `json:"title"`
	Artist     string        `json:"artist"`
	Album      string        `json:"album"`
	Duration   int           `json:"duration"`
	Position   int           `json:"position"`
	Playing    bool          `json:"playing"`
	View       bool          `json:"view"`
	Page       int           `json:"page"`
	Provided   *lyricsRecord `json:"provided,omitempty"`
	Enabled    bool          `json:"enabled"`
}
type lyricsRequest struct {
	Lines     []lyricLine `json:"lines,omitempty"`
	Operation string      `json:"operation"`
	Title     string      `json:"title"`
	Artist    string      `json:"artist"`
	Previous  string      `json:"previous"`
	Current   string      `json:"current"`
	Next      string      `json:"next"`
	Message   string      `json:"message"`
	Plain     bool        `json:"plain"`
	LRC       string      `json:"lrc"`
	Text      string      `json:"text"`
}

func (b *Broker) lyricSession() (lyricsSession, error) {
	raw, e := b.call("status")
	if e != nil {
		return lyricsSession{}, e
	}
	var status mediaDeviceStatus
	if e = json.Unmarshal([]byte(raw), &status); e != nil {
		return lyricsSession{}, e
	}
	c, e := readConfig()
	if e != nil {
		return lyricsSession{}, e
	}
	b.mediaMu.Lock()
	defer b.mediaMu.Unlock()
	s := b.mediaLatest
	s.Enabled = c.LyricsEnabled
	s.View = status.LyricsView
	s.Page = status.LyricsPage
	if time.Since(b.mediaReceived) > 20*time.Second {
		return lyricsSession{View: s.View, Enabled: s.Enabled}, nil
	}
	if s.Playing {
		s.PositionMS += time.Since(b.mediaReceived).Milliseconds()
		s.Position = int(s.PositionMS / 1000)
		if s.Position > s.Duration {
			s.Position = s.Duration
			s.PositionMS = int64(s.Duration) * 1000
		}
	}
	s.Provided = b.providedLyrics
	return s, nil
}
func registerLyrics(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/lyrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
		var q lyricsRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&q) != nil || len(q.Title) > 512 || len(q.Artist) > 512 || len(q.LRC) > 65536 || len(q.Text) > 65536 {
			http.Error(w, "invalid lyrics request", 400)
			return
		}
		s, e := b.lyricSession()
		if e != nil {
			http.Error(w, "device unavailable", 502)
			return
		}
		if q.Operation == "session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(s)
			return
		}
		if q.Title != s.Title || q.Artist != s.Artist || q.Title == "" {
			http.Error(w, "track changed", 409)
			return
		}
		if q.Operation == "clear-provided" {
			b.mediaMu.Lock()
			b.providedLyrics = nil
			b.mediaMu.Unlock()
			fmt.Fprint(w, "OK")
			return
		}
		if !s.Enabled {
			http.Error(w, "lyrics disabled in hub settings", 403)
			return
		}
		if q.Operation == "provide" {
			record := &lyricsRecord{Title: q.Title, Artist: q.Artist, Plain: q.Text, Synced: q.LRC}
			b.mediaMu.Lock()
			if b.mediaLatest.Title != q.Title || b.mediaLatest.Artist != q.Artist {
				b.mediaMu.Unlock()
				http.Error(w, "track changed", 409)
				return
			}
			b.providedLyrics = record
			b.mediaMu.Unlock()
			fmt.Fprint(w, "OK")
			return
		}
		if q.Operation == "schedule" {
			if !s.View || len(q.Lines) < 1 || len(q.Lines) > 12 {
				http.Error(w, "invalid lyrics schedule", 400)
				return
			}
			identity := fnv.New32a()
			title, _ := hex.DecodeString(asciiHex(s.Title))
			artist, _ := hex.DecodeString(asciiHex(s.Artist))
			identity.Write(title)
			identity.Write([]byte("|"))
			identity.Write(artist)
			commands := []string{fmt.Sprintf("lyrics-begin %d,%d", len(q.Lines), identity.Sum32())}
			previous := -1.0
			for _, line := range q.Lines {
				if line.At < 0 || line.At > 86400 || line.At < previous || len(line.Text) > 512 {
					http.Error(w, "invalid lyrics timeline", 400)
					return
				}
				previous = line.At
				commands = append(commands, fmt.Sprintf("lyrics-line %d %s", int64(line.At*1000), asciiHex(line.Text)))
			}
			commands = append(commands, "lyrics-commit")
			if e = b.batch(commands); e != nil {
				http.Error(w, "lyrics delivery failed", 502)
				return
			}
			fmt.Fprint(w, "OK")
			return
		}
		if q.Operation != "update" || len(q.Previous) > 512 || len(q.Current) > 512 || len(q.Next) > 512 || len(q.Message) > 512 {
			http.Error(w, "invalid lyrics update", 400)
			return
		}
		if !s.View {
			http.Error(w, "lyrics view closed", 409)
			return
		}
		plain := 0
		if q.Plain {
			plain = 1
		}
		if e = b.batch([]string{"media-lyrics previous " + asciiHex(q.Previous), "media-lyrics current " + asciiHex(q.Current), "media-lyrics next " + asciiHex(q.Next), "media-lyrics message " + asciiHex(q.Message), fmt.Sprintf("lyrics-state %d", plain)}); e != nil {
			http.Error(w, "lyrics delivery failed", 502)
			return
		}
		fmt.Fprint(w, "OK")
	})
}
func lyricsSupervisor(ctx context.Context, b *Broker) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done <-chan error
	retry := time.Time{}
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
			retry = time.Now().Add(10 * time.Second)
		case <-ticker.C:
			b.mediaMu.Lock()
			hasMedia := b.mediaLatest.Title != "" && time.Since(b.mediaReceived) < 20*time.Second
			b.mediaMu.Unlock()
			if !hasMedia {
				stop()
				continue
			}
			s, e := b.lyricSession()
			if e != nil || !s.View || !s.Enabled {
				stop()
				continue
			}
			if cancel != nil || time.Now().Before(retry) {
				continue
			}
			exe, e := os.Executable()
			if e != nil {
				continue
			}
			child, stopChild := context.WithCancel(ctx)
			cmd := exec.CommandContext(child, exe, "lyrics-worker")
			cmd.Stderr = os.Stderr
			if e = cmd.Start(); e != nil {
				stopChild()
				continue
			}
			ch := make(chan error, 1)
			go func() { ch <- cmd.Wait() }()
			cancel = stopChild
			done = ch
		}
	}
}

var lrcStamp = regexp.MustCompile(`^\[(\d{1,3}):(\d{2}(?:\.\d{1,3})?)\]`)

func parseLyrics(record lyricsRecord) ([]lyricLine, bool) {
	var lines []lyricLine
	offset := 0.0
	for _, line := range strings.Split(record.Synced, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[offset:") {
			v := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(line), "[offset:"), "]")
			if ms, e := strconv.Atoi(v); e == nil && ms >= -60000 && ms <= 60000 {
				offset = float64(ms) / 1000
			}
		}
	}
	for _, line := range strings.Split(record.Synced, "\n") {
		if len(lines) >= 600 {
			break
		}
		var times []float64
		for {
			m := lrcStamp.FindStringSubmatch(line)
			if m == nil {
				break
			}
			mins, _ := strconv.Atoi(m[1])
			secs, _ := strconv.ParseFloat(m[2], 64)
			if secs < 60 {
				times = append(times, float64(mins)*60+secs)
			}
			line = line[len(m[0]):]
		}
		line = strings.TrimSpace(line)
		if len(line) > 512 {
			line = line[:512]
		}
		for _, at := range times {
			at += offset
			if at < 0 {
				at = 0
			}
			if len(lines) < 600 {
				lines = append(lines, lyricLine{At: at, Text: line})
			}
		}
	}
	if len(lines) > 0 {
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].At < lines[j].At })
		return lines, false
	}
	for _, text := range strings.Split(record.Plain, "\n") {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if len(text) > 512 {
			text = text[:512]
		}
		lines = append(lines, lyricLine{Text: text})
		if len(lines) >= 600 {
			break
		}
	}
	return lines, true
}
func fetchLyrics(ctx context.Context, s lyricsSession) (lyricsRecord, error) {
	var record lyricsRecord
	q := url.Values{"track_name": {s.Title}, "artist_name": {s.Artist}}
	if s.Album != "" {
		q.Set("album_name", s.Album)
	}
	if s.Duration > 0 && s.Duration <= 3600 {
		q.Set("duration", strconv.Itoa(s.Duration))
	}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://lrclib.net/api/get?"+q.Encode(), nil)
	if e != nil {
		return record, e
	}
	req.Header.Set("User-Agent", "FractureOS/"+version+" (https://github.com/sepisotoni/FractureOS-v3)")
	client := http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("lyrics redirect refused") }}
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return record, e
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return record, errors.New("lyrics not found for this track")
	}
	if res.StatusCode != 200 {
		return record, fmt.Errorf("lyrics service HTTP %d", res.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 256*1024+1))
	if e != nil {
		return record, e
	}
	if len(data) > 256*1024 {
		return record, errors.New("lyrics response too large")
	}
	if e = json.Unmarshal(data, &record); e != nil {
		return record, e
	}
	if len(record.Plain) > 65536 || len(record.Synced) > 65536 {
		return lyricsRecord{}, errors.New("lyrics too long")
	}
	return record, nil
}
func lyricsKey(s lyricsSession) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\n%s\n%s\n%d", s.Title, s.Artist, s.Album, s.Duration))))
}
func cachedLyrics(ctx context.Context, s lyricsSession) (lyricsRecord, error) {
	dir := filepath.Join(configDir(), "lyrics-cache")
	path := filepath.Join(dir, lyricsKey(s)+".json")
	var record lyricsRecord
	if info, e := os.Stat(path); e == nil && info.Size() <= 140000 && time.Since(info.ModTime()) < 7*24*time.Hour {
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &record) == nil {
			return record, nil
		}
	}
	record, e := fetchLyrics(ctx, s)
	if e != nil {
		return record, e
	}
	if os.MkdirAll(dir, 0700) == nil {
		data, _ := json.Marshal(record)
		if len(data) <= 140000 {
			_ = os.WriteFile(path, data, 0600)
		}
		entries, _ := os.ReadDir(dir)
		type item struct {
			path string
			at   time.Time
		}
		var files []item
		for _, entry := range entries {
			if entry.IsDir() || len(entry.Name()) != 69 || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if info, e := entry.Info(); e == nil {
				files = append(files, item{filepath.Join(dir, entry.Name()), info.ModTime()})
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].at.After(files[j].at) })
		if len(files) > 8 {
			for _, file := range files[8:] {
				_ = os.Remove(file.path)
			}
		}
	}
	return record, nil
}
func lyricsWorker() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	key, lastUpdate := "", ""
	windowStart, windowEnd := -1, -1
	var record lyricsRecord
	var lines []lyricLine
	plain := false
	message := ""
	var provided *lyricsRecord
	for {
		c, e := readConfig()
		if e != nil {
			return e
		}
		raw, e := hubRequest(c, "/api/lyrics", lyricsRequest{Operation: "session"})
		if e != nil {
			return e
		}
		var s lyricsSession
		if e = json.Unmarshal([]byte(raw), &s); e != nil {
			return e
		}
		if !s.View || !s.Enabled {
			return nil
		}
		providedChanged := (s.Provided == nil) != (provided == nil) || (s.Provided != nil && provided != nil && *provided != *s.Provided)
		if next := lyricsKey(s); next != key || providedChanged {
			key = next
			lastUpdate = ""
			windowStart, windowEnd = -1, -1
			message = ""
			provided = s.Provided
			if s.Provided != nil {
				record = *s.Provided
				e = nil
			} else if s.Title == "" {
				e = errors.New("Waiting for a playing track")
			} else {
				record, e = cachedLyrics(ctx, s)
			}
			if e != nil {
				lines = nil
				plain = false
				message = e.Error()
			} else if record.Instrumental {
				lines = nil
				plain = false
				message = "Instrumental track"
			} else {
				lines, plain = parseLyrics(record)
				if len(lines) == 0 {
					message = "Lyrics unavailable for this track"
				} else if plain {
					message = "Plain lyrics"
				} else {
					message = "Synced lyrics / LRCLIB"
				}
			}
		}
		if len(lines) > 0 && !plain {
			raw, e = hubRequest(c, "/api/lyrics", lyricsRequest{Operation: "session"})
			var current lyricsSession
			if e != nil || json.Unmarshal([]byte(raw), &current) != nil {
				return e
			}
			if lyricsKey(current) != key {
				continue
			}
			s = current
			index := sort.Search(len(lines), func(i int) bool { return lines[i].At > float64(s.PositionMS)/1000 }) - 1
			if windowStart < 0 || index < windowStart || index >= windowEnd-4 {
				start := index - 1
				if start < 0 {
					start = 0
				}
				end := start + 12
				if end > len(lines) {
					end = len(lines)
				}
				if start != windowStart || end != windowEnd {
					_, err := hubRequest(c, "/api/lyrics", lyricsRequest{Operation: "schedule", Title: s.Title, Artist: s.Artist, Lines: lines[start:end]})
					if err == nil {
						windowStart, windowEnd = start, end
					}
				}
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
			continue
		}
		q := lyricsRequest{Operation: "update", Title: s.Title, Artist: s.Artist, Plain: plain, Message: message}
		if len(lines) > 0 {
			index := -1
			if plain {
				index = s.Page*3 + 1
				if index >= len(lines) {
					index = len(lines) - 1
				}
			} else {
				for i, line := range lines {
					if line.At > float64(s.PositionMS)/1000 {
						break
					}
					index = i
				}
			}
			if index > 0 {
				q.Previous = lines[index-1].Text
			}
			if index >= 0 {
				q.Current = lines[index].Text
			}
			if index+1 < len(lines) {
				q.Next = lines[index+1].Text
			}
		}
		encoded, _ := json.Marshal(q)
		if string(encoded) != lastUpdate {
			if _, e = hubRequest(c, "/api/lyrics", q); e == nil {
				lastUpdate = string(encoded)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
