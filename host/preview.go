package main

// Home preview transports a small model plus content-addressed drawing assets.
// It never asks the device to capture its framebuffer.
import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type previewRequest struct {
	Operation string `json:"operation"`
	Kind      string `json:"kind,omitempty"`
	Hash      uint32 `json:"hash,omitempty"`
}
type previewReply struct {
	Model string `json:"model,omitempty"`
	Data  []byte `json:"data,omitempty"`
	Hash  uint32 `json:"hash,omitempty"`
}

func assetHash(data []byte) uint32 { h := fnv.New32a(); h.Write(data); return h.Sum32() }
func previewAssetSize(kind string) int {
	switch kind {
	case "atlas":
		return 170000
	case "font":
		return 45699
	case "notice":
		return 4608
	case "wallpaper":
		return 153600
	}
	return 0
}
func downloadPreviewAsset(address, kind string) ([]byte, error) {
	size := previewAssetSize(kind)
	if size == 0 {
		return nil, errors.New("unknown preview asset")
	}
	return previewTelnet(address, func(t *telnet) ([]byte, error) {
		return readPreviewChunks(t, size, func(at, _ int) string {
			return fmt.Sprintf("/bin/sh /mnt/sdcard/fracture-v3/control.sh preview-asset %s %d", kind, at)
		})
	})
}

func previewTelnet(address string, transfer func(*telnet) ([]byte, error)) ([]byte, error) {
	c, e := net.DialTimeout("tcp", net.JoinHostPort(address, devicePort), 3*time.Second)
	if e != nil {
		return nil, e
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(25 * time.Second))
	t := telnet{c: c}
	if _, e = t.readUntil(regexp.MustCompile(`login: *$`)); e != nil {
		return nil, e
	}
	if _, e = c.Write([]byte("root\r\n")); e != nil {
		return nil, e
	}
	if _, e = t.readUntil(regexp.MustCompile(`(?m)^# *$`)); e != nil {
		return nil, e
	}
	if _, e = t.command("stty -echo; PS1=''; PS2=''"); e != nil {
		return nil, e
	}
	defer c.Write([]byte("exit\r\n"))
	return transfer(&t)
}

func readPreviewChunks(t *telnet, size int, command func(int, int) string) ([]byte, error) {
	data := make([]byte, 0, size)
	for at := 0; at < size; at += 8192 {
		want := min(size-at, 8192)
		raw, e := t.command(command(at, want))
		if e != nil {
			return nil, e
		}
		chunk, e := hex.DecodeString(strings.TrimSpace(raw))
		if e != nil || len(chunk) != want {
			return nil, errors.New("short preview asset")
		}
		data = append(data, chunk...)
	}
	return data, nil
}

func captureTomTomScreen(address string) ([]byte, error) {
	const header = "P6\n320 240\n255\n"
	const width, height = 320, 240
	data, err := previewTelnet(address, func(t *telnet) ([]byte, error) {
		if out, err := t.command("/bin/sh /mnt/sdcard/fracture-v3/control.sh capture"); err != nil || strings.TrimSpace(out) != "OK" {
			return nil, errors.New("device capture failed")
		}
		if _, err := t.command("slot=$(cat /mnt/sdcard/fracture-v3/current); case \"$slot\" in ''|*[!a-zA-Z0-9._-]*|.|..) false;; *) probe=/mnt/sdcard/fracture-v3/releases/$slot/platform-probe; test -x \"$probe\";; esac"); err != nil {
			return nil, errors.New("device capture helper unavailable")
		}
		return readPreviewChunks(t, len(header)+width*height*3, func(at, count int) string {
			return fmt.Sprintf("\"$probe\" --hex /tmp/fracture-v3-screen.ppm %d %d", at, count)
		})
	})
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte(header)) {
		return nil, errors.New("invalid device capture")
	}
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for source, dest := len(header), 0; dest < len(frame.Pix); source, dest = source+3, dest+4 {
		copy(frame.Pix[dest:dest+3], data[source:source+3])
		frame.Pix[dest+3] = 255
	}
	var out bytes.Buffer
	err = png.Encode(&out, frame)
	return out.Bytes(), err
}

func registerPreview(mux *http.ServeMux, b *Broker, token string) {
	var capture sync.Mutex
	mux.HandleFunc("/api/preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 512)
		var q previewRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&q) != nil {
			http.Error(w, "invalid preview request", 400)
			return
		}
		var reply previewReply
		if q.Operation == "model" {
			model, e := b.call("home-state")
			if e != nil {
				http.Error(w, "home preview requires device 0.1.11 or later", 502)
				return
			}
			reply.Model = model + "\n"
		} else if q.Operation == "screen" {
			if !capture.TryLock() {
				http.Error(w, "a TomTom capture is already running", 429)
				return
			}
			data, err := captureTomTomScreen(b.device)
			capture.Unlock()
			if err != nil {
				http.Error(w, "could not read the TomTom screen: "+err.Error(), 502)
				return
			}
			reply.Data = data
		} else if q.Operation == "asset" && previewAssetSize(q.Kind) > 0 && q.Hash != 0 {
			data, e := downloadPreviewAsset(b.device, q.Kind)
			if e != nil {
				http.Error(w, "preview asset unavailable", 502)
				return
			}
			if assetHash(data) != q.Hash {
				http.Error(w, "preview assets changed; refresh", 409)
				return
			}
			reply.Data = data
			reply.Hash = q.Hash
		} else {
			http.Error(w, "invalid preview operation", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reply)
	})
}

type homePreviewPacket struct {
	Model                                          string
	Received                                       time.Time
	Atlas, Font, Notice, Wallpaper                 []byte
	AtlasHash, FontHash, NoticeHash, WallpaperHash uint32
}

func previewCachedAsset(c Config, kind string, hash uint32) ([]byte, error) {
	if hash == 0 {
		return nil, nil
	}
	dir := filepath.Join(configDir(), "preview-cache")
	path := filepath.Join(dir, fmt.Sprintf("%s-%08x.bin", kind, hash))
	size := previewAssetSize(kind)
	if info, e := os.Stat(path); e == nil && info.Size() == int64(size) {
		if data, e := os.ReadFile(path); e == nil && assetHash(data) == hash {
			return data, nil
		}
	}
	raw, e := hubRequest(c, "/api/preview", previewRequest{Operation: "asset", Kind: kind, Hash: hash})
	if e != nil {
		return nil, e
	}
	var reply previewReply
	if json.Unmarshal([]byte(raw), &reply) != nil || len(reply.Data) != size || assetHash(reply.Data) != hash {
		return nil, errors.New("invalid preview asset")
	}
	if os.MkdirAll(dir, 0700) == nil {
		tmp := path + ".new"
		if os.WriteFile(tmp, reply.Data, 0600) == nil {
			_ = replaceFile(tmp, path)
		}
		// Keep bounded asset history on disk, never an in-memory collection of faces.
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if !entry.IsDir() && entry.Name() != filepath.Base(path) && strings.HasPrefix(entry.Name(), kind+"-") && strings.HasSuffix(entry.Name(), ".bin") {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
	return reply.Data, nil
}
func fetchHomePreview(c Config, previous homePreviewPacket) (homePreviewPacket, error) {
	packet, err := fetchHomePreviewOnce(c, previous)
	if err == nil && time.Since(packet.Received) > time.Second {
		if fresh, freshErr := fetchHomePreviewOnce(c, packet); freshErr == nil {
			return fresh, nil
		}
	}
	return packet, err
}

func fetchHomePreviewOnce(c Config, previous homePreviewPacket) (homePreviewPacket, error) {
	raw, e := hubRequest(c, "/api/preview", previewRequest{Operation: "model"})
	if e != nil {
		return homePreviewPacket{}, e
	}
	var reply previewReply
	if json.Unmarshal([]byte(raw), &reply) != nil || len(reply.Model) > 8191 || !strings.HasPrefix(reply.Model, "FPR1\n") || !strings.HasSuffix(reply.Model, "END\n") {
		return homePreviewPacket{}, errors.New("invalid home preview model")
	}
	packet := homePreviewPacket{Model: reply.Model, Received: time.Now()}
	for _, line := range strings.Split(reply.Model, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, _ := strconv.ParseUint(v, 10, 32)
		switch k {
		case "atlas_hash":
			packet.AtlasHash = uint32(n)
		case "font_hash":
			packet.FontHash = uint32(n)
		case "notice_hash":
			packet.NoticeHash = uint32(n)
		case "wallpaper_hash":
			packet.WallpaperHash = uint32(n)
		}
	}
	if packet.AtlasHash == 0 {
		return homePreviewPacket{}, errors.New("home face is unavailable")
	}
	if packet.AtlasHash == previous.AtlasHash {
		packet.Atlas = previous.Atlas
	} else if packet.Atlas, e = previewCachedAsset(c, "atlas", packet.AtlasHash); e != nil {
		return homePreviewPacket{}, e
	}
	if packet.FontHash == previous.FontHash {
		packet.Font = previous.Font
	} else if packet.Font, e = previewCachedAsset(c, "font", packet.FontHash); e != nil {
		return homePreviewPacket{}, e
	}
	if packet.NoticeHash == previous.NoticeHash {
		packet.Notice = previous.Notice
	} else if packet.Notice, e = previewCachedAsset(c, "notice", packet.NoticeHash); e != nil {
		return homePreviewPacket{}, e
	}
	if packet.WallpaperHash == previous.WallpaperHash {
		packet.Wallpaper = previous.Wallpaper
	} else if packet.Wallpaper, e = previewCachedAsset(c, "wallpaper", packet.WallpaperHash); e != nil {
		packet.Wallpaper = nil
		packet.WallpaperHash = 0
	}
	return packet, nil
}
