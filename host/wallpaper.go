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
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
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
	LastPCHash string `json:"last_pc_hash,omitempty"`
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
		return nil
	}
	var entries []wallpaperHistoryEntry
	if json.Unmarshal(data, &entries) != nil {
		return nil
	}
	var kept []wallpaperHistoryEntry
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
	sum := sha256.Sum256(pngBytes)
	id := hex.EncodeToString(sum[:8])
	imgPath := filepath.Join(wallpaperDir(), id+".png")
	if err := writeWallpaperFile(imgPath, pngBytes, 0600); err != nil {
		return "", err
	}
	existing := readWallpaperHistory(false)
	if strings.TrimSpace(name) == "" {
		name = "Custom wallpaper"
	}
	if len(name) > 48 {
		name = name[:48]
	}
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
			_, pngThumb, err := imageToWallpaper(raw)
			if err != nil {
				continue
			}
			seen[item.Name()] = true
			out = append(out, wallpaperPresetInfo{
				Name:    item.Name(),
				Preview: "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngThumb),
			})
		}
	}
	return out
}

func pushWallpaperToDevice(b *Broker, rawImage []byte) error {
	rgb565, _, err := imageToWallpaper(rawImage)
	if err != nil {
		return err
	}
	if err := uploadWallpaper(b.device, rgb565); err != nil {
		return err
	}
	_, err = b.call("wallpaper load")
	return err
}

func applyWallpaper(b *Broker, next WallpaperConfig, uploadedImage []byte, customName string) (WallpaperConfig, string, error) {
	wallpaperMu.Lock()
	defer wallpaperMu.Unlock()

	if next.Color == "" {
		next.Color = "000000"
	}
	if !wallpaperHexColor.MatchString(next.Color) {
		return next, "", errors.New("invalid hex color")
	}
	next.Color = strings.ToLower(next.Color)

	var rgb565 []byte
	var warning string
	switch next.Mode {
	case "color":
		// Persist first, then clear on device if reachable.
	case "pc":
		raw, hash, err := readPCWallpaper()
		if err != nil {
			return next, "", err
		}
		rgb565, _, err = imageToWallpaper(raw)
		if err != nil {
			return next, "", err
		}
		next.LastPCHash = hash
	case "preset":
		presetPath, err := findWallpaperPresetPath(next.Preset)
		if err != nil {
			return next, "", err
		}
		raw, err := os.ReadFile(presetPath)
		if err != nil {
			return next, "", err
		}
		rgb565, _, err = imageToWallpaper(raw)
		if err != nil {
			return next, "", err
		}
		next.Preset = filepath.Base(presetPath)
	case "custom":
		if len(uploadedImage) > 0 {
			var pngBytes []byte
			var err error
			rgb565, pngBytes, err = imageToWallpaper(uploadedImage)
			if err != nil {
				return next, "", err
			}
			id, err := addCustomWallpaperToHistory(customName, pngBytes)
			if err != nil {
				return next, "", err
			}
			next.CustomID = id
		} else if wallpaperIDRegex.MatchString(next.CustomID) {
			raw, err := os.ReadFile(filepath.Join(wallpaperDir(), next.CustomID+".png"))
			if err != nil {
				return next, "", err
			}
			rgb565, _, err = imageToWallpaper(raw)
			if err != nil {
				return next, "", err
			}
		} else {
			return next, "", errors.New("no custom wallpaper selected")
		}
	default:
		return next, "", errors.New("unsupported wallpaper mode")
	}

	c, err := readConfig()
	if err == nil {
		c.Wallpaper = next
		_ = saveConfig(c)
	}

	if b != nil && b.device != "" {
		if next.Mode == "color" {
			if _, err := b.call("wallpaper clear"); err != nil {
				warning = "Saved solid colour; will apply when TomTom connects."
			} else {
				_ = b.batch([]string{
					"face-set background " + next.Color,
					"theme-set background " + next.Color,
				})
			}
		} else if len(rgb565) == wallpaperRGB565Size {
			if err := uploadWallpaper(b.device, rgb565); err != nil {
				warning = "Saved wallpaper; will sync to TomTom when connected."
			} else if _, err := b.call("wallpaper load"); err != nil {
				warning = "Saved wallpaper; will sync to TomTom when connected."
			}
		}
	}
	return next, warning, nil
}

func activeWallpaperPreview(w WallpaperConfig) string {
	var raw []byte
	switch w.Mode {
	case "color":
		return ""
	case "pc":
		data, _, err := readPCWallpaper()
		if err != nil {
			return ""
		}
		raw = data
	case "preset":
		p, err := findWallpaperPresetPath(w.Preset)
		if err != nil {
			return ""
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return ""
		}
		raw = data
	case "custom":
		if !wallpaperIDRegex.MatchString(w.CustomID) {
			return ""
		}
		data, err := os.ReadFile(filepath.Join(wallpaperDir(), w.CustomID+".png"))
		if err != nil {
			return ""
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	default:
		return ""
	}
	_, pngThumb, err := imageToWallpaper(raw)
	if err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngThumb)
}

func wallpaperSupervisor(ctx context.Context, b *Broker) {
	syncOnce := func() {
		c, err := readConfig()
		if err != nil {
			return
		}
		statusRaw, err := b.call("status")
		if err != nil {
			return
		}
		var status struct {
			WallpaperActive bool `json:"wallpaper_active"`
		}
		if json.Unmarshal([]byte(statusRaw), &status) != nil {
			return
		}
		switch c.Wallpaper.Mode {
		case "pc":
			raw, hash, err := readPCWallpaper()
			if err != nil {
				return
			}
			if hash != c.Wallpaper.LastPCHash || !status.WallpaperActive {
				wallpaperMu.Lock()
				err = pushWallpaperToDevice(b, raw)
				if err == nil {
					c2, e2 := readConfig()
					if e2 == nil && c2.Wallpaper.Mode == "pc" {
						c2.Wallpaper.LastPCHash = hash
						_ = saveConfig(c2)
					}
				}
				wallpaperMu.Unlock()
			}
		case "custom":
			if !status.WallpaperActive && wallpaperIDRegex.MatchString(c.Wallpaper.CustomID) {
				if raw, err := os.ReadFile(filepath.Join(wallpaperDir(), c.Wallpaper.CustomID+".png")); err == nil {
					wallpaperMu.Lock()
					_ = pushWallpaperToDevice(b, raw)
					wallpaperMu.Unlock()
				}
			}
		case "preset":
			if !status.WallpaperActive && c.Wallpaper.Preset != "" {
				if presetPath, err := findWallpaperPresetPath(c.Wallpaper.Preset); err == nil {
					if raw, err := os.ReadFile(presetPath); err == nil {
						wallpaperMu.Lock()
						_ = pushWallpaperToDevice(b, raw)
						wallpaperMu.Unlock()
					}
				}
			}
		}
	}

	// Initial sync shortly after hub startup so newly paired/updated devices get the wallpaper immediately.
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

func registerWallpaper(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/wallpaper", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		switch r.Method {
		case "GET":
			c, err := readConfig()
			if err != nil {
				http.Error(w, "config error", 500)
				return
			}
			_, _, pcErr := readPCWallpaper()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"config":         c.Wallpaper,
				"history":        readWallpaperHistory(true),
				"presets":        listWallpaperPresets(),
				"pc_available":   pcErr == nil,
				"active_preview": activeWallpaperPreview(c.Wallpaper),
			})
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
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid wallpaper request", 400)
				return
			}
			if req.Operation == "get" {
				c, err := readConfig()
				if err != nil {
					http.Error(w, "config error", 500)
					return
				}
				_, _, pcErr := readPCWallpaper()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"config":         c.Wallpaper,
					"history":        readWallpaperHistory(true),
					"presets":        listWallpaperPresets(),
					"pc_available":   pcErr == nil,
					"active_preview": activeWallpaperPreview(c.Wallpaper),
				})
				return
			}
			if req.Operation == "delete" || req.DeleteID != "" {
				if !wallpaperIDRegex.MatchString(req.DeleteID) {
					http.Error(w, "invalid wallpaper id", 400)
					return
				}
				wallpaperMu.Lock()
				_ = os.Remove(filepath.Join(wallpaperDir(), req.DeleteID+".png"))
				existing := readWallpaperHistory(false)
				var kept []wallpaperHistoryEntry
				for _, e := range existing {
					if e.ID != req.DeleteID {
						kept = append(kept, e)
					}
				}
				_ = saveWallpaperHistory(kept)
				wallpaperMu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"history": readWallpaperHistory(true)})
				return
			}
			var imgBytes []byte
			if req.ImageBase64 != "" {
				b64 := req.ImageBase64
				if idx := strings.Index(b64, ","); idx >= 0 {
					b64 = b64[idx+1:]
				}
				var err error
				imgBytes, err = base64.StdEncoding.DecodeString(b64)
				if err != nil {
					http.Error(w, "invalid image data", 400)
					return
				}
			}
			next := WallpaperConfig{
				Mode:     req.Mode,
				Color:    req.Color,
				Preset:   req.Preset,
				CustomID: req.CustomID,
			}
			applied, warning, err := applyWallpaper(b, next, imgBytes, req.Name)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"config":         applied,
				"history":        readWallpaperHistory(true),
				"active_preview": activeWallpaperPreview(applied),
				"warning":        warning,
			})
		case "DELETE":
			id := r.URL.Query().Get("id")
			if !wallpaperIDRegex.MatchString(id) {
				http.Error(w, "invalid wallpaper id", 400)
				return
			}
			wallpaperMu.Lock()
			_ = os.Remove(filepath.Join(wallpaperDir(), id+".png"))
			existing := readWallpaperHistory(false)
			var kept []wallpaperHistoryEntry
			for _, e := range existing {
				if e.ID != id {
					kept = append(kept, e)
				}
			}
			_ = saveWallpaperHistory(kept)
			wallpaperMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"history": readWallpaperHistory(true)})
		default:
			http.Error(w, "method not allowed", 405)
		}
	})
}
