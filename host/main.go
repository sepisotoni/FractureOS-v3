package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const version = "0.1.11"

//go:embed ui.html
var page string

type Config struct {
	LyricsEnabled      bool               `json:"lyrics_enabled"`
	WindowsTemperature bool               `json:"windows_temperature"`
	Device             string             `json:"device"`
	Hub                string             `json:"hub"`
	Listen             string             `json:"listen"`
	Token              string             `json:"token"`
	Fingerprint        string             `json:"fingerprint"`
	HubMetrics         bool               `json:"hub_metrics"`
	MetricsDisk        string             `json:"metrics_disk"`
	MetricsInterface   string             `json:"metrics_interface"`
	MediaEnabled       bool               `json:"media_enabled"`
	MediaStatus        bool               `json:"media_status"`
	Weather            WeatherConfig      `json:"weather"`
	Notifications      NotificationConfig `json:"notifications"`
}

func configDir() string {
	p, err := os.UserConfigDir()
	if err != nil {
		fatal(err)
	}
	return filepath.Join(p, "fracture-v3")
}
func readConfig() (Config, error) {
	c := Config{Device: "192.168.101.115", Hub: "https://127.0.0.1:47831", Listen: "127.0.0.1:47831", Weather: defaultWeather(), LyricsEnabled: true}
	b, e := os.ReadFile(filepath.Join(configDir(), "config.json"))
	if errors.Is(e, os.ErrNotExist) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func saveConfig(c Config) error {
	if e := validateIntegrations(c.Weather, c.Notifications); e != nil {
		return e
	}
	if c.MetricsDisk != "" && (!filepath.IsAbs(c.MetricsDisk) || len(c.MetricsDisk) > 512) {
		return errors.New("metrics disk must be an absolute mount path")
	}
	if len(c.MetricsInterface) > 32 || (c.MetricsInterface != "" && !regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`).MatchString(c.MetricsInterface)) {
		return errors.New("invalid network interface")
	}
	if net.ParseIP(c.Device) == nil {
		return errors.New("device must be an IP address")
	}
	if !strings.HasPrefix(c.Hub, "https://") {
		return errors.New("hub requires https://")
	}
	if _, _, e := net.SplitHostPort(c.Listen); e != nil {
		return e
	}
	if c.Fingerprint != "" {
		b, e := hex.DecodeString(c.Fingerprint)
		if e != nil || len(b) != 32 {
			return errors.New("fingerprint must be 64 hex characters")
		}
	}
	if e := os.MkdirAll(configDir(), 0700); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	path := filepath.Join(configDir(), "config.json")
	tmp, e := os.CreateTemp(configDir(), "settings-*.tmp")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if _, e = tmp.Write(b); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	return replaceFile(tmp.Name(), path)
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func fatal(e error) { fmt.Fprintln(os.Stderr, "FractureOS:", e); os.Exit(1) }
func validateCommand(cmd string) error {
	a := strings.Fields(cmd)
	if len(a) == 0 || len(cmd) > 240 {
		return errors.New("invalid command")
	}
	n := func(i, lo, hi int) bool {
		if i >= len(a) {
			return false
		}
		v, e := strconv.Atoi(a[i])
		return e == nil && v >= lo && v <= hi
	}
	switch a[0] {
	case "forecast-reload", "weather-clear", "media-clear", "status", "face-get", "face-reset", "theme-get", "theme-reset":
		if len(a) == 1 {
			return nil
		}
	case "face", "theme", "theme-save":
		if len(a) == 2 && len(a[1]) <= 31 && regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`).MatchString(a[1]) {
			return nil
		}
	case "face-set", "theme-set":
		if len(a) == 3 && regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(a[1]) && regexp.MustCompile(`^[a-z0-9,._-]+$`).MatchString(a[2]) && !strings.Contains(a[2], "..") {
			return nil
		}
	case "ui-timeout":
		if len(a) == 3 && ((a[1] == "menu" && n(2, 0, 600)) || (a[1] == "notification" && n(2, 1, 120))) {
			return nil
		}
	case "notice-text":
		if len(a) == 3 && (a[1] == "app" || a[1] == "title" || a[1] == "body") && mediaTextValid(a[2]) {
			return nil
		}
	case "notice-art":
		if len(a) == 2 && (a[1] == "load" || a[1] == "clear") {
			return nil
		}
	case "notice-show":
		if len(a) == 2 && n(1, 0, 120) {
			return nil
		}
	case "media-lyrics":
		if len(a) == 3 && (a[1] == "previous" || a[1] == "current" || a[1] == "next" || a[1] == "message") && mediaTextValid(a[2]) {
			return nil
		}
	case "lyrics-state":
		if len(a) == 2 && n(1, 0, 1) {
			return nil
		}
	case "media-text":
		if len(a) == 3 && (a[1] == "title" || a[1] == "artist" || a[1] == "message") && mediaTextValid(a[2]) {
			return nil
		}
	case "notice":
		if len(a) == 2 && mediaTextValid(a[1]) {
			return nil
		}
	case "lyrics-begin":
		if len(a) == 2 && regexp.MustCompile(`^(?:[1-9]|1[0-2]),[0-9]{1,10}$`).MatchString(a[1]) {
			return nil
		}
	case "preview-asset":
		if len(a) == 3 && (a[1] == "atlas" || a[1] == "font" || a[1] == "notice") && n(2, 0, 170000) {
			return nil
		}
	case "lyrics-line":
		if len(a) == 3 && n(1, 0, 86400000) && mediaTextValid(a[2]) {
			return nil
		}
	case "lyrics-commit", "home-state":
		if len(a) == 1 {
			return nil
		}
	case "media-clock":
		if len(a) == 2 {
			var at int64
			var playing int
			if count, e := fmt.Sscanf(a[1], "%d,%d", &at, &playing); e == nil && count == 2 && at >= 0 && at <= 86400000 && (playing == 0 || playing == 1) && fmt.Sprintf("%d,%d", at, playing) == a[1] {
				return nil
			}
		}
	case "stats-bundle":
		if len(a) == 3 && (a[2] == "hub" || a[2] == "pc") {
			var cpu, ram, temp, down, up int
			if count, e := fmt.Sscanf(a[1], "%d,%d,%d,%d,%d", &cpu, &ram, &temp, &down, &up); e == nil && count == 5 && cpu >= 0 && cpu <= 100 && ram >= 0 && ram <= 100 && temp >= -1000 && temp <= 1500 && down >= -1 && up >= -1 && fmt.Sprintf("%d,%d,%d,%d,%d", cpu, ram, temp, down, up) == a[1] {
				return nil
			}
		}
	case "media-state":
		if len(a) == 2 && regexp.MustCompile(`^[0-9,-]{1,80}$`).MatchString(a[1]) {
			return nil
		}
	case "media-art":
		if len(a) == 2 && (a[1] == "load" || a[1] == "clear") {
			return nil
		}
	case "media-ack":
		if len(a) == 2 {
			if _, e := strconv.ParseUint(a[1], 10, 32); e == nil {
				return nil
			}
		}
	case "weather-state":
		if len(a) == 2 {
			var code, temp, unusual int
			if count, e := fmt.Sscanf(a[1], "%d,%d,%d", &code, &temp, &unusual); e == nil && count == 3 && code >= 0 && code <= 99 && temp >= -1000 && temp <= 700 && (unusual == 0 || unusual == 1) && fmt.Sprintf("%d,%d,%d", code, temp, unusual) == a[1] {
				return nil
			}
		}
	case "feature":
		if len(a) == 3 && n(2, 0, 2) {
			switch a[1] {
			case "weather", "appearance":
				return nil
			case "telemetry", "media", "notifications":
				if n(2, 0, 1) {
					return nil
				}
			}
		}
	case "link", "hold-indicator":
		if len(a) == 2 && n(1, 0, 1) {
			return nil
		}
	case "stopwatch":
		if len(a) == 2 && (a[1] == "toggle" || a[1] == "reset" || a[1] == "lap") {
			return nil
		}
	case "profile":
		if len(a) == 2 && (a[1] == "minimal" || a[1] == "desk" || a[1] == "bedside") {
			return nil
		}
	case "brightness":
		if len(a) == 2 && n(1, 1, 100) {
			return nil
		}
	case "offset":
		if len(a) == 2 && n(1, -720, 840) {
			return nil
		}
	case "timer":
		if len(a) == 2 && n(1, 0, 86400) {
			return nil
		}
	case "alarm":
		if len(a) == 2 && a[1] == "off" {
			return nil
		}
		if len(a) == 3 && n(1, 0, 23) && n(2, 0, 59) {
			return nil
		}
	case "stats-temp":
		if (len(a) == 2 || (len(a) == 3 && (a[2] == "hub" || a[2] == "pc"))) && n(1, -1000, 1500) {
			return nil
		}
	case "stats-extra":
		if len(a) == 3 && n(1, -1, 100) && regexp.MustCompile(`^-?[0-9]{1,10},-?[0-9]{1,10}$`).MatchString(a[2]) {
			return nil
		}
	case "stats":
		if len(a) == 3 && n(1, 0, 100) && n(2, 0, 100) {
			return nil
		}
	case "time":
		if len(a) == 2 && n(1, 1700000000, 2147483647) {
			return nil
		}
	case "event":
		if len(a) == 2 {
			for _, v := range []string{"tap", "double", "hold", "power", "long", "home", "back"} {
				if a[1] == v {
					return nil
				}
			}
		}
	}
	return errors.New("unsupported command or invalid value")
}
func identity(c *Config) error {
	if e := os.MkdirAll(configDir(), 0700); e != nil {
		return e
	}
	certPath := filepath.Join(configDir(), "hub.crt")
	keyPath := filepath.Join(configDir(), "hub.key")
	if _, e := os.Stat(certPath); errors.Is(e, os.ErrNotExist) {
		pub, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		if e != nil {
			return e
		}
		tmpl := x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "FractureOS local hub"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(5, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, e := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, pub, priv)
		if e != nil {
			return e
		}
		key, e := x509.MarshalPKCS8PrivateKey(priv)
		if e != nil {
			return e
		}
		if e = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); e != nil {
			return e
		}
		if e = os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); e != nil {
			return e
		}
	}
	b, e := os.ReadFile(certPath)
	if e != nil {
		return e
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return errors.New("invalid hub certificate")
	}
	sum := sha256.Sum256(block.Bytes)
	c.Fingerprint = hex.EncodeToString(sum[:])
	if c.Token == "" {
		c.Token, e = randomToken()
		if e != nil {
			return e
		}
	}
	return saveConfig(*c)
}
func hubCall(c Config, cmd string) (string, error) {
	if e := validateCommand(cmd); e != nil {
		return "", e
	}
	return hubRequest(c, "/api/command", map[string]string{"command": cmd})
}
func hubRequest(c Config, path string, payload any) (string, error) {
	if !strings.HasPrefix(c.Hub, "https://") {
		return "", errors.New("hub requires https://")
	}
	if c.Token == "" || c.Fingerprint == "" {
		return "", errors.New("configure hub token and certificate fingerprint in Settings")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("no server certificate")
		}
		sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
		expected, e := hex.DecodeString(c.Fingerprint)
		if e != nil || subtle.ConstantTimeCompare(sum[:], expected) != 1 {
			return errors.New("hub certificate fingerprint mismatch")
		}
		cert := cs.PeerCertificates[0]
		if time.Now().Before(cert.NotBefore) || time.Now().After(cert.NotAfter) {
			return errors.New("hub certificate expired or not yet valid")
		}
		return nil
	}}}
	defer transport.CloseIdleConnections()
	requestTimeout := 12 * time.Second
	if path != "/api/command" {
		requestTimeout = 35 * time.Second
	}
	client := http.Client{Transport: transport, Timeout: requestTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("hub redirects refused") }}
	body, _ := json.Marshal(payload)
	req, e := http.NewRequest("POST", strings.TrimRight(c.Hub, "/")+path, bytes.NewReader(body))
	if e != nil {
		return "", e
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, e := client.Do(req)
	if e != nil {
		return "", e
	}
	defer res.Body.Close()
	limit := int64(8192)
	if path == "/api/preview" {
		limit = 384 * 1024
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if len(data) > int(limit) {
		return "", errors.New("hub response exceeds limit")
	}
	if e != nil {
		return "", e
	}
	if res.StatusCode != 200 {
		return "", fmt.Errorf("hub: %s", strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(string(data)), nil
}

type Broker struct {
	mu                 sync.Mutex
	device             string
	cached             string
	cachedAt           time.Time
	temperaturePCUntil time.Time
	mediaMu            sync.Mutex
	mediaLatest        lyricsSession
	mediaReceived      time.Time
	providedLyrics     *lyricsRecord
}

func (b *Broker) call(cmd string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cmd == "status" && time.Since(b.cachedAt) < time.Second {
		return b.cached, nil
	}
	if strings.HasPrefix(cmd, "stats-temp ") && strings.HasSuffix(cmd, " hub") && time.Now().Before(b.temperaturePCUntil) {
		return "OK", nil
	}
	if strings.HasPrefix(cmd, "stats-bundle ") && strings.HasSuffix(cmd, " hub") && time.Now().Before(b.temperaturePCUntil) {
		parts := strings.Split(strings.Fields(cmd)[1], ",")
		parts[2] = "-1000"
		cmd = "stats-bundle " + strings.Join(parts, ",") + " hub"
	}
	if strings.HasPrefix(cmd, "weather-state ") {
		conf, err := readConfig()
		if err != nil {
			return "", err
		}
		if !conf.Weather.Enabled {
			return "OK disabled", nil
		}
	}
	out, e := deviceCommand(b.device, cmd)
	if e == nil && strings.HasPrefix(cmd, "stats-temp ") && strings.HasSuffix(cmd, " pc") {
		b.temperaturePCUntil = time.Now().Add(8 * time.Second)
	}
	if e == nil && cmd == "status" {
		b.cached = out
		b.cachedAt = time.Now()
	} else if cmd != "stats" && !strings.HasPrefix(cmd, "stats ") {
		b.cachedAt = time.Time{}
	}
	return out, e
}
func (b *Broker) batch(commands []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := deviceCommands(b.device, commands)
	b.cachedAt = time.Time{}
	return err
}
func serve() error {
	c, e := readConfig()
	if e != nil {
		return e
	}
	if e = identity(&c); e != nil {
		return e
	}
	b := &Broker{device: c.Device}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	mux := http.NewServeMux()
	registerIntegrations(mux, b, c.Token)
	registerLyrics(mux, b, c.Token)
	registerForecast(mux, b, c.Token)
	registerPreview(mux, b, c.Token)
	mux.HandleFunc("/api/media", func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+c.Token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		mediaAPI(b, w, r)
	})
	mux.HandleFunc("/api/command", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+c.Token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "POST required", 405)
			return
		}
		var q struct {
			Command string `json:"command"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if e := dec.Decode(&q); e != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		if e := validateCommand(q.Command); e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		out, e := b.call(q.Command)
		if e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprintln(w, out)
	})
	srv := &http.Server{Addr: c.Listen, Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 30 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	go func() {
		<-ctx.Done()
		shutdown, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		_ = srv.Shutdown(shutdown)
	}()
	go hubBackground(ctx, b)
	go weatherSupervisor(ctx, b)
	go lyricsSupervisor(ctx, b)
	log.Printf("FractureOS hub %s listening on https://%s; device %s", version, c.Listen, c.Device)
	e = srv.ListenAndServeTLS(filepath.Join(configDir(), "hub.crt"), filepath.Join(configDir(), "hub.key"))
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}

// Each enabled collector is a separate process; cancellation always reaps it.
func launchMetrics(ctx context.Context) (*exec.Cmd, context.CancelFunc, <-chan error, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, nil, err
	}
	child, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(child, exe, "metrics-worker")
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return cmd, cancel, done, nil
}
func hubBackground(ctx context.Context, b *Broker) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var worker *exec.Cmd
	var cancel context.CancelFunc
	var done <-chan error
	lastSync := time.Time{}
	statusFailure := time.Time{}
	stopWorker := func() {
		if worker != nil {
			cancel()
			<-done
			worker = nil
			done = nil
		}
	}
	defer stopWorker()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			cancel()
			worker = nil
			done = nil
		case <-ticker.C:
			c, err := readConfig()
			if err != nil {
				stopWorker()
				continue
			}
			if time.Since(lastSync) > 5*time.Minute {
				if _, err = b.call(fmt.Sprintf("time %d", time.Now().Unix())); err == nil {
					lastSync = time.Now()
				}
			}
			_, _ = b.call("link 1")
			if !c.HubMetrics {
				stopWorker()
				continue
			}
			status, err := b.call("status")
			if err != nil {
				if statusFailure.IsZero() {
					statusFailure = time.Now()
				}
				if time.Since(statusFailure) > 5*time.Second {
					stopWorker()
				}
				continue
			}
			statusFailure = time.Time{}
			if strings.Contains(status, `"telemetry":0`) || (!strings.Contains(status, `"profile":"desk"`) || !strings.Contains(status, `"menu":0`)) {
				stopWorker()
				continue
			}
			if worker == nil {
				worker, cancel, done, _ = launchMetrics(ctx)
			}
		}
	}
}

func settings(open bool) error {
	token, e := randomToken()
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	mux := http.NewServeMux()
	var lastMu sync.Mutex
	last := time.Now()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.URL.Path == "/favicon.ico" {
			w.Header().Set("Content-Type", "image/x-icon")
			_, _ = w.Write(brandICO)
			return
		}
		if r.URL.Path == "/fracture.svg" {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write(brandSVG)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, page)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Fracture-Session")), []byte(token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		lastMu.Lock()
		last = time.Now()
		lastMu.Unlock()
		c, e := readConfig()
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		switch r.URL.Path {
		case "/update":
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			var q struct {
				Action string `json:"action"`
			}
			r.Body = http.MaxBytesReader(w, r.Body, 256)
			if json.NewDecoder(r.Body).Decode(&q) != nil {
				http.Error(w, "invalid update request", 400)
				return
			}
			if q.Action == "check" {
				offer, err := checkUpdate(r.Context())
				if err != nil {
					http.Error(w, err.Error(), 502)
					return
				}
				_ = json.NewEncoder(w).Encode(offer)
			} else if q.Action == "install" {
				result, err := installUpdate(r.Context())
				if err != nil {
					http.Error(w, err.Error(), 502)
					return
				}
				fmt.Fprint(w, result)
			} else {
				http.Error(w, "unknown update action", 400)
			}
		case "/integrations":
			if r.Method != "GET" && r.Method != "POST" {
				http.Error(w, "GET or POST required", 405)
				return
			}
			var payload any = map[string]string{"operation": "get"}
			if r.Method == "POST" {
				var q IntegrationSettings
				r.Body = http.MaxBytesReader(w, r.Body, 4096)
				if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
					http.Error(w, "invalid settings", 400)
					return
				}
				q.Operation = "set"
				payload = q
			}
			out, err := hubRequest(c, "/api/integrations", payload)
			if err != nil {
				http.Error(w, err.Error(), 502)
				return
			}
			fmt.Fprint(w, out)
		case "/config":
			if r.Method == "GET" {
				c.Token = ""
				_ = json.NewEncoder(w).Encode(c)
				return
			}
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			var newc Config
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if e = json.NewDecoder(r.Body).Decode(&newc); e != nil {
				http.Error(w, "invalid configuration", 400)
				return
			}
			// Hub integrations are edited through /integrations. Preserve the latest
			// values when saving local connection/media settings from an older form.
			newc.Weather = c.Weather
			newc.Notifications = c.Notifications
			if newc.Token == "" {
				newc.Token = c.Token
			}
			if e = saveConfig(newc); e != nil {
				http.Error(w, e.Error(), 400)
				return
			}
			fmt.Fprint(w, "Saved")
		case "/command":
			if r.Method != "POST" {
				http.Error(w, "POST required", 405)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1024)
			var q struct {
				Command string `json:"command"`
			}
			if e = json.NewDecoder(r.Body).Decode(&q); e != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			out, e := hubCall(c, q.Command)
			if e != nil {
				http.Error(w, e.Error(), 502)
				return
			}
			fmt.Fprint(w, out)
		default:
			http.NotFound(w, r)
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 120 * time.Second}
	go func() {
		for range time.Tick(30 * time.Second) {
			lastMu.Lock()
			idle := time.Since(last)
			lastMu.Unlock()
			if idle > 5*time.Minute {
				_ = srv.Close()
				return
			}
		}
	}()
	url := "http://" + listener.Addr().String() + "/#" + token
	if open {
		if e = openURL(url); e != nil {
			return e
		}
	} else {
		fmt.Println(url)
	}
	e = srv.Serve(listener)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
func main() {
	if len(os.Args) == 1 {
		if e := platformDefault(); e != nil {
			fatal(e)
		}
		return
	}
	a := os.Args[1:]
	var e error
	switch a[0] {
	case "version":
		fmt.Println("FractureOS", version)
	case "serve":
		e = serve()
	case "dashboard":
		e = platformDashboard()
	case "settings":
		if len(a) > 1 && a[1] == "--no-open" {
			e = settings(false)
		} else {
			e = platformDashboard()
		}
	case "web":
		e = settings(true)
	case "apply-update":
		e = applyUpdate(a[1:])
	case "update":
		var result string
		result, e = installUpdate(context.Background())
		if e == nil {
			fmt.Println(result)
		}
	case "temperature-authorize":
		e = authorizedTemperatureWorker()
	case "temperature-helper":
		if len(a) != 4 {
			e = errors.New("invalid sensor helper arguments")
		} else {
			e = sensorHelper(a[1], a[2], a[3])
		}
	case "temperature-worker":
		e = windowsTemperatureWorker(false)
	case "pawnio-check":
		e = windowsTemperatureWorker(true)
	case "pawnio-modules":
		e = installPawnModules()
	case "lyrics-worker":
		e = lyricsWorker()
	case "weather-worker":
		e = weatherWorker()
	case "notify":
		if len(a) != 4 {
			e = errors.New("usage: fracture notify APP TITLE BODY")
		} else {
			var c Config
			c, e = readConfig()
			if e == nil {
				_, e = hubRequest(c, "/api/notifications", Notification{App: a[1], Title: a[2], Body: a[3]})
			}
		}
	case "tray":
		e = platformTray()
	case "media-worker":
		e = localMediaWorker()
	case "metrics-worker":
		e = metricsWorker()
	case "uninstall":
		e = platformUninstall()
	case "device":
		e = deviceManagement(a[1:])
	case "pair-info":
		var c Config
		c, e = readConfig()
		if e == nil {
			b, _ := json.MarshalIndent(c, "", "  ")
			fmt.Println(string(b))
		}
	case "config":
		e = configCommand(a[1:])
	case "help", "--help", "-h":
		help()
	default:
		var c Config
		c, e = readConfig()
		if e == nil {
			var out string
			out, e = hubCall(c, strings.Join(a, " "))
			if e == nil {
				fmt.Println(out)
			}
		}
	}
	if e != nil {
		fatal(e)
	}
}
func configCommand(a []string) error {
	c, e := readConfig()
	if e != nil {
		return e
	}
	if len(a) == 0 || a[0] == "get" {
		c.Token = "[redacted]"
		b, _ := json.MarshalIndent(c, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if len(a) != 3 || a[0] != "set" {
		return errors.New("usage: fracture config set KEY VALUE")
	}
	switch a[1] {
	case "hub":
		c.Hub = a[2]
	case "device":
		c.Device = a[2]
	case "listen":
		c.Listen = a[2]
	case "token":
		c.Token = a[2]
	case "fingerprint":
		c.Fingerprint = a[2]
	case "media_enabled":
		c.MediaEnabled, e = strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
	case "media_status":
		c.MediaStatus, e = strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
	case "windows_temperature":
		c.WindowsTemperature, e = strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
	case "weather_enabled":
		c.Weather.Enabled, e = strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
	case "weather_latitude", "weather_longitude", "weather_hot", "weather_cold", "weather_wind":
		var v float64
		v, e = strconv.ParseFloat(a[2], 64)
		if e != nil {
			return e
		}
		switch a[1] {
		case "weather_latitude":
			c.Weather.Latitude = v
		case "weather_longitude":
			c.Weather.Longitude = v
		case "weather_hot":
			c.Weather.Hot = v
		case "weather_cold":
			c.Weather.Cold = v
		case "weather_wind":
			c.Weather.Wind = v
		}
	case "notifications_enabled":
		c.Notifications.Enabled, e = strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
	case "metrics_disk":
		c.MetricsDisk = a[2]
	case "metrics_interface":
		if a[2] == "auto" {
			c.MetricsInterface = ""
		} else {
			c.MetricsInterface = a[2]
		}
	case "hub_metrics":
		v, e := strconv.ParseBool(a[2])
		if e != nil {
			return e
		}
		c.HubMetrics = v
	default:
		return errors.New("unknown configuration key")
	}
	return saveConfig(c)
}
func help() {
	fmt.Println(`FractureOS 0.1.11
  fracture status | profile minimal|desk|bedside | theme teal|amber|blue
  fracture theme NAME | theme-get | theme-set KEY VALUE | theme-save NEW_NAME | theme-reset
  fracture face NAME | face-get | face-set KEY VALUE | face-reset
  fracture brightness 1..100 | offset -720..840 | timer SECONDS
  fracture alarm HOUR MINUTE | alarm off
  fracture settings                 Open settings (unloads after idle)
  fracture config get | config set KEY VALUE
  fracture pair-info                Show private hub pairing credentials
  fracture serve                    Run TLS hub
  fracture device install|restore   Linux USB host only
  fracture uninstall                Remove host app; keep user settings
  fracture version
No resource collector starts unless hub_metrics is enabled and Desk is visible.`)
}
