package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeTomTom models the device-side wallpaper contract: upload + "wallpaper
// load" sets wallpaper_hash, "wallpaper clear" zeroes it. The hash function
// differs from the real device on purpose: the host must not depend on it.
type fakeTomTom struct {
	online     bool
	legacy     bool // firmware < 0.1.11: no home-state
	hash       uint32
	staged     []byte
	uploads    int
	clears     int
	batches    [][]string
	failLoad   bool
	noWallpape bool
}

func (f *fakeTomTom) wpConnected() bool { return true }
func (f *fakeTomTom) wpUpload(d []byte) error {
	if !f.online {
		return errors.New("offline")
	}
	f.uploads++
	f.staged = append([]byte(nil), d...)
	return nil
}
func (f *fakeTomTom) batch(c []string) error {
	if !f.online {
		return errors.New("offline")
	}
	f.batches = append(f.batches, c)
	return nil
}
func (f *fakeTomTom) call(cmd string) (string, error) {
	if !f.online {
		return "", errors.New("offline")
	}
	switch cmd {
	case "wallpaper load":
		if f.failLoad {
			return "", errors.New("ERR")
		}
		h := fnv.New32a()
		h.Write(f.staged)
		f.hash = h.Sum32() | 1
		return "OK", nil
	case "wallpaper clear":
		f.clears++
		f.hash = 0
		return "OK", nil
	case "home-state":
		if f.legacy {
			return "", errors.New("unknown command")
		}
		return fmt.Sprintf("FPR1\nface=aqua\nwallpaper_hash=%d\nEND\n", f.hash), nil
	case "status":
		return fmt.Sprintf(`{"wallpaper_active":%v}`, f.hash != 0), nil
	}
	return "", errors.New("unexpected command " + cmd)
}

// offlineTomTom is a Broker-like device that is not connected at all.
type offlineTomTom struct{ fakeTomTom }

func (o *offlineTomTom) wpConnected() bool { return false }

func wpEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)
	orig, _ := os.Getwd()
	if err := os.Chdir(".."); err != nil { // repo root so assets/ presets resolve
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	c, err := readConfig() // real defaults, as the app starts
	if err != nil {
		t.Fatal(err)
	}
	c.Wallpaper = WallpaperConfig{Mode: "color", Color: "000000"}
	if err := saveConfig(c); err != nil {
		t.Fatal(err)
	}
}

func presetSel(name string) WallpaperConfig { return WallpaperConfig{Mode: "preset", Preset: name} }

const presetA = "01-purple-blue-ribbon-flow.jpg"
const presetB = "02-midnight-sweep.jpg"

func mustApply(t *testing.T, d wallpaperDevice, w WallpaperConfig) wallpaperApplyResult {
	t.Helper()
	r, err := applyWallpaper(d, w, nil, wallpaperApplyOptions{})
	if err != nil {
		t.Fatalf("apply %+v: %v", w, err)
	}
	return r
}

func TestOfflineApplyThenPlugInSyncsOnceAndIsIdempotent(t *testing.T) {
	wpEnv(t)
	res := mustApply(t, &offlineTomTom{}, presetSel(presetA))
	if res.DeviceApplied || !strings.Contains(res.Warning, "when it connects") {
		t.Fatalf("offline apply should save with a warning, got %+v", res)
	}
	c, _ := readConfig()
	if c.Wallpaper.Preset != presetA || c.Wallpaper.Mode != "preset" {
		t.Fatalf("selection not persisted: %+v", c.Wallpaper)
	}
	dev := &fakeTomTom{online: true}
	if pushed, err := syncWallpaper(dev); err != nil || !pushed || dev.uploads != 1 || dev.hash == 0 {
		t.Fatalf("plug-in sync: pushed=%v err=%v uploads=%d hash=%d", pushed, err, dev.uploads, dev.hash)
	}
	if pushed, err := syncWallpaper(dev); err != nil || pushed || dev.uploads != 1 {
		t.Fatalf("second sync must be a no-op: pushed=%v err=%v uploads=%d", pushed, err, dev.uploads)
	}
}

func TestStaleDeviceWallpaperIsReplacedAfterOfflineChange(t *testing.T) {
	// Regression: the old supervisor only checked "wallpaper_active", so a device
	// still holding wallpaper A never received the newly chosen wallpaper B.
	wpEnv(t)
	dev := &fakeTomTom{online: true}
	mustApply(t, dev, presetSel(presetA))
	hashA := dev.hash
	mustApply(t, &offlineTomTom{}, presetSel(presetB))
	if pushed, err := syncWallpaper(dev); err != nil || !pushed {
		t.Fatalf("expected B to be pushed: pushed=%v err=%v", pushed, err)
	}
	if dev.hash == hashA || dev.uploads != 2 {
		t.Fatalf("device still has old wallpaper (hash %d, uploads %d)", dev.hash, dev.uploads)
	}
}

func TestDeviceReflashRestoresWallpaper(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true}
	mustApply(t, dev, presetSel(presetA))
	dev.hash = 0 // device lost its wallpaper (reflash / file wiped)
	if pushed, err := syncWallpaper(dev); err != nil || !pushed || dev.hash == 0 {
		t.Fatalf("wallpaper not restored: pushed=%v err=%v hash=%d", pushed, err, dev.hash)
	}
}

func TestSolidColourAppliesAndResyncs(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true}
	mustApply(t, dev, presetSel(presetA)) // device has a wallpaper
	mustApply(t, &offlineTomTom{}, WallpaperConfig{Mode: "color", Color: "112233"})
	if pushed, err := syncWallpaper(dev); err != nil || !pushed {
		t.Fatalf("colour resync: pushed=%v err=%v", pushed, err)
	}
	if dev.hash != 0 || dev.clears != 1 || len(dev.batches) != 1 ||
		dev.batches[0][0] != "face-set background 112233" || dev.batches[0][1] != "theme-set background 112233" {
		t.Fatalf("colour not applied: hash=%d clears=%d batches=%v", dev.hash, dev.clears, dev.batches)
	}
	if pushed, _ := syncWallpaper(dev); pushed {
		t.Fatal("colour sync must be idempotent")
	}
}

func TestLegacyFirmwareFallsBackToStatus(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true, legacy: true}
	mustApply(t, dev, presetSel(presetA))
	if pushed, _ := syncWallpaper(dev); pushed {
		t.Fatal("legacy device with wallpaper loaded must not be re-pushed")
	}
	dev.hash = 0
	if pushed, err := syncWallpaper(dev); err != nil || !pushed {
		t.Fatalf("legacy device missing wallpaper must be re-pushed: %v %v", pushed, err)
	}
}

func TestUnconfirmedLoadIsReportedAndRetried(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true, failLoad: true}
	res := mustApply(t, dev, presetSel(presetA))
	if res.DeviceApplied || res.Warning == "" {
		t.Fatalf("failed load must warn, got %+v", res)
	}
	dev.failLoad = false
	if pushed, err := syncWallpaper(dev); err != nil || !pushed {
		t.Fatalf("retry after failure: %v %v", pushed, err)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true}
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	img.SetRGBA(1, 1, color.RGBA{200, 10, 10, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	res, err := applyWallpaper(dev, WallpaperConfig{Mode: "custom"}, buf.Bytes(), wallpaperApplyOptions{DryRun: true, Name: "x"})
	if err != nil || !res.DryRun || len(res.PNG) == 0 {
		t.Fatalf("dry run: %v %+v", err, res)
	}
	c, _ := readConfig()
	if c.Wallpaper.Mode != "color" {
		t.Fatalf("dry run saved config: %+v", c.Wallpaper)
	}
	if len(readWallpaperHistory(false)) != 0 || dev.uploads != 0 || dev.clears != 0 || dev.hash != 0 {
		t.Fatal("dry run touched history or device")
	}
	if _, err := readPreviousWallpaper(); err == nil {
		t.Fatal("dry run recorded a restore point")
	}
}

func TestRestorePreviousSelection(t *testing.T) {
	wpEnv(t)
	dev := &fakeTomTom{online: true}
	mustApply(t, dev, presetSel(presetA))
	mustApply(t, dev, presetSel(presetB))
	prev, err := readPreviousWallpaper()
	if err != nil || prev.Preset != presetA {
		t.Fatalf("restore point wrong: %v %+v", err, prev)
	}
	mustApply(t, dev, prev)
	c, _ := readConfig()
	if c.Wallpaper.Preset != presetA || dev.uploads != 3 {
		t.Fatalf("restore failed: %+v uploads=%d", c.Wallpaper, dev.uploads)
	}
}

func TestInvalidSelectionLeavesConfigUntouched(t *testing.T) {
	wpEnv(t)
	for _, w := range []WallpaperConfig{
		{Mode: "preset", Preset: "../../etc/passwd"},
		{Mode: "preset", Preset: "nope.jpg"},
		{Mode: "custom", CustomID: "missing-id-1234"},
		{Mode: "color", Color: "zzzzzz"},
		{Mode: "bogus"},
	} {
		if _, err := applyWallpaper(&fakeTomTom{online: true}, w, nil, wallpaperApplyOptions{}); err == nil {
			t.Fatalf("expected error for %+v", w)
		}
	}
	c, _ := readConfig()
	if c.Wallpaper.Mode != "color" {
		t.Fatalf("config changed by invalid request: %+v", c.Wallpaper)
	}
}

func TestTruncateRunesKeepsUTF8Valid(t *testing.T) {
	got := truncateRunes(strings.Repeat("é", 60), 48)
	if len([]rune(got)) != 48 || !strings.HasPrefix(got, "éé") {
		t.Fatalf("bad truncation: %q", got)
	}
}

func TestHTTPDryRunRestoreAndDeleteGuard(t *testing.T) {
	wpEnv(t)
	mux := http.NewServeMux()
	b := &Broker{}
	registerWallpaper(mux, b, "tok")
	srv := httptest.NewServer(mux)
	defer srv.Close()
	do := func(method, body string) (int, map[string]any) {
		req, _ := http.NewRequest(method, srv.URL+"/api/wallpaper", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer tok")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	img.SetRGBA(3, 3, color.RGBA{9, 99, 199, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	// dry run: preview but no history, config unchanged
	code, out := do("POST", `{"mode":"custom","image_base64":"`+b64+`","dry_run":true}`)
	if code != 200 || out["dry_run"] != true || out["active_preview"] == "" {
		t.Fatalf("dry run response: %d %v", code, out)
	}
	if len(out["history"].([]any)) != 0 {
		t.Fatal("dry run stored history")
	}
	// real apply
	code, out = do("POST", `{"mode":"custom","name":"n","image_base64":"`+b64+`"}`)
	if code != 200 {
		t.Fatalf("apply: %d %v", code, out)
	}
	id := out["config"].(map[string]any)["custom_id"].(string)
	// deleting the applied wallpaper is refused
	if code, _ = do("POST", `{"operation":"delete","delete_id":"`+id+`"}`); code != 409 {
		t.Fatalf("delete of applied wallpaper = %d, want 409", code)
	}
	// switch away, then delete succeeds
	do("POST", `{"mode":"color","color":"000000"}`)
	if code, _ = do("POST", `{"operation":"delete","delete_id":"`+id+`"}`); code != 200 {
		t.Fatalf("delete after switching = %d", code)
	}
	// unauthorised
	req, _ := http.NewRequest("GET", srv.URL+"/api/wallpaper", nil)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 401 {
		t.Fatalf("unauthenticated GET = %d", res.StatusCode)
	}
}
