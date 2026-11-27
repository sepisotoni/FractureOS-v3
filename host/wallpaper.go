package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	wallpaperWidth      = 320
	wallpaperHeight     = 240
	wallpaperRGB565Size = wallpaperWidth * wallpaperHeight * 2
	maxCustomWallpapers = 5
)

type WallpaperConfig struct {
	Mode       string `json:"mode"` // "pc", "color", "preset", "custom"
	Color      string `json:"color"`
	Preset     string `json:"preset"`
	CustomID   string `json:"custom_id"`
	LastPCHash string `json:"last_pc_hash,omitempty"` // legacy; superseded by Applied

	// Applied/AppliedHash record what the host last confirmed on the TomTom:
	// a content key of the wallpaper and the wallpaper_hash the device reported
	// afterwards. The supervisor re-pushes whenever either side drifts (new
	// selection while unplugged, PC wallpaper changed, device re-flashed).
	Applied     string `json:"applied,omitempty"`
	AppliedHash uint32 `json:"applied_hash,omitempty"`

	// Overrides records the device values this feature replaced, as
	// {"theme.background": "<original>", "face.background": "<original>"}.
	// Only a non-black solid colour writes them. When the selection moves to
	// anything else, exactly these keys are set back to their recorded values
	// and the record is cleared. The wallpaper path never calls theme-reset or
	// face-reset, so unrelated theme/face customisation is never touched.
	Overrides map[string]string `json:"wallpaper_overrides,omitempty"`
}

type wallpaperHistoryEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"created_at"`
	Preview   string `json:"preview,omitempty"`
}

type wallpaperPresetInfo struct {
	Name    string `json:"name"`
	Preview string `json:"preview,omitempty"`
}

var (
	wallpaperHexColor = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)
	wallpaperIDRegex  = regexp.MustCompile(`^[a-zA-Z0-9_-]{4,48}$`)
	wallpaperMu       sync.Mutex
)

func defaultWallpaper() WallpaperConfig {
	mode := "color"
	if runtime.GOOS == "windows" {
		mode = "pc"
	}
	return WallpaperConfig{
		Mode:  mode,
		Color: "000000",
	}
}

func wallpaperDir() string {
	return filepath.Join(configDir(), "wallpapers")
}

func wallpaperPresetsDirs() []string {
	rels := []string{
		filepath.Join("assets", "wallpaper-packs", "tomtom-one-v6", "wallpapers", "ai-generated"),
		filepath.Join("assets", "wallpapers"),
	}
	var dirs []string
	seen := map[string]bool{}
	add := func(p string) {
		if !seen[p] {
			if info, err := os.Stat(p); err == nil && info.IsDir() {
				seen[p] = true
				dirs = append(dirs, p)
			}
		}
	}
	for _, rel := range rels {
		add(rel)
		if exe, err := os.Executable(); err == nil {
			dir := filepath.Dir(exe)
			add(filepath.Join(dir, rel))
			add(filepath.Join(dir, "..", rel))
		}
	}
	return dirs
}

func findWallpaperPresetPath(name string) (string, error) {
	base := filepath.Base(name)
	if base == "" || base == "." || base == ".." {
		return "", errors.New("invalid preset name")
	}
	for _, dir := range wallpaperPresetsDirs() {
		p := filepath.Join(dir, base)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}

func decodeWallpaperImage(data []byte) (image.Image, error) {
	if len(data) == 0 || len(data) > 20*1024*1024 {
		return nil, errors.New("invalid image size")
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	if err == nil {
		return im, nil
	}
	// Fallback for uncompressed 24/32-bit Windows BMP wallpapers.
	if len(data) >= 54 && data[0] == 'B' && data[1] == 'M' {
		if bmpImg, bmpErr := decodeBMP(data); bmpErr == nil {
			return bmpImg, nil
		}
	}
	return nil, err
}

func decodeBMP(data []byte) (image.Image, error) {
	if len(data) < 54 || data[0] != 'B' || data[1] != 'M' {
		return nil, errors.New("not a bmp")
	}
	pixelOffset := int(uint32(data[10]) | uint32(data[11])<<8 | uint32(data[12])<<16 | uint32(data[13])<<24)
	w := int(int32(uint32(data[18]) | uint32(data[19])<<8 | uint32(data[20])<<16 | uint32(data[21])<<24))
	h := int(int32(uint32(data[22]) | uint32(data[23])<<8 | uint32(data[24])<<16 | uint32(data[25])<<24))
	bpp := int(uint16(data[28]) | uint16(data[29])<<8)
	comp := int(uint32(data[30]) | uint32(data[31])<<8 | uint32(data[32])<<16 | uint32(data[33])<<24)
	topDown := false
	if h < 0 {
		h = -h
		topDown = true
	}
	if w < 1 || h < 1 || w > 8192 || h > 8192 || (bpp != 24 && bpp != 32) || comp != 0 {
		return nil, errors.New("unsupported bmp format")
	}
	rowBytes := ((w*bpp + 31) / 32) * 4
	if pixelOffset+rowBytes*h > len(data) {
		return nil, errors.New("truncated bmp")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	stride := bpp / 8
	for y := 0; y < h; y++ {
		srcY := h - 1 - y
		if topDown {
			srcY = y
		}
		rowStart := pixelOffset + srcY*rowBytes
		for x := 0; x < w; x++ {
			off := rowStart + x*stride
			b, g, r := data[off], data[off+1], data[off+2]
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img, nil
}

// imageToWallpaper converts any supported image into a 320x240 RGB565 payload
// (cover-cropped and bilinearly sampled) plus a 320x240 PNG for local history.
func imageToWallpaper(data []byte) ([]byte, []byte, error) {
	im, err := decodeWallpaperImage(data)
	if err != nil {
		return nil, nil, err
	}
	bounds := im.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW < 1 || srcH < 1 {
		return nil, nil, errors.New("empty image")
	}

	// Cover-fit crop rectangle in source image coordinates.
	targetAspect := float64(wallpaperWidth) / float64(wallpaperHeight)
	srcAspect := float64(srcW) / float64(srcH)
	cropW, cropH := float64(srcW), float64(srcH)
	if srcAspect > targetAspect {
		cropW = float64(srcH) * targetAspect
	} else if srcAspect < targetAspect {
		cropH = float64(srcW) / targetAspect
	}
	cropX := float64(bounds.Min.X) + (float64(srcW)-cropW)/2.0
	cropY := float64(bounds.Min.Y) + (float64(srcH)-cropH)/2.0

	rgb565 := make([]byte, wallpaperRGB565Size)
	pngImg := image.NewRGBA(image.Rect(0, 0, wallpaperWidth, wallpaperHeight))

	for y := 0; y < wallpaperHeight; y++ {
		sy := cropY + (float64(y)+0.5)*cropH/float64(wallpaperHeight) - 0.5
		y0 := int(sy)
		if y0 < bounds.Min.Y {
			y0 = bounds.Min.Y
		}
		y1 := y0 + 1
		if y1 >= bounds.Max.Y {
			y1 = bounds.Max.Y - 1
		}
		fy := sy - float64(y0)
		if fy < 0 {
			fy = 0
		} else if fy > 1 {
			fy = 1
		}

		for x := 0; x < wallpaperWidth; x++ {
			sx := cropX + (float64(x)+0.5)*cropW/float64(wallpaperWidth) - 0.5
			x0 := int(sx)
			if x0 < bounds.Min.X {
				x0 = bounds.Min.X
			}
			x1 := x0 + 1
			if x1 >= bounds.Max.X {
				x1 = bounds.Max.X - 1
			}
			fx := sx - float64(x0)
			if fx < 0 {
				fx = 0
			} else if fx > 1 {
				fx = 1
			}

			r00, g00, b00, _ := im.At(x0, y0).RGBA()
			r10, g10, b10, _ := im.At(x1, y0).RGBA()
			r01, g01, b01, _ := im.At(x0, y1).RGBA()
			r11, g11, b11, _ := im.At(x1, y1).RGBA()

			w00 := (1 - fx) * (1 - fy)
			w10 := fx * (1 - fy)
			w01 := (1 - fx) * fy
			w11 := fx * fy

			r := uint8((float64(r00>>8)*w00 + float64(r10>>8)*w10 + float64(r01>>8)*w01 + float64(r11>>8)*w11))
			g := uint8((float64(g00>>8)*w00 + float64(g10>>8)*w10 + float64(g01>>8)*w01 + float64(g11>>8)*w11))
			b := uint8((float64(b00>>8)*w00 + float64(b10>>8)*w10 + float64(b01>>8)*w01 + float64(b11>>8)*w11))

			pngImg.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			v := uint16((uint16(r)>>3)<<11 | (uint16(g)>>2)<<5 | (uint16(b) >> 3))
			at := (y*wallpaperWidth + x) * 2
			rgb565[at] = byte(v)
			rgb565[at+1] = byte(v >> 8)
		}
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, pngImg); err != nil {
		return nil, nil, err
	}
	return rgb565, pngBuf.Bytes(), nil
}

func readWallpaperHistory(withPreviews bool) []wallpaperHistoryEntry {
	path := filepath.Join(wallpaperDir(), "history.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return []wallpaperHistoryEntry{}
	}
	var entries []wallpaperHistoryEntry
	if json.Unmarshal(data, &entries) != nil {
		return []wallpaperHistoryEntry{}
	}
	kept := []wallpaperHistoryEntry{} // never nil: JSON must be [] not null
	for _, entry := range entries {
		if !wallpaperIDRegex.MatchString(entry.ID) {
			continue
		}
		imgPath := filepath.Join(wallpaperDir(), entry.ID+".png")
		raw, err := os.ReadFile(imgPath)
		if err != nil {
			continue
		}
		if withPreviews {
			entry.Preview = "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
		}
		kept = append(kept, entry)
		if len(kept) >= maxCustomWallpapers {
			break
		}
	}
	return kept
}

func writeWallpaperFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".wp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	_ = tmp.Chmod(mode)
	if err := tmp.Close(); err != nil {
		return err
	}
	return replaceFile(tmp.Name(), path)
}

func saveWallpaperHistory(entries []wallpaperHistoryEntry) error {
	if err := os.MkdirAll(wallpaperDir(), 0700); err != nil {
		return err
	}
	var clean []wallpaperHistoryEntry
	for _, e := range entries {
		if len(clean) >= maxCustomWallpapers {
			_ = os.Remove(filepath.Join(wallpaperDir(), e.ID+".png"))
			continue
		}
		e.Preview = ""
		clean = append(clean, e)
	}
	data, err := json.MarshalIndent(clean, "", "  ")
	if err != nil {
		return err
	}
	return writeWallpaperFile(filepath.Join(wallpaperDir(), "history.json"), data, 0600)
}

func addCustomWallpaperToHistory(name string, pngBytes []byte) (string, error) {
	if err := os.MkdirAll(wallpaperDir(), 0700); err != nil {
		return "", err
	}
	id := customWallpaperID(pngBytes)
	imgPath := filepath.Join(wallpaperDir(), id+".png")
	if err := writeWallpaperFile(imgPath, pngBytes, 0600); err != nil {
		return "", err
	}
	existing := readWallpaperHistory(false)
	if strings.TrimSpace(name) == "" {
		name = "Custom wallpaper"
	}
	name = truncateRunes(name, 48)
	updated := []wallpaperHistoryEntry{{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now().Unix(),
	}}
	for _, e := range existing {
		if e.ID != id {
			updated = append(updated, e)
		}
	}
	if err := saveWallpaperHistory(updated); err != nil {
		return "", err
	}
	return id, nil
}

var presetPreviewCache sync.Map // "path|size|mtime" -> data URL

func cachedPresetPreview(path string, raw []byte) (string, error) {
	key := path
	if info, err := os.Stat(path); err == nil {
		key = fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	}
	if v, ok := presetPreviewCache.Load(key); ok {
		return v.(string), nil
	}
	_, pngThumb, err := imageToWallpaper(raw)
	if err != nil {
		return "", err
	}
	url := pngDataURL(pngThumb)
	presetPreviewCache.Store(key, url)
	return url, nil
}

func listWallpaperPresets() []wallpaperPresetInfo {
	var out []wallpaperPresetInfo
	seen := map[string]bool{}
	for _, dir := range wallpaperPresetsDirs() {
		items, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.IsDir() || seen[item.Name()] {
				continue
			}
			ext := strings.ToLower(filepath.Ext(item.Name()))
			if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".bmp" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, item.Name()))
			if err != nil {
				continue
			}
			preview, err := cachedPresetPreview(filepath.Join(dir, item.Name()), raw)
			if err != nil {
				continue
			}
			seen[item.Name()] = true
			out = append(out, wallpaperPresetInfo{Name: item.Name(), Preview: preview})
		}
	}
	return out
}

// ---- device seam -----------------------------------------------------------

// wallpaperDevice is the slice of Broker that wallpaper code needs. It exists
// so the whole save/apply/resync flow can be tested against a fake TomTom.
type wallpaperDevice interface {
	call(cmd string) (string, error)
	batch(cmds []string) error
	wpUpload(data []byte) error
	wpConnected() bool
}

func (b *Broker) wpUpload(data []byte) error { return uploadWallpaper(b.device, data) }
func (b *Broker) wpConnected() bool          { return b != nil && b.device != "" }

var errWallpaperNoDevice = errors.New("no TomTom connected")

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func customWallpaperID(pngBytes []byte) string {
	sum := sha256.Sum256(pngBytes)
	return hex.EncodeToString(sum[:8])
}

// ---- resolving a selection to pixels ---------------------------------------

type resolvedWallpaper struct {
	RGB    []byte // 320x240 RGB565 little-endian; nil for a solid colour
	PNG    []byte // 320x240 PNG preview; nil for a solid colour
	Key    string // content key; identical selections give identical keys
	PCHash string
}

// resolveWallpaper turns a selection into device pixels. uploaded, when set,
// is the raw image of a not-yet-stored custom wallpaper.
func resolveWallpaper(w WallpaperConfig, uploaded []byte) (resolvedWallpaper, error) {
	var r resolvedWallpaper
	var raw []byte
	var err error
	switch w.Mode {
	case "color":
		r.Key = "color:" + strings.ToLower(w.Color)
		return r, nil
	case "pc":
		raw, r.PCHash, err = readPCWallpaper()
	case "preset":
		var path string
		if path, err = findWallpaperPresetPath(w.Preset); err == nil {
			raw, err = os.ReadFile(path)
		}
	case "custom":
		if len(uploaded) > 0 {
			raw = uploaded
		} else if wallpaperIDRegex.MatchString(w.CustomID) {
			raw, err = os.ReadFile(filepath.Join(wallpaperDir(), w.CustomID+".png"))
		} else {
			err = errors.New("no custom wallpaper selected")
		}
	default:
		err = errors.New("unsupported wallpaper mode")
	}
	if err != nil {
		return r, err
	}
	if r.RGB, r.PNG, err = imageToWallpaper(raw); err != nil {
		return r, err
	}
	sum := sha256.Sum256(r.RGB)
	r.Key = "img:" + hex.EncodeToString(sum[:8])
	return r, nil
}

func localWallpaperPayload(c Config, payload map[string]any) error {
	if runtime.GOOS != "windows" || effectiveConnectionMode(c) == "direct" || payload["mode"] != "pc" {
		return nil
	}
	raw, _, err := readPCWallpaper()
	if err != nil {
		return err
	}
	_, preview, err := imageToWallpaper(raw)
	if err != nil {
		return err
	}
	payload["mode"] = "custom"
	payload["name"] = "Windows desktop wallpaper"
	payload["image_base64"] = base64.StdEncoding.EncodeToString(preview)
	return nil
}

// ---- device state ----------------------------------------------------------

func parseModelUint(model, key string) (uint32, bool) {
	for _, line := range strings.Split(model, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && k == key {
			var n uint64
			if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n <= 0xffffffff {
				return uint32(n), true
			}
		}
	}
	return 0, false
}

// wallpaperDeviceHash returns the device's wallpaper_hash (0 = none loaded).
// Devices older than 0.1.11 have no home-state; they fall back to the boolean
// wallpaper_active from status (reported as 1 or 0).
func wallpaperDeviceHash(t wallpaperDevice) (uint32, error) {
	if out, err := t.call("home-state"); err == nil && strings.HasPrefix(strings.TrimSpace(out), "FPR1") {
		if h, ok := parseModelUint(out, "wallpaper_hash"); ok {
			return h, nil
		}
	}
	raw, err := t.call("status")
	if err != nil {
		return 0, err
	}
	var st struct {
		WallpaperActive bool `json:"wallpaper_active"`
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return 0, err
	}
	if st.WallpaperActive {
		return 1, nil
	}
	return 0, nil
}

// wallpaperOverrideKeys is the fixed order in which overrides are written.
var wallpaperOverrideKeys = []struct{ Key, Get, Set string }{
	{"face.background", "face-get", "face-set background "},
	{"theme.background", "theme-get", "theme-set background "},
}

func readDeviceBackgrounds(t wallpaperDevice) map[string]string {
	out := map[string]string{}
	for _, k := range wallpaperOverrideKeys {
		raw, err := t.call(k.Get)
		if err != nil {
			continue
		}
		var j struct {
			Background string `json:"background"`
		}
		if json.Unmarshal([]byte(raw), &j) == nil && wallpaperHexColor.MatchString(j.Background) {
			out[k.Key] = strings.ToLower(j.Background)
		}
	}
	return out
}

func setBackgrounds(t wallpaperDevice, values map[string]string) error {
	var cmds []string
	for _, k := range wallpaperOverrideKeys {
		if v, ok := values[k.Key]; ok {
			cmds = append(cmds, k.Set+v)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return t.batch(cmds)
}

// wantsOverride reports whether the selection needs theme/face overrides.
// Solid black ("legacy") is just a cleared wallpaper and overrides nothing.
func wantsOverride(w WallpaperConfig) bool {
	return w.Mode == "color" && strings.ToLower(w.Color) != "000000"
}

// reconcileOverrides brings the device's theme/face backgrounds in line with
// the selection, recording originals before the first override and restoring
// exactly those originals afterwards. It returns the updated record.
func reconcileOverrides(t wallpaperDevice, w WallpaperConfig) (map[string]string, error) {
	rec := w.Overrides
	if wantsOverride(w) {
		if len(rec) == 0 {
			rec = readDeviceBackgrounds(t)
			if len(rec) == 0 {
				return nil, errors.New("cannot read the current theme/face background, so it was not overridden")
			}
		}
		target := map[string]string{}
		for k := range rec {
			target[k] = strings.ToLower(w.Color)
		}
		if err := setBackgrounds(t, target); err != nil {
			return w.Overrides, err
		}
		return rec, nil
	}
	if len(rec) > 0 {
		if err := setBackgrounds(t, rec); err != nil {
			return rec, err
		}
	}
	return nil, nil
}

func applyColorToDevice(t wallpaperDevice, w WallpaperConfig) (map[string]string, error) {
	if _, err := t.call("wallpaper clear"); err != nil {
		return w.Overrides, err
	}
	return reconcileOverrides(t, w)
}

// pushResolved puts a resolved selection on the device and verifies the device
// reports the expected state afterwards. It returns the config with
// Applied/AppliedHash filled in.
func pushResolved(t wallpaperDevice, w WallpaperConfig, res resolvedWallpaper) (WallpaperConfig, error) {
	if w.Mode == "color" {
		ov, err := applyColorToDevice(t, w)
		w.Overrides = ov
		if err != nil {
			return w, err
		}
	} else {
		if len(res.RGB) != wallpaperRGB565Size {
			return w, errors.New("wallpaper has no pixel data")
		}
		if err := t.wpUpload(res.RGB); err != nil {
			return w, err
		}
		if _, err := t.call("wallpaper load"); err != nil {
			return w, err
		}
		ov, err := reconcileOverrides(t, w) // give back any earlier colour override
		w.Overrides = ov
		if err != nil {
			return w, err
		}
	}
	hash, err := wallpaperDeviceHash(t)
	if err != nil {
		// Pushed but unverified: leave Applied empty so the next sync verifies.
		w.Applied, w.AppliedHash = "", 0
		return w, nil
	}
	if (w.Mode == "color") != (hash == 0) {
		w.Applied, w.AppliedHash = "", 0
		return w, errors.New("TomTom did not confirm the wallpaper change")
	}
	w.Applied, w.AppliedHash = res.Key, hash
	return w, nil
}

// syncWallpaper makes the TomTom match the saved selection. It is idempotent:
// it only pushes when the saved selection, the PC wallpaper, or the device
// itself has changed since the last confirmed push.
func syncWallpaper(t wallpaperDevice) (bool, error) {
	if !t.wpConnected() {
		return false, errWallpaperNoDevice
	}
	wallpaperMu.Lock()
	defer wallpaperMu.Unlock()
	c, err := readConfig()
	if err != nil {
		return false, err
	}
	w := c.Wallpaper
	if w.Mode == "" {
		w = defaultWallpaper()
	}
	res, err := resolveWallpaper(w, nil)
	if err != nil {
		return false, err
	}
	dev, err := wallpaperDeviceHash(t)
	if err != nil {
		return false, err
	}
	pendingRestore := !wantsOverride(w) && len(w.Overrides) > 0
	if w.Applied == res.Key && w.AppliedHash == dev && !pendingRestore {
		return false, nil
	}
	applied, perr := pushResolved(t, w, res)
	if c2, e2 := readConfig(); e2 == nil {
		// Only record the result if the selection was not changed meanwhile.
		same := c2.Wallpaper.Mode == w.Mode && c2.Wallpaper.Color == w.Color &&
			c2.Wallpaper.Preset == w.Preset && c2.Wallpaper.CustomID == w.CustomID
		if same {
			c2.Wallpaper.Applied, c2.Wallpaper.AppliedHash = applied.Applied, applied.AppliedHash
			c2.Wallpaper.Overrides = applied.Overrides
			if res.PCHash != "" {
				c2.Wallpaper.LastPCHash = res.PCHash
			}
			if e := saveConfig(c2); e != nil && perr == nil {
				perr = e
			}
		}
	}
	return perr == nil, perr
}

// ---- apply / dry-run / restore --------------------------------------------

type wallpaperApplyOptions struct {
	DryRun bool
	Name   string
}

type wallpaperApplyResult struct {
	Config        WallpaperConfig
	Warning       string
	PNG           []byte
	DryRun        bool
	DeviceApplied bool
}

func previousWallpaperPath() string { return filepath.Join(wallpaperDir(), "previous.json") }

func readPreviousWallpaper() (WallpaperConfig, error) {
	var w WallpaperConfig
	data, err := os.ReadFile(previousWallpaperPath())
	if err != nil {
		return w, errors.New("no previous wallpaper to restore")
	}
	if err := json.Unmarshal(data, &w); err != nil {
		return w, errors.New("previous wallpaper record is damaged")
	}
	return w, nil
}

func savePreviousWallpaper(w WallpaperConfig) error {
	if err := os.MkdirAll(wallpaperDir(), 0700); err != nil {
		return err
	}
	w.Applied, w.AppliedHash, w.LastPCHash, w.Overrides = "", 0, "", nil
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	return writeWallpaperFile(previousWallpaperPath(), data, 0600)
}

func sameWallpaperSelection(a, b WallpaperConfig) bool {
	return a.Mode == b.Mode && strings.EqualFold(a.Color, b.Color) && a.Preset == b.Preset && a.CustomID == b.CustomID
}

// applyWallpaper validates and renders the selection. With DryRun it stops
// there: nothing is saved, stored or sent to the TomTom. Otherwise it saves the
// selection (keeping the previous one for restore) and, when a TomTom is
// reachable, pushes and verifies it. An unreachable TomTom is not an error: the
// supervisor resyncs when it appears.
func applyWallpaper(t wallpaperDevice, next WallpaperConfig, uploaded []byte, opts wallpaperApplyOptions) (wallpaperApplyResult, error) {
	wallpaperMu.Lock()
	defer wallpaperMu.Unlock()

	out := wallpaperApplyResult{DryRun: opts.DryRun}
	if next.Color == "" {
		next.Color = "000000"
	}
	if !wallpaperHexColor.MatchString(next.Color) {
		return out, errors.New("invalid hex color")
	}
	next.Color = strings.ToLower(next.Color)
	next.Applied, next.AppliedHash = "", 0

	res, err := resolveWallpaper(next, uploaded)
	if err != nil {
		return out, err
	}
	out.PNG = res.PNG
	if len(uploaded) > 0 && next.Mode == "custom" {
		next.CustomID = customWallpaperID(res.PNG)
	}
	next.Preset = filepath.Base(strings.TrimSpace(next.Preset))
	if next.Mode != "preset" {
		next.Preset = ""
	}
	out.Config = next
	if opts.DryRun {
		return out, nil
	}

	if len(uploaded) > 0 && next.Mode == "custom" {
		if next.CustomID, err = addCustomWallpaperToHistory(opts.Name, res.PNG); err != nil {
			return out, err
		}
	}
	c, err := readConfig()
	if err != nil {
		return out, fmt.Errorf("cannot read settings: %w", err)
	}
	if c.Wallpaper.Mode != "" && !sameWallpaperSelection(c.Wallpaper, next) {
		if err := savePreviousWallpaper(c.Wallpaper); err != nil {
			log.Printf("wallpaper: cannot keep previous selection: %v", err)
		}
	}
	if res.PCHash != "" {
		next.LastPCHash = res.PCHash
	}
	next.Overrides = c.Wallpaper.Overrides // pending record survives until restored
	c.Wallpaper = next
	if err := saveConfig(c); err != nil {
		return out, fmt.Errorf("cannot save settings: %w", err)
	}
	out.Config = next
	if !t.wpConnected() {
		out.Warning = "Saved. It will be applied to the TomTom when it connects."
		return out, nil
	}
	applied, perr := pushResolved(t, next, res)
	if perr != nil {
		out.Warning = "Saved, but the TomTom did not accept it yet (" + perr.Error() + "). It will be retried automatically."
		return out, nil
	}
	out.Config, out.DeviceApplied = applied, true
	if c2, e2 := readConfig(); e2 == nil && sameWallpaperSelection(c2.Wallpaper, next) {
		c2.Wallpaper = applied
		if e := saveConfig(c2); e != nil {
			out.Warning = "Applied, but the confirmation could not be saved: " + e.Error()
		}
	}
	return out, nil
}

// ---- previews --------------------------------------------------------------

func pngDataURL(png []byte) string {
	if len(png) == 0 {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}

func activeWallpaperPreview(w WallpaperConfig) string {
	res, err := resolveWallpaper(w, nil)
	if err != nil {
		return ""
	}
	return pngDataURL(res.PNG)
}

// ---- supervisor ------------------------------------------------------------

func wallpaperSupervisor(ctx context.Context, b *Broker) {
	lastErr := ""
	syncOnce := func() {
		pushed, err := syncWallpaper(b)
		switch {
		case err != nil && err.Error() != lastErr:
			lastErr = err.Error()
			if !errors.Is(err, errWallpaperNoDevice) {
				log.Printf("wallpaper sync: %v", err)
			}
		case err == nil:
			lastErr = ""
			if pushed {
				log.Printf("wallpaper sync: TomTom updated")
			}
		}
	}
	// Initial sync shortly after hub startup so a TomTom plugged in later, or an
	// updated one, picks up the saved wallpaper without user action.
	select {
	case <-ctx.Done():
		return
	case <-time.After(2 * time.Second):
	}
	syncOnce()
	ticker := time.NewTicker(12 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncOnce()
		}
	}
}

// ---- HTTP ------------------------------------------------------------------

func wallpaperStateJSON(b *Broker, c Config) map[string]any {
	_, _, pcErr := readPCWallpaper()
	_, prevErr := readPreviousWallpaper()
	return map[string]any{
		"config":           c.Wallpaper,
		"history":          readWallpaperHistory(true),
		"presets":          listWallpaperPresets(),
		"pc_available":     pcErr == nil,
		"active_preview":   activeWallpaperPreview(c.Wallpaper),
		"can_restore":      prevErr == nil,
		"device_connected": b.wpConnected(),
	}
}

func removeCustomWallpaper(id string) (int, string) {
	wallpaperMu.Lock()
	defer wallpaperMu.Unlock()
	if c, err := readConfig(); err == nil && c.Wallpaper.Mode == "custom" && c.Wallpaper.CustomID == id {
		return 409, "that wallpaper is currently applied; apply a different one first"
	}
	if prev, err := readPreviousWallpaper(); err == nil && prev.Mode == "custom" && prev.CustomID == id {
		_ = os.Remove(previousWallpaperPath())
	}
	_ = os.Remove(filepath.Join(wallpaperDir(), id+".png"))
	var kept []wallpaperHistoryEntry
	for _, e := range readWallpaperHistory(false) {
		if e.ID != id {
			kept = append(kept, e)
		}
	}
	if err := saveWallpaperHistory(kept); err != nil {
		return 500, "cannot update wallpaper history"
	}
	return 0, ""
}

func registerWallpaper(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/wallpaper", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		writeJSON := func(v any) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(v)
		}
		sendState := func() {
			c, err := readConfig()
			if err != nil {
				http.Error(w, "config error", 500)
				return
			}
			writeJSON(wallpaperStateJSON(b, c))
		}
		deleteID := func(id string) {
			if !wallpaperIDRegex.MatchString(id) {
				http.Error(w, "invalid wallpaper id", 400)
				return
			}
			if code, msg := removeCustomWallpaper(id); code != 0 {
				http.Error(w, msg, code)
				return
			}
			writeJSON(map[string]any{"history": readWallpaperHistory(true)})
		}
		switch r.Method {
		case "GET":
			sendState()
		case "DELETE":
			deleteID(r.URL.Query().Get("id"))
		case "POST":
			r.Body = http.MaxBytesReader(w, r.Body, 8*1024*1024)
			var req struct {
				Operation   string `json:"operation"`
				DeleteID    string `json:"delete_id"`
				Mode        string `json:"mode"`
				Color       string `json:"color"`
				Preset      string `json:"preset"`
				CustomID    string `json:"custom_id"`
				Name        string `json:"name"`
				ImageBase64 string `json:"image_base64"`
				DryRun      bool   `json:"dry_run"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid wallpaper request", 400)
				return
			}
			switch {
			case req.Operation == "get":
				sendState()
				return
			case req.Operation == "delete" || req.DeleteID != "":
				deleteID(req.DeleteID)
				return
			case req.Operation == "sync":
				pushed, err := syncWallpaper(b)
				if err != nil && !errors.Is(err, errWallpaperNoDevice) {
					http.Error(w, err.Error(), 502)
					return
				}
				c, _ := readConfig()
				state := wallpaperStateJSON(b, c)
				state["pushed"] = pushed
				writeJSON(state)
				return
			}
			next := WallpaperConfig{Mode: req.Mode, Color: req.Color, Preset: req.Preset, CustomID: req.CustomID}
			if req.Operation == "restore" {
				prev, err := readPreviousWallpaper()
				if err != nil {
					http.Error(w, err.Error(), 404)
					return
				}
				next = prev
			}
			var imgBytes []byte
			if req.ImageBase64 != "" {
				b64 := req.ImageBase64
				if idx := strings.Index(b64, ","); idx >= 0 {
					b64 = b64[idx+1:]
				}
				var err error
				if imgBytes, err = base64.StdEncoding.DecodeString(b64); err != nil {
					http.Error(w, "invalid image data", 400)
					return
				}
			}
			res, err := applyWallpaper(b, next, imgBytes, wallpaperApplyOptions{DryRun: req.DryRun, Name: req.Name})
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			writeJSON(map[string]any{
				"config":           res.Config,
				"history":          readWallpaperHistory(true),
				"active_preview":   pngDataURL(res.PNG),
				"warning":          res.Warning,
				"dry_run":          res.DryRun,
				"device_applied":   res.DeviceApplied,
				"device_connected": b.wpConnected(),
				"can_restore":      func() bool { _, e := readPreviousWallpaper(); return e == nil }(),
			})
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
}
