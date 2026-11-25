package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestImageToWallpaperSizeAndBMP(t *testing.T) {
	// Test PNG cover-crop and RGB565 sizing.
	src := image.NewRGBA(image.Rect(0, 0, 640, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 640; x++ {
			src.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	rgb565, pngBytes, err := imageToWallpaper(buf.Bytes())
	if err != nil {
		t.Fatalf("imageToWallpaper failed: %v", err)
	}
	if len(rgb565) != wallpaperRGB565Size {
		t.Fatalf("rgb565 size = %d, want %d", len(rgb565), wallpaperRGB565Size)
	}
	// Pure red in RGB565 little-endian is 0xF800 -> [0x00, 0xF8]
	if rgb565[0] != 0x00 || rgb565[1] != 0xf8 {
		t.Fatalf("pixel 0 = %02x %02x, want 00 f8", rgb565[0], rgb565[1])
	}
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 320 || decoded.Bounds().Dy() != 240 {
		t.Fatalf("decoded png size = %v, want 320x240", decoded.Bounds())
	}
}

func TestWallpaperHistoryLimitFive(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("APPDATA", tmp)

	for i := 0; i < 7; i++ {
		img := image.NewRGBA(image.Rect(0, 0, 16, 16))
		img.SetRGBA(0, 0, color.RGBA{R: byte(i * 30), G: 10, B: 20, A: 255})
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		_, err := addCustomWallpaperToHistory("test", buf.Bytes())
		if err != nil {
			t.Fatalf("addCustomWallpaperToHistory(%d): %v", i, err)
		}
	}

	hist := readWallpaperHistory(true)
	if len(hist) != maxCustomWallpapers {
		t.Fatalf("history len = %d, want %d", len(hist), maxCustomWallpapers)
	}
	for _, entry := range hist {
		if entry.Preview == "" {
			t.Errorf("history entry %s missing preview", entry.ID)
		}
		if _, err := os.Stat(filepath.Join(wallpaperDir(), entry.ID+".png")); err != nil {
			t.Errorf("missing png for %s: %v", entry.ID, err)
		}
	}
}

func TestListWallpaperPresetsIncludesTomTomPack(t *testing.T) {
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	presets := listWallpaperPresets()
	if len(presets) < 3 {
		t.Fatalf("expected >= 3 presets from tomtom-one-v6 pack, got %d", len(presets))
	}
	found := map[string]bool{}
	for _, p := range presets {
		found[p.Name] = p.Preview != ""
	}
	for _, want := range []string{"01-purple-blue-ribbon-flow.jpg", "02-midnight-sweep.jpg", "03-dual-current.jpg"} {
		if !found[want] {
			t.Errorf("missing preset or preview for %s", want)
		}
	}
}
