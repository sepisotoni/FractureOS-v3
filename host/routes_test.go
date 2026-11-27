package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type routeEnv struct {
	dev    *fakeShellDevice
	broker *Broker
	hub    *httptest.Server
	ui     *httptest.Server
}

const hubToken = "hubtok"
const uiSession = "sess"

// newRouteEnv wires the real UI mux -> real pinned-TLS hubRequest -> real hub
// mux -> real telnet client -> fake shell device, all on loopback.
func newRouteEnv(t *testing.T) *routeEnv {
	t.Helper()
	wpEnv(t) // isolated config dir, repo-root cwd, default config
	e := &routeEnv{dev: startFakeShellDevice(t), broker: &Broker{device: "127.0.0.1"}}
	e.hub = httptest.NewUnstartedServer(newHubMux(e.broker, hubToken))
	e.hub.StartTLS()
	t.Cleanup(e.hub.Close)
	sum := sha256.Sum256(e.hub.Certificate().Raw)
	c, err := readConfig()
	if err != nil {
		t.Fatal(err)
	}
	c.Hub, c.Token, c.Fingerprint, c.Device = e.hub.URL, hubToken, hex.EncodeToString(sum[:]), "127.0.0.1"
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	e.ui = httptest.NewServer(newSettingsMux(uiSession, func() {}))
	t.Cleanup(e.ui.Close)
	return e
}

func uiDo(t *testing.T, e *routeEnv, method, path, body, session string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.ui.URL+path, strings.NewReader(body))
	if session != "" {
		req.Header.Set("X-Fracture-Session", session)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, strings.TrimSpace(string(b))
}

func hubClient(e *routeEnv) *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 20 * time.Second}
}

func hubDo(t *testing.T, e *routeEnv, method, path, token, ctype, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.hub.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := hubClient(e).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, strings.TrimSpace(string(b))
}

// ---- settings UI routes ----------------------------------------------------

func TestUIRoutesRequireSessionAndServePage(t *testing.T) {
	e := newRouteEnv(t)
	for _, p := range []string{"/changelog", "/update", "/integrations", "/wallpaper", "/notification-test", "/config", "/command", "/nope"} {
		if code, _ := uiDo(t, e, "GET", p, "", ""); code != 401 {
			t.Errorf("%s without session = %d, want 401", p, code)
		}
		if code, _ := uiDo(t, e, "GET", p, "", "wrong"); code != 401 {
			t.Errorf("%s with wrong session = %d, want 401", p, code)
		}
	}
	res, err := http.Get(e.ui.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), "data-section=\"wallpaper\"") {
		t.Fatalf("page: %d", res.StatusCode)
	}
	if res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("security headers missing")
	}
	if code, _ := uiDo(t, e, "GET", "/nope", "", uiSession); code != 404 {
		t.Fatalf("unknown route = %d", code)
	}
}

func TestUIChangelogAndMethods(t *testing.T) {
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "GET", "/changelog", "", uiSession)
	var entries []map[string]any
	if code != 200 || json.Unmarshal([]byte(body), &entries) != nil || len(entries) == 0 {
		t.Fatalf("changelog: %d %.80s", code, body)
	}
	for path, method := range map[string]string{"/changelog": "POST", "/update": "GET", "/notification-test": "GET", "/command": "GET"} {
		if code, _ := uiDo(t, e, method, path, "", uiSession); code != 405 {
			t.Errorf("%s %s = %d, want 405", method, path, code)
		}
	}
}

func TestUIConfigRoundTripAndPreservesHubSettings(t *testing.T) {
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "GET", "/config", "", uiSession)
	var c Config
	if code != 200 || json.Unmarshal([]byte(body), &c) != nil || c.Token != "" {
		t.Fatalf("GET /config must hide the token: %d %.100s", code, body)
	}
	if code, _ = uiDo(t, e, "POST", "/config", `{"connection_mode":"bogus"}`, uiSession); code != 400 {
		t.Fatalf("bad mode = %d", code)
	}
	cur, _ := readConfig()
	cur.Weather.Enabled, cur.Weather.Latitude = true, 12.5
	if err := saveConfig(cur); err != nil {
		t.Fatal(err)
	}
	// An older form posts stale hub-only settings; they must not overwrite the saved ones.
	stale := cur
	stale.Weather.Enabled, stale.Weather.Latitude = false, 1
	stale.MediaEnabled = true
	raw, _ := json.Marshal(stale)
	if code, body = uiDo(t, e, "POST", "/config", string(raw), uiSession); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	after, _ := readConfig()
	if !after.Weather.Enabled || after.Weather.Latitude != 12.5 || !after.MediaEnabled || after.Token != hubToken {
		t.Fatalf("settings not preserved/saved: %+v", after)
	}
}

func TestUIDirectModeConfigGeneratesIdentity(t *testing.T) {
	// Host side of "TomTom plugged into this PC": direct mode fills in the local
	// hub address, the USB device address and a pinned certificate.
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "POST", "/config", `{"connection_mode":"direct","device":"","hub":"","listen":"","token":""}`, uiSession)
	if code != 200 {
		t.Fatalf("direct save: %d %s", code, body)
	}
	c, _ := readConfig()
	if c.ConnectionMode != "direct" || c.Listen != "127.0.0.1:47831" || c.Hub != "https://127.0.0.1:47831" || c.Device != "192.168.101.115" {
		t.Fatalf("direct defaults wrong: %+v", c)
	}
	if c.Token == "" || len(c.Fingerprint) != 64 {
		t.Fatalf("identity not generated: token=%q fp=%q", c.Token, c.Fingerprint)
	}
	if effectiveConnectionMode(c) != "direct" {
		t.Fatal("effective mode must be direct")
	}
	for _, f := range []string{"hub.crt", "hub.key"} {
		if _, err := os.Stat(filepath.Join(configDir(), f)); err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
	}
}

func TestDirectModeLocalHubEndToEnd(t *testing.T) {
	// Generates the real identity, serves the hub exactly as direct mode does
	// (TLS 1.3 with hub.crt/hub.key on loopback) and drives it through the same
	// pinned client the Windows app uses.
	wpEnv(t)
	dev := startFakeShellDevice(t)
	c, _ := readConfig()
	c.ConnectionMode, c.Device = "direct", "127.0.0.1"
	if err := identity(&c); err != nil {
		t.Fatal(err)
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(configDir(), "hub.crt"), filepath.Join(configDir(), "hub.key"))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: newHubMux(&Broker{device: "127.0.0.1"}, c.Token), TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}
	go func() { _ = srv.ServeTLS(ln, "", "") }()
	t.Cleanup(func() { _ = srv.Close() })
	c.Hub = "https://" + ln.Addr().String()
	out, err := hubCall(c, "time 1767268800")
	if err != nil || out != "OK" || !dev.saw("time 1767268800") {
		t.Fatalf("direct hub call: %q %v cmds=%v", out, err, dev.commands())
	}
	bad := c
	bad.Fingerprint = strings.Repeat("0", 64)
	if _, err = hubCall(bad, "time 1767268800"); err == nil || !strings.Contains(err.Error(), "fingerprint") {
		t.Fatalf("wrong fingerprint must be refused, got %v", err)
	}
	bad = c
	bad.Token = "wrong"
	if _, err = hubCall(bad, "time 1767268800"); err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("wrong token must be refused, got %v", err)
	}
}

func TestUICommandThroughHubToDevice(t *testing.T) {
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "POST", "/command", `{"command":"time 1767268800"}`, uiSession)
	if code != 200 || body != "OK" || !e.dev.saw("time 1767268800") {
		t.Fatalf("command: %d %q cmds=%v", code, body, e.dev.commands())
	}
	e.dev.reset()
	code, body = uiDo(t, e, "POST", "/command", `{"command":"rm -rf /"}`, uiSession)
	if code != 502 || !strings.Contains(body, "unsupported command") || len(e.dev.commands()) != 0 {
		t.Fatalf("invalid command must be refused before the device: %d %q %v", code, body, e.dev.commands())
	}
	e.dev.mu.Lock()
	e.dev.failOn = "time"
	e.dev.mu.Unlock()
	if code, body = uiDo(t, e, "POST", "/command", `{"command":"time 1767268800"}`, uiSession); code != 502 || !strings.Contains(body, "device error") {
		t.Fatalf("device failure must surface: %d %q", code, body)
	}
	_ = e.dev.ln.Close()
	if code, _ = uiDo(t, e, "POST", "/command", `{"command":"time 1767268801"}`, uiSession); code != 502 {
		t.Fatalf("unreachable device = %d, want 502", code)
	}
}

func TestTimeSyncSendsCurrentUnixTime(t *testing.T) {
	wpEnv(t)
	dev := startFakeShellDevice(t)
	before := time.Now().Unix()
	if err := syncDeviceTime(&Broker{device: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	var sent int64
	for _, c := range dev.commands() {
		if _, err := fmt.Sscanf(c, "time %d", &sent); err == nil {
			break
		}
	}
	if sent < before || sent > time.Now().Unix() {
		t.Fatalf("time sync sent %d, want within [%d, now]", sent, before)
	}
	for _, bad := range []string{"time 1", "time 99999999999", "time abc", "time"} {
		if validateCommand(bad) == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	_ = dev.ln.Close()
	if err := syncDeviceTime(&Broker{device: "127.0.0.1"}); err == nil {
		t.Fatal("sync against an unreachable device must fail")
	}
}

func TestUIOldHubGetsActionableMessage(t *testing.T) {
	e := newRouteEnv(t)
	old := httptest.NewTLSServer(http.NewServeMux()) // knows no /api routes: Go's own 404
	t.Cleanup(old.Close)
	sum := sha256.Sum256(old.Certificate().Raw)
	c, _ := readConfig()
	c.Hub, c.Fingerprint = old.URL, hex.EncodeToString(sum[:])
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/wallpaper", "/integrations"} {
		code, body := uiDo(t, e, "GET", p, "", uiSession)
		if code != 502 || !strings.Contains(body, "older than this app") || strings.Contains(body, "404 page not found") {
			t.Errorf("%s: %d %q", p, code, body)
		}
	}
}

func TestUIIntegrationsThroughHub(t *testing.T) {
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "GET", "/integrations", "", uiSession)
	var s IntegrationSettings
	if code != 200 || json.Unmarshal([]byte(body), &s) != nil || s.Audio == nil || s.Agenda == nil {
		t.Fatalf("get: %d %.120s", code, body)
	}
	s.Weather.Enabled, s.Weather.Latitude, s.Weather.Longitude = true, 51.5, -0.1
	raw, _ := json.Marshal(s)
	if code, body = uiDo(t, e, "POST", "/integrations", string(raw), uiSession); code != 200 {
		t.Fatalf("set: %d %s", code, body)
	}
	c, _ := readConfig()
	if !c.Weather.Enabled || c.Weather.Latitude != 51.5 {
		t.Fatalf("weather not persisted: %+v", c.Weather)
	}
	s.Weather.Latitude = 123 // out of range
	raw, _ = json.Marshal(s)
	if code, body = uiDo(t, e, "POST", "/integrations", string(raw), uiSession); code != 502 || body == "" {
		t.Fatalf("invalid weather must be refused: %d %q", code, body)
	}
	c, _ = readConfig()
	if c.Weather.Latitude != 51.5 {
		t.Fatalf("invalid save changed config: %+v", c.Weather)
	}
	s.Weather.Latitude, s.Weather.Enabled = 51.5, false
	raw, _ = json.Marshal(s)
	uiDo(t, e, "POST", "/integrations", string(raw), uiSession)
	if !e.dev.saw("weather-clear") {
		t.Fatalf("disabling weather must clear the device: %v", e.dev.commands())
	}
}

func TestUINotificationTestIsWindowsOnly(t *testing.T) {
	e := newRouteEnv(t)
	code, body := uiDo(t, e, "POST", "/notification-test", "", uiSession)
	if code == 200 {
		t.Skip("notification test path is available on this platform; covered by Windows runs")
	}
	if code != 502 || body == "" {
		t.Fatalf("expected a clear 502 on unsupported platforms, got %d %q", code, body)
	}
}

func TestUIWallpaperThroughHubPushesToDevice(t *testing.T) {
	e := newRouteEnv(t)
	active := false
	e.dev.hook = func(a []string) (string, bool) {
		switch {
		case a[0] == "wallpaper" && len(a) > 1 && a[1] == "load":
			active = true
		case a[0] == "wallpaper" && len(a) > 1 && a[1] == "clear":
			active = false
		case a[0] == "status":
			return fmt.Sprintf(`{"wallpaper_active":%v}`, active), true
		case a[0] == "home-state":
			return "", false
		}
		return "", false
	}
	dry := fmt.Sprintf(`{"mode":"preset","preset":%q,"dry_run":true}`, presetA)
	code, body := uiDo(t, e, "POST", "/wallpaper", dry, uiSession)
	var res map[string]any
	_ = json.Unmarshal([]byte(body), &res)
	if code != 200 || res["dry_run"] != true || e.dev.file("/mnt/sdcard/fracture-v3/wallpaper.rgb") != nil || len(e.dev.commands()) != 0 {
		t.Fatalf("dry run must not touch the device: %d %.100s cmds=%v", code, body, e.dev.commands())
	}
	apply := fmt.Sprintf(`{"mode":"preset","preset":%q}`, presetA)
	code, body = uiDo(t, e, "POST", "/wallpaper", apply, uiSession)
	_ = json.Unmarshal([]byte(body), &res)
	data := e.dev.file("/mnt/sdcard/fracture-v3/wallpaper.rgb")
	if code != 200 || res["device_applied"] != true || len(data) != 153600 || !e.dev.saw("wallpaper load") {
		t.Fatalf("apply: %d applied=%v bytes=%d cmds=%v body=%.120s", code, res["device_applied"], len(data), e.dev.commands(), body)
	}
	c, _ := readConfig()
	if c.Wallpaper.Preset != presetA || c.Wallpaper.Applied == "" {
		t.Fatalf("selection/applied state not persisted: %+v", c.Wallpaper)
	}
	e.dev.reset()
	if code, body = uiDo(t, e, "POST", "/wallpaper", `{"operation":"sync"}`, uiSession); code != 200 || len(e.dev.commands()) > 2 {
		t.Fatalf("sync of an unchanged device must not re-upload: %d cmds=%v", code, e.dev.commands())
	}
	apply2 := fmt.Sprintf(`{"mode":"preset","preset":%q}`, presetB)
	uiDo(t, e, "POST", "/wallpaper", apply2, uiSession)
	if code, body = uiDo(t, e, "POST", "/wallpaper", `{"operation":"restore"}`, uiSession); code != 200 {
		t.Fatalf("restore: %d %s", code, body)
	}
	c, _ = readConfig()
	if c.Wallpaper.Preset != presetA {
		t.Fatalf("restore did not go back to the previous selection: %+v", c.Wallpaper)
	}
}

// ---- hub API routes --------------------------------------------------------

func TestHubRoutesRequireBearerToken(t *testing.T) {
	e := newRouteEnv(t)
	for _, p := range []string{"/api/command", "/api/media", "/api/preview", "/api/wallpaper", "/api/integrations", "/api/notifications", "/api/audio", "/api/agenda", "/api/forecast", "/api/lyrics"} {
		for _, tok := range []string{"", "wrong"} {
			if code, _ := hubDo(t, e, "POST", p, tok, "application/json", "{}"); code != 401 {
				t.Errorf("%s token=%q = %d, want 401", p, tok, code)
			}
		}
	}
	for _, p := range []string{"/api/command", "/api/preview", "/api/integrations", "/api/notifications", "/api/agenda", "/api/forecast", "/api/lyrics", "/api/media"} {
		if code, _ := hubDo(t, e, "GET", p, hubToken, "", ""); code != 405 {
			t.Errorf("GET %s = %d, want 405", p, code)
		}
	}
}

func TestHubCommandStrictness(t *testing.T) {
	e := newRouteEnv(t)
	cases := []struct {
		body string
		want int
	}{{`{"command":"time 1767268800"}`, 200}, {`{"command":"time 1"}`, 400}, {`{"command":"time 1767268800","extra":1}`, 400}, {`not json`, 400}, {`{"command":"` + strings.Repeat("a", 2000) + `"}`, 400}}
	for _, c := range cases {
		if code, body := hubDo(t, e, "POST", "/api/command", hubToken, "application/json", c.body); code != c.want {
			t.Errorf("%.40s = %d (%s), want %d", c.body, code, body, c.want)
		}
	}
}

func TestHubNotificationsFlow(t *testing.T) {
	e := newRouteEnv(t)
	post := func(body string) (int, string) {
		return hubDo(t, e, "POST", "/api/notifications", hubToken, "application/json", body)
	}
	if code, body := post(`{"app":"Mail","title":"Hi","body":"there","seconds":5}`); code != 200 || body != "disabled" {
		t.Fatalf("disabled by default: %d %q", code, body)
	}
	c, _ := readConfig()
	c.Notifications.Enabled, c.Notifications.Excluded = true, []string{"spam.app"}
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
	e.dev.replies["status"] = `{"notifications":1,"menu":0}`
	if code, body := post(`{"app":"Spam","app_id":"spam.app","title":"x","body":"y","seconds":5}`); code != 200 || body != "excluded" {
		t.Fatalf("excluded app: %d %q", code, body)
	}
	if code, _ := post(`{"app":"","title":"x"}`); code != 400 {
		t.Fatalf("empty app = %d", code)
	}
	if code, _ := post(`{"app":"Mail","title":"Hi","unknown":1}`); code != 400 {
		t.Fatalf("unknown field = %d", code)
	}
	e.dev.reset()
	if code, body := post(`{"app":"Mail","title":"Hi","body":"there","seconds":5}`); code != 200 || body != "OK" {
		t.Fatalf("deliver: %d %q", code, body)
	}
	cmds := strings.Join(e.dev.commands(), "|")
	for _, want := range []string{"notice -", "notice-text app", "notice-text title", "notice-text body", "notice-show 5"} {
		if !strings.Contains(cmds, want) {
			t.Errorf("device never received %q: %s", want, cmds)
		}
	}
	if code, _ := post(`{"app":"Mail","title":"Hi2","body":"b","seconds":5}`); code != 429 {
		t.Fatalf("burst must be rate limited, got %d", code)
	}
	time.Sleep(300 * time.Millisecond)
	e.dev.replies["status"] = `{"notifications":0}`
	if code, body := post(`{"app":"Mail","title":"Hi3","body":"b","seconds":5}`); code != 200 || body != "disabled on device" {
		t.Fatalf("device-side disable: %d %q", code, body)
	}
}

func TestHubAudioRouteGuards(t *testing.T) {
	e := newRouteEnv(t)
	if code, _ := hubDo(t, e, "POST", "/api/audio", hubToken, "application/json", ""); code != 415 {
		t.Fatalf("wrong content type = %d", code)
	}
	if code, _ := hubDo(t, e, "GET", "/api/audio", hubToken, "application/octet-stream", ""); code != 415 {
		t.Fatalf("GET = %d", code)
	}
	req, _ := http.NewRequest("POST", e.hub.URL+"/api/audio", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer "+hubToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Fracture-Audio-Source", "bogus")
	res, err := hubClient(e).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("bad source = %d", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", e.hub.URL+"/api/audio", strings.NewReader("x"))
	req.Header.Set("Authorization", "Bearer "+hubToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Fracture-Audio-Source", "windows")
	res, err = hubClient(e).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 409 {
		t.Fatalf("audio disabled must be 409, got %d", res.StatusCode)
	}
}

func TestHubForecastUploadsToDevice(t *testing.T) {
	e := newRouteEnv(t)
	now := time.Now().Unix()
	good := fmt.Sprintf(`{"Saved":%d,"Offset":7200,"Hours":[{"When":%d,"Temp":215,"Code":1,"Rain":10}],"Days":[{"When":%d,"Low":120,"High":250,"Code":2,"Rain":20}]}`, now, now, now)
	if code, body := hubDo(t, e, "POST", "/api/forecast", hubToken, "application/json", good); code != 200 {
		t.Fatalf("forecast: %d %s cmds=%v", code, body, e.dev.commands())
	}
	if len(e.dev.file("/mnt/sdcard/fracture-v3/weather.dat")) == 0 {
		t.Fatal("forecast data never reached the device")
	}
	bad := fmt.Sprintf(`{"Saved":%d,"Offset":7200,"Hours":[{"When":%d,"Temp":9999,"Code":1,"Rain":10}],"Days":[{"When":%d,"Low":120,"High":250,"Code":2,"Rain":20}]}`, now, now, now)
	if code, _ := hubDo(t, e, "POST", "/api/forecast", hubToken, "application/json", bad); code != 400 {
		t.Fatalf("out-of-range temperature = %d, want 400", code)
	}
	if code, _ := hubDo(t, e, "POST", "/api/forecast", hubToken, "application/json", `{"Hours":[],"Days":[]}`); code != 400 {
		t.Fatalf("empty forecast = %d, want 400", code)
	}
}

func TestHubAgendaPushesEvents(t *testing.T) {
	e := newRouteEnv(t)
	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	good := fmt.Sprintf(`{"events":[{"at":%q,"title":"Standup","location":"Room 1"}]}`, at)
	code, body := hubDo(t, e, "POST", "/api/agenda", hubToken, "application/json", good)
	if code != 200 || !e.dev.saw("agenda-clear") {
		t.Fatalf("agenda: %d %q cmds=%v", code, body, e.dev.commands())
	}
	for _, bad := range []string{`{"events":[{"at":"` + at + `","title":""}]}`, `{"events":[{"title":"x"}]}`, `{"events":[],"zzz":1}`} {
		if code, _ := hubDo(t, e, "POST", "/api/agenda", hubToken, "application/json", bad); code != 400 {
			t.Errorf("%.60s = %d, want 400", bad, code)
		}
	}
}

func TestHubMediaUpdateAndValidation(t *testing.T) {
	e := newRouteEnv(t)
	ok := `{"title":"Song","artist":"Band","playing":true,"position":10,"duration":100,"volume":5,"volume_ok":true,"play_ok":true}`
	code, body := hubDo(t, e, "POST", "/api/media", hubToken, "application/json", ok)
	if code != 200 || !e.dev.saw("media") {
		t.Fatalf("media: %d %q cmds=%v", code, body, e.dev.commands())
	}
	for _, bad := range []string{`{"position":200,"duration":100}`, `{"duration":999999}`, `{"volume":-5}`, `garbage`} {
		if code, _ := hubDo(t, e, "POST", "/api/media", hubToken, "application/json", bad); code != 400 {
			t.Errorf("%.40s = %d, want 400", bad, code)
		}
	}
}

func TestHubPreviewModel(t *testing.T) {
	e := newRouteEnv(t)
	e.dev.replies["home-state"] = "FPR1\nface=aqua\nwallpaper_hash=0\nEND"
	code, body := hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", `{"operation":"model"}`)
	if code != 200 || !strings.Contains(body, "FPR1") {
		t.Fatalf("model: %d %.100s", code, body)
	}
	if code, _ = hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", `{"operation":"asset","kind":"bogus","hash":1}`); code == 200 {
		t.Fatal("unknown asset kind must not succeed")
	}
	e.dev.failOn = "home-state"
	if code, body = hubDo(t, e, "POST", "/api/preview", hubToken, "application/json", `{"operation":"model"}`); code != 502 || !strings.Contains(body, "0.1.11") {
		t.Fatalf("older device must give an actionable error: %d %q", code, body)
	}
}

func TestHubLyricsRejectsBadInput(t *testing.T) {
	e := newRouteEnv(t)
	big := `{"title":"` + strings.Repeat("a", 600) + `"}`
	if code, _ := hubDo(t, e, "POST", "/api/lyrics", hubToken, "application/json", big); code != 400 {
		t.Fatalf("oversized title = %d, want 400", code)
	}
	if code, _ := hubDo(t, e, "POST", "/api/lyrics", hubToken, "application/json", `{"bogus":1}`); code != 400 {
		t.Fatalf("unknown field = %d, want 400", code)
	}
}
