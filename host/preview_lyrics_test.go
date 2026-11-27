package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ---- preview asset download -----------------------------------------------

type previewFixture struct {
	assets map[string][]byte
	model  string
}

func newPreviewFixture(e *routeEnv, short string) *previewFixture {
	f := &previewFixture{assets: map[string][]byte{}}
	for i, kind := range []string{"atlas", "font", "notice", "wallpaper"} {
		d := make([]byte, previewAssetSize(kind))
		for j := range d {
			d[j] = byte(j*7 + i*31)
		}
		f.assets[kind] = d
	}
	f.model = fmt.Sprintf("FPR1\nface=aqua\natlas_hash=%d\nfont_hash=%d\nnotice_hash=%d\nwallpaper_hash=%d\nEND",
		assetHash(f.assets["atlas"]), assetHash(f.assets["font"]), assetHash(f.assets["notice"]), assetHash(f.assets["wallpaper"]))
	e.dev.hook = func(a []string) (string, bool) {
		switch {
		case a[0] == "home-state":
			return f.model, true
		case a[0] == "preview-asset" && len(a) == 3:
			at, _ := strconv.Atoi(a[2])
			d := f.assets[a[1]]
			if d == nil || at < 0 || at >= len(d) {
				return "", true
			}
			if a[1] == short {
				return "00", true // truncated chunk
			}
			end := at + 8192
			if end > len(d) {
				end = len(d)
			}
			return hex.EncodeToString(d[at:end]), true
		}
		return "", false
	}
	return f
}

func assetCalls(e *routeEnv, kind string) int {
	n := 0
	for _, c := range e.dev.commands() {
		if strings.HasPrefix(c, "preview-asset "+kind+" ") {
			n++
		}
	}
	return n
}

func TestHubPreviewAssetDownload(t *testing.T) {
	e := newRouteEnv(t)
	f := newPreviewFixture(e, "")
	h := assetHash(f.assets["notice"])
	code, body := hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", fmt.Sprintf(`{"operation":"asset","kind":"notice","hash":%d}`, h))
	var reply struct {
		Data []byte
		Hash uint32
	}
	if code != 200 || json.Unmarshal([]byte(body), &reply) != nil || string(reply.Data) != string(f.assets["notice"]) || reply.Hash != h {
		t.Fatalf("asset: %d len=%d", code, len(reply.Data))
	}
	// The asset changed on the device since the model was read: refuse so the client refreshes.
	if code, _ = hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", fmt.Sprintf(`{"operation":"asset","kind":"notice","hash":%d}`, h+1)); code != 409 {
		t.Fatalf("stale hash = %d, want 409", code)
	}
	for _, bad := range []string{`{"operation":"asset","kind":"bogus","hash":1}`, `{"operation":"asset","kind":"notice","hash":0}`, `{"operation":"zzz"}`} {
		if code, _ = hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", bad); code != 400 {
			t.Errorf("%s = %d, want 400", bad, code)
		}
	}
	// A truncated chunk from the device must be an error, not corrupt pixels.
	f2 := newPreviewFixture(e, "notice")
	_ = f2
	if code, _ = hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", fmt.Sprintf(`{"operation":"asset","kind":"notice","hash":%d}`, h)); code != 502 {
		t.Fatalf("short asset = %d, want 502", code)
	}
}

func TestFetchHomePreviewDownloadsCachesAndReuses(t *testing.T) {
	e := newRouteEnv(t)
	f := newPreviewFixture(e, "")
	c, _ := readConfig()
	first, err := fetchHomePreview(c, homePreviewPacket{})
	if err != nil {
		t.Fatal(err)
	}
	for kind, got := range map[string][]byte{"atlas": first.Atlas, "font": first.Font, "notice": first.Notice, "wallpaper": first.Wallpaper} {
		if string(got) != string(f.assets[kind]) {
			t.Errorf("%s asset differs from the device's (%d bytes)", kind, len(got))
		}
	}
	if assetCalls(e, "atlas") != 21 { // ceil(170000/8192) chunks
		t.Fatalf("atlas chunks = %d, want 21", assetCalls(e, "atlas"))
	}
	e.dev.reset()
	if _, err = fetchHomePreview(c, first); err != nil || assetCalls(e, "atlas")+assetCalls(e, "wallpaper") != 0 {
		t.Fatalf("unchanged hashes must reuse the previous packet: err=%v cmds=%d", err, len(e.dev.commands()))
	}
	if _, err = fetchHomePreview(c, homePreviewPacket{}); err != nil || assetCalls(e, "atlas") != 0 {
		t.Fatalf("disk cache must serve a fresh client: err=%v atlas calls=%d", err, assetCalls(e, "atlas"))
	}
	// A corrupted cache entry is detected by its hash and re-fetched.
	matches, _ := filepath.Glob(filepath.Join(configDir(), "preview-cache", "notice-*.bin"))
	if len(matches) != 1 {
		t.Fatalf("notice cache files = %v", matches)
	}
	raw, _ := os.ReadFile(matches[0])
	raw[0] ^= 0xff
	_ = os.WriteFile(matches[0], raw, 0600)
	e.dev.reset()
	p, err := fetchHomePreview(c, homePreviewPacket{})
	if err != nil || string(p.Notice) != string(f.assets["notice"]) || assetCalls(e, "notice") == 0 {
		t.Fatalf("corrupt cache not repaired: err=%v calls=%d", err, assetCalls(e, "notice"))
	}
}

func TestFetchHomePreviewDegradesAndValidates(t *testing.T) {
	e := newRouteEnv(t)
	f := newPreviewFixture(e, "wallpaper")
	c, _ := readConfig()
	p, err := fetchHomePreview(c, homePreviewPacket{})
	if err != nil || p.Wallpaper != nil || p.WallpaperHash != 0 || len(p.Atlas) != 170000 {
		t.Fatalf("wallpaper failure must degrade to no wallpaper, not fail the preview: err=%v wp=%d hash=%d", err, len(p.Wallpaper), p.WallpaperHash)
	}
	f.model = strings.Replace(f.model, fmt.Sprintf("atlas_hash=%d", assetHash(f.assets["atlas"])), "atlas_hash=0", 1)
	if _, err = fetchHomePreview(c, homePreviewPacket{}); err == nil || !strings.Contains(err.Error(), "home face is unavailable") {
		t.Fatalf("no face must be an error, got %v", err)
	}
	f.model = "garbage"
	if _, err = fetchHomePreview(c, homePreviewPacket{}); err == nil || !strings.Contains(err.Error(), "invalid home preview model") {
		t.Fatalf("malformed model must be refused, got %v", err)
	}
}

// ---- lyrics hub operations -------------------------------------------------

func lyricsEnv(t *testing.T) *routeEnv {
	e := newRouteEnv(t)
	e.dev.replies["status"] = `{"lyrics_view":true,"lyrics_page":0}`
	snap := `{"title":"Song","artist":"Band","album":"Alb","playing":true,"position":10,"duration":200,"volume":5,"volume_ok":true,"play_ok":true}`
	if code, body := hubDo(t, e, "POST", "/api/media", hubToken, "application/json", snap); code != 200 {
		t.Fatalf("media: %d %s", code, body)
	}
	e.dev.reset()
	return e
}

func lyr(t *testing.T, e *routeEnv, body string) (int, string) {
	t.Helper()
	return hubDo(t, e, "POST", "/api/lyrics", hubToken, "application/json", body)
}

func TestHubLyricsSessionProvideAndClear(t *testing.T) {
	e := lyricsEnv(t)
	code, body := lyr(t, e, `{"operation":"session"}`)
	var s lyricsSession
	if code != 200 || json.Unmarshal([]byte(body), &s) != nil || s.Title != "Song" || s.Artist != "Band" || !s.Enabled || !s.View || s.Provided != nil {
		t.Fatalf("session: %d %s", code, body)
	}
	if code, _ = lyr(t, e, `{"operation":"provide","title":"Other","artist":"Band","text":"x"}`); code != 409 {
		t.Fatalf("provide for a different track = %d, want 409", code)
	}
	if code, body = lyr(t, e, `{"operation":"provide","title":"Song","artist":"Band","text":"la la","lrc":"[00:01.00]la"}`); code != 200 || body != "OK" {
		t.Fatalf("provide: %d %s", code, body)
	}
	_, body = lyr(t, e, `{"operation":"session"}`)
	_ = json.Unmarshal([]byte(body), &s)
	if s.Provided == nil || s.Provided.Plain != "la la" {
		t.Fatalf("provided lyrics not in session: %s", body)
	}
	if code, _ = lyr(t, e, `{"operation":"clear-provided","title":"Song","artist":"Band"}`); code != 200 {
		t.Fatalf("clear = %d", code)
	}
	_, body = lyr(t, e, `{"operation":"session"}`)
	s = lyricsSession{}
	_ = json.Unmarshal([]byte(body), &s)
	if s.Provided != nil {
		t.Fatal("clear-provided left the lyrics in place")
	}
	c, _ := readConfig()
	c.LyricsEnabled = false
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	if code, _ = lyr(t, e, `{"operation":"provide","title":"Song","artist":"Band","text":"x"}`); code != 403 {
		t.Fatalf("lyrics disabled in settings must give 403, got %d", code)
	}
	if code, _ = lyr(t, e, `{"operation":"session"}`); code != 200 {
		t.Fatal("session must still answer while disabled")
	}
}

func TestHubLyricsScheduleAndUpdateReachDevice(t *testing.T) {
	e := lyricsEnv(t)
	ok := `{"operation":"schedule","title":"Song","artist":"Band","lines":[{"at":1.5,"text":"first"},{"at":3,"text":"second"}]}`
	if code, body := lyr(t, e, ok); code != 200 || body != "OK" {
		t.Fatalf("schedule: %d %s", code, body)
	}
	var cmds []string
	for _, c := range e.dev.commands() {
		if c != "status" { // the session lookup reads device status first
			cmds = append(cmds, c)
		}
	}
	if len(cmds) != 4 || !strings.HasPrefix(cmds[0], "lyrics-begin 2,") || !strings.HasPrefix(cmds[1], "lyrics-line 1500 ") || !strings.HasPrefix(cmds[2], "lyrics-line 3000 ") || cmds[3] != "lyrics-commit" {
		t.Fatalf("schedule commands: %v", cmds)
	}
	many := `{"operation":"schedule","title":"Song","artist":"Band","lines":[` + strings.TrimSuffix(strings.Repeat(`{"at":1,"text":"x"},`, 13), ",") + `]}`
	for name, bad := range map[string]string{
		"decreasing times": `{"operation":"schedule","title":"Song","artist":"Band","lines":[{"at":5,"text":"a"},{"at":1,"text":"b"}]}`,
		"no lines":         `{"operation":"schedule","title":"Song","artist":"Band","lines":[]}`,
		"13 lines":         many,
		"time past a day":  `{"operation":"schedule","title":"Song","artist":"Band","lines":[{"at":90000,"text":"a"}]}`,
	} {
		if code, _ := lyr(t, e, bad); code != 400 {
			t.Errorf("%s = %d, want 400", name, code)
		}
	}
	e.dev.reset()
	up := `{"operation":"update","title":"Song","artist":"Band","previous":"p","current":"c","next":"n","plain":true}`
	if code, body := lyr(t, e, up); code != 200 || body != "OK" || !e.dev.saw("media-lyrics previous") || !e.dev.saw("media-lyrics current") || !e.dev.saw("media-lyrics next") {
		t.Fatalf("update: %d %s cmds=%v", code, body, e.dev.commands())
	}
	if code, _ := lyr(t, e, `{"operation":"update","title":"Song","artist":"Band","current":"`+strings.Repeat("a", 600)+`"}`); code != 400 {
		t.Fatalf("oversized line = %d, want 400", code)
	}
	if code, _ := lyr(t, e, `{"operation":"nonsense","title":"Song","artist":"Band"}`); code != 400 {
		t.Fatalf("unknown operation = %d, want 400", code)
	}
	if code, _ := lyr(t, e, `{"operation":"update","title":"Changed","artist":"Band"}`); code != 409 {
		t.Fatalf("track changed = %d, want 409", code)
	}
	e.dev.replies["status"] = `{"lyrics_view":false}`
	e.broker.mu.Lock()
	e.broker.cachedAt = time.Time{} // Broker caches status for 1s by design
	e.broker.mu.Unlock()
	if code, _ := lyr(t, e, up); code != 409 {
		t.Fatalf("lyrics view closed = %d, want 409", code)
	}
}

// ---- LRCLIB search path ----------------------------------------------------

func fakeLRCLIB(t *testing.T, handler http.HandlerFunc) *int32 {
	t.Helper()
	var hits int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		handler(w, r)
	}))
	oldURL, oldTr := lrclibURL, lyricsTransport
	lrclibURL, lyricsTransport = srv.URL+"/api/get", srv.Client().Transport
	t.Cleanup(func() { srv.Close(); lrclibURL, lyricsTransport = oldURL, oldTr })
	return &hits
}

func TestFetchLyricsResponses(t *testing.T) {
	wpEnv(t)
	var gotQuery string
	record := `{"trackName":"Song","artistName":"Band","plainLyrics":"la la","syncedLyrics":"[00:01.00]la","instrumental":false}`
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{"found", func(w http.ResponseWriter, r *http.Request) { gotQuery = r.URL.RawQuery; fmt.Fprint(w, record) }, ""},
		{"not found", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) }, "not found for this track"},
		{"service error", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "x", 503) }, "HTTP 503"},
		{"malformed", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "{") }, "unexpected end"},
		{"too large", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(strings.Repeat("a", 300*1024))) }, "too large"},
		{"lyrics too long", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"plainLyrics":%q}`, strings.Repeat("a", 70000))
		}, "too long"},
		{"redirect refused", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/elsewhere", 302) }, "redirect refused"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeLRCLIB(t, c.handler)
			rec, err := fetchLyrics(context.Background(), lyricsSession{Title: "Song", Artist: "Band", Album: "Alb", Duration: 200})
			if c.wantErr == "" {
				if err != nil || rec.Plain != "la la" || rec.Synced == "" {
					t.Fatalf("rec=%+v err=%v", rec, err)
				}
				for _, want := range []string{"track_name=Song", "artist_name=Band", "album_name=Alb", "duration=200"} {
					if !strings.Contains(gotQuery, want) {
						t.Errorf("query %q lacks %s", gotQuery, want)
					}
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("want %q, got %v", c.wantErr, err)
			}
		})
	}
	// Implausible durations are not sent as a filter.
	fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) { gotQuery = r.URL.RawQuery; fmt.Fprint(w, record) })
	if _, err := fetchLyrics(context.Background(), lyricsSession{Title: "Song", Artist: "Band", Duration: 99999}); err != nil || strings.Contains(gotQuery, "duration") {
		t.Fatalf("duration must be omitted: %q %v", gotQuery, err)
	}
}

func TestCachedLyricsCachesExpiresAndPrunes(t *testing.T) {
	wpEnv(t)
	hits := fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"trackName":%q,"plainLyrics":"text"}`, r.URL.Query().Get("track_name"))
	})
	s := lyricsSession{Title: "Song", Artist: "Band", Duration: 200}
	for i := 0; i < 2; i++ {
		if rec, err := cachedLyrics(context.Background(), s); err != nil || rec.Plain != "text" {
			t.Fatalf("call %d: %+v %v", i, rec, err)
		}
	}
	if *hits != 1 {
		t.Fatalf("second lookup must come from the cache, server hits = %d", *hits)
	}
	path := filepath.Join(configDir(), "lyrics-cache", lyricsKey(s)+".json")
	old := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := cachedLyrics(context.Background(), s); err != nil || *hits != 2 {
		t.Fatalf("entries older than 7 days must be refetched: hits=%d err=%v", *hits, err)
	}
	for i := 0; i < 12; i++ {
		_, _ = cachedLyrics(context.Background(), lyricsSession{Title: fmt.Sprintf("T%d", i), Artist: "A"})
	}
	entries, _ := os.ReadDir(filepath.Join(configDir(), "lyrics-cache"))
	if len(entries) > 8 {
		t.Fatalf("cache must be bounded to 8 entries, has %d", len(entries))
	}
	fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	missing := lyricsSession{Title: "Nope", Artist: "Nobody"}
	if _, err := cachedLyrics(context.Background(), missing); err == nil {
		t.Fatal("not-found must be an error")
	}
	if _, err := os.Stat(filepath.Join(configDir(), "lyrics-cache", lyricsKey(missing)+".json")); err == nil {
		t.Fatal("failures must not be cached")
	}
}
