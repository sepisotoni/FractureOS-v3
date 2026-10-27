package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Trusted USB development/management channel. Never expose TomTom Telnet to LAN.
type telnet struct {
	c           net.Conn
	state, verb byte
}

func (t *telnet) readUntil(pattern *regexp.Regexp) (string, error) {
	var data []byte
	buf := make([]byte, 4096)
	for len(data) < 64*1024 {
		n, err := t.c.Read(buf)
		if err != nil {
			return "", err
		}
		for _, b := range buf[:n] {
			switch t.state {
			case 0:
				if b == 255 {
					t.state = 1
				} else if b != 13 && b != 0 {
					data = append(data, b)
				}
			case 1:
				if b == 255 {
					data = append(data, b)
					t.state = 0
				} else if b >= 251 && b <= 254 {
					t.verb = b
					t.state = 2
				} else if b == 250 {
					t.state = 3
				} else {
					t.state = 0
				}
			case 2:
				if t.verb == 251 {
					reply := byte(254)
					if b == 1 || b == 3 {
						reply = 253
					}
					if _, err = t.c.Write([]byte{255, reply, b}); err != nil {
						return "", err
					}
				} else if t.verb == 253 {
					if _, err = t.c.Write([]byte{255, 252, b}); err != nil {
						return "", err
					}
				}
				t.state = 0
			case 3:
				if b == 255 {
					t.state = 4
				}
			case 4:
				if b == 240 {
					t.state = 0
				} else {
					t.state = 3
				}
			}
		}
		if pattern.Match(data) {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("device response exceeds limit")
}
func (t *telnet) command(command string) (string, error) {
	key := make([]byte, 12)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	marker := "__FRACTURE_" + hex.EncodeToString(key)
	_, err := fmt.Fprintf(t.c, "{\n%s\n}; fracture_result=$?; echo; echo %s=$fracture_result\r\n", command, marker)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\n` + marker + `=(\d+)\n`)
	out, err := t.readUntil(re)
	if err != nil {
		return "", err
	}
	parts := re.FindStringSubmatchIndex(out)
	body := strings.TrimSpace(out[:parts[0]])
	if out[parts[2]:parts[3]] != "0" {
		return "", fmt.Errorf("device error: %s", body)
	}
	return body, nil
}
func deviceCommand(address, command string) (string, error) {
	if err := validateCommand(command); err != nil {
		return "", err
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(address, "23"), 3*time.Second)
	if err != nil {
		return "", err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(8 * time.Second))
	t := telnet{c: c}
	if _, err = t.readUntil(regexp.MustCompile(`login: *$`)); err != nil {
		return "", err
	}
	if _, err = c.Write([]byte("root\r\n")); err != nil {
		return "", err
	}
	if _, err = t.readUntil(regexp.MustCompile(`(?m)^# *$`)); err != nil {
		return "", err
	}
	if _, err = t.command("stty -echo; PS1=''; PS2=''"); err != nil {
		return "", err
	}
	var cmd bytes.Buffer
	cmd.WriteString("/bin/sh /mnt/sdcard/fracture-v3/control.sh")
	// Validator limits values; still quote each argument independently.
	for _, arg := range strings.Fields(command) {
		cmd.WriteString(" '")
		cmd.WriteString(strings.ReplaceAll(arg, "'", "'\\''"))
		cmd.WriteByte('\'')
	}
	out, err := t.command(cmd.String())
	_, _ = c.Write([]byte("exit\r\n"))
	return out, err
}

// Artwork is a fixed 18 KiB RGB565 image, served once over the trusted USB hop.
// Nothing supplied by Spotify is interpolated into the shell command.
func uploadMediaArt(address string, data []byte) error { return uploadDeviceArt(address, data, false) }
func uploadDeviceArt(address string, data []byte, notice bool) error {
	kind := 0
	if notice {
		kind = 1
	}
	return uploadDevicePayload(address, data, kind)
}
func uploadDevicePayload(address string, data []byte, kind int) error {
	if (kind == 0 && len(data) != 18432) || (kind == 1 && len(data) != 4608) || (kind == 2 && (len(data) < 32 || len(data) > 8192)) || kind < 0 || kind > 2 {
		return fmt.Errorf("invalid artwork size")
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(address, "23"), 3*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(12 * time.Second))
	t := telnet{c: c}
	if _, err = t.readUntil(regexp.MustCompile(`login: *$`)); err != nil {
		return err
	}
	if _, err = c.Write([]byte("root\r\n")); err != nil {
		return err
	}
	if _, err = t.readUntil(regexp.MustCompile(`(?m)^# *$`)); err != nil {
		return err
	}
	if _, err = t.command("stty -echo; PS1=''; PS2=''"); err != nil {
		return err
	}
	local := c.LocalAddr().(*net.TCPAddr).IP.String()
	ln, err := net.Listen("tcp", net.JoinHostPort(local, "0"))
	if err != nil {
		return err
	}
	key, err := randomToken()
	if err != nil {
		ln.Close()
		return err
	}
	srv := &http.Server{ReadHeaderTimeout: 2 * time.Second, WriteTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, _, _ := net.SplitHostPort(r.RemoteAddr)
		if peer != address || r.URL.Path != "/"+key || r.Method != "GET" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		_, _ = w.Write(data)
	})}
	defer srv.Close()
	go func() { _ = srv.Serve(ln) }()
	destination := "/tmp/fracture-media.rgb"
	if kind == 1 {
		destination = "/tmp/fracture-notice.rgb"
	}
	if kind == 2 {
		destination = "/mnt/sdcard/fracture-v3/weather.dat"
	}
	_, err = t.command("wget -q -O " + destination + ".new 'http://" + ln.Addr().String() + "/" + key + "' && mv " + destination + ".new " + destination)
	_, _ = c.Write([]byte("exit\r\n"))
	return err
}
