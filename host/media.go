package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"
	"time"
)

type mediaDeviceStatus struct {
	LyricsView bool   `json:"lyrics_view"`
	LyricsPage int    `json:"lyrics_page"`
	Enabled    *int   `json:"media_enabled"`
	View       bool   `json:"media_view"`
	Menu       int    `json:"menu"`
	Art        int    `json:"media_art_loaded"`
	Pending    bool   `json:"media_pending"`
	Sequence   uint   `json:"media_seq"`
	Action     string `json:"media_action"`
	Argument   int    `json:"media_arg"`
}
type mediaSnapshot struct {
	PositionMS *int64 `json:"position_ms,omitempty"`
	SampledAt  int64  `json:"sampled_at,omitempty"`
	Album      string `json:"album"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Message    string `json:"message"`
	Playing    bool   `json:"playing"`
	Position   int    `json:"position"`
	Duration   int    `json:"duration"`
	Volume     int    `json:"volume"`
	VolumeOK   bool   `json:"volume_ok"`
	PlayOK     bool   `json:"play_ok"`
	PreviousOK bool   `json:"previous_ok"`
	NextOK     bool   `json:"next_ok"`
	Artwork    []byte `json:"artwork,omitempty"`
	ClearArt   bool   `json:"clear_art"`
	Clear      bool   `json:"clear"`
}

func asciiHex(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 80 {
			break
		}
		if r < 32 {
			r = ' '
		}
		if r > 126 {
			r = '?'
		}
		b.WriteByte(byte(r))
	}
	if b.Len() == 0 {
		return "-"
	}
	return hex.EncodeToString([]byte(b.String()))
}
func mediaTextValid(s string) bool {
	if s == "-" {
		return true
	}
	if len(s) > 160 || len(s)%2 != 0 {
		return false
	}
	b, e := hex.DecodeString(s)
	if e != nil {
		return false
	}
	for _, c := range b {
		if c < 32 || c > 126 {
			return false
		}
	}
	return true
}
func mediaAPI(b *Broker, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	var s mediaSnapshot
	if json.NewDecoder(r.Body).Decode(&s) != nil || s.Position < 0 || s.Duration < 0 || s.Duration > 86400 || s.Position > s.Duration || s.Volume < -1 || s.Volume > 100 || len(s.Title) > 512 || len(s.Artist) > 512 || len(s.Album) > 512 || len(s.Message) > 512 || (s.PositionMS != nil && (*s.PositionMS < 0 || *s.PositionMS > int64(s.Duration)*1000)) || (len(s.Artwork) != 0 && len(s.Artwork) != 18432) {
		http.Error(w, "invalid media update", 400)
		return
	}
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	received := time.Now()
	positionMS := int64(s.Position) * 1000
	if s.PositionMS != nil {
		positionMS = *s.PositionMS
	}
	// Timestamped Windows samples account for transport delay, bounded against
	// clock skew. Older companions retain their integer-second fallback.
	if s.Playing && s.SampledAt > 0 {
		age := received.UnixMilli() - s.SampledAt
		if age > 0 && age < 15000 {
			positionMS += age
		}
	}
	if positionMS > int64(s.Duration)*1000 {
		positionMS = int64(s.Duration) * 1000
	}
	commands := []string{}
	if s.Clear {
		commands = append(commands, "media-clear")
	} else {
		commands = append(commands, "media-text title "+asciiHex(s.Title), "media-text artist "+asciiHex(s.Artist), fmt.Sprintf("media-state %d,%d,%d,%d,%d,%d,%d,%d", bit(s.Playing), s.Position, s.Duration, s.Volume, bit(s.VolumeOK), bit(s.PlayOK), bit(s.PreviousOK), bit(s.NextOK)))
	}
	commands = append(commands, "media-text message "+asciiHex(s.Message))
	if !s.Clear && s.PositionMS != nil {
		commands = append(commands, fmt.Sprintf("media-clock %d,%d", positionMS+int64(bit(s.Playing))*time.Since(received).Milliseconds(), bit(s.Playing)))
	}
	if s.ClearArt {
		commands = append(commands, "media-art clear")
	}
	if e := b.batch(commands); e != nil {
		http.Error(w, "device update failed", 502)
		return
	}

	b.mediaMu.Lock()
	if s.Clear {
		b.mediaLatest = lyricsSession{}
		b.providedLyrics = nil
	} else {
		if b.mediaLatest.Title != s.Title || b.mediaLatest.Artist != s.Artist {
			b.providedLyrics = nil
		}
		b.mediaLatest = lyricsSession{Title: s.Title, Artist: s.Artist, Album: s.Album, Position: int(positionMS / 1000), PositionMS: positionMS, Duration: s.Duration, Playing: s.Playing}
		b.mediaReceived = received
	}
	b.mediaMu.Unlock()

	if len(s.Artwork) > 0 {
		if e := uploadMediaArt(b.device, s.Artwork); e != nil {
			http.Error(w, "artwork transfer failed", 502)
			return
		}
		if _, e := b.call("media-art load"); e != nil {
			http.Error(w, "artwork no longer requested", 409)
			return
		}
	}
	fmt.Fprint(w, `{"updated":true}`)
}
func thumbnailRGB(data []byte) ([]byte, error) { return thumbnailRGBSize(data, 96) }
func thumbnailRGBSize(data []byte, size int) ([]byte, error) {
	if len(data) > 2*1024*1024 {
		return nil, errors.New("thumbnail too large")
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 2048 || cfg.Height > 2048 {
		return nil, errors.New("invalid thumbnail")
	}
	im, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, e
	}
	out := make([]byte, size*size*2)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			r, g, b, _ := im.At(im.Bounds().Min.X+x*cfg.Width/size, im.Bounds().Min.Y+y*cfg.Height/size).RGBA()
			v := uint16((r>>11)<<11 | (g>>10)<<5 | (b >> 11))
			at := (y*size + x) * 2
			out[at] = byte(v)
			out[at+1] = byte(v >> 8)
		}
	}
	return out, nil
}
