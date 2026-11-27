package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// fakeShellDevice speaks the same telnet/shell framing as a TomTom: a login
// prompt, a "# " prompt, "{ cmd }; echo MARKER=$?" blocks, control.sh calls and
// the wget-back upload. It lets the real deviceCommands/uploadDevicePayload
// code run end to end on loopback. It is written from reading the host code,
// so it proves the host's own framing and flow, not the firmware's behaviour.
type fakeShellDevice struct {
	ln      net.Listener
	mu      sync.Mutex
	calls   [][]string        // one entry per control.sh invocation
	files   map[string][]byte // paths fetched by "wget -O"
	replies map[string]string // first control arg -> stdout
	failOn  string            // first control arg that exits non-zero
	hook    func(args []string) (string, bool)
}

var (
	markerRE = regexp.MustCompile(`^\}; fracture_result=\$\?; echo; echo (__FRACTURE_[0-9a-f]+)=\$fracture_result$`)
	wgetRE   = regexp.MustCompile(`^wget -q -O (\S+)\.new 'http://([^']+)' && mv (\S+)\.new (\S+)$`)
	quotedRE = regexp.MustCompile(`'([^']*)'`)
)

func startFakeShellDevice(t *testing.T) *fakeShellDevice {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeShellDevice{ln: ln, files: map[string][]byte{}, replies: map[string]string{}}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	old := devicePort
	devicePort = port
	t.Cleanup(func() { devicePort = old; _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handle(c)
		}
	}()
	return f
}

func (f *fakeShellDevice) handle(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	_, _ = c.Write([]byte("login: "))
	if _, err := r.ReadString('\n'); err != nil {
		return
	}
	_, _ = c.Write([]byte("# "))
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "exit" {
			return
		}
		if line != "{" {
			continue
		}
		var body []string
		marker := ""
		for marker == "" {
			l, err := r.ReadString('\n')
			if err != nil {
				return
			}
			l = strings.TrimRight(l, "\r\n")
			if m := markerRE.FindStringSubmatch(l); m != nil {
				marker = m[1]
			} else {
				body = append(body, l)
			}
		}
		out, rc := f.exec(strings.Join(body, "\n"))
		fmt.Fprintf(c, "%s\n%s=%d\n", out, marker, rc)
	}
}

func (f *fakeShellDevice) exec(cmd string) (string, int) {
	switch {
	case strings.HasPrefix(cmd, "stty"):
		return "", 0
	case strings.HasPrefix(cmd, "wget "):
		m := wgetRE.FindStringSubmatch(cmd)
		if m == nil || m[1] != m[3] {
			return "bad wget command", 1
		}
		resp, err := http.Get("http://" + m[2])
		if err != nil {
			return err.Error(), 1
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Sprintf("wget: HTTP %d", resp.StatusCode), 1
		}
		data, _ := io.ReadAll(resp.Body)
		f.mu.Lock()
		f.files[m[4]] = data
		f.mu.Unlock()
		return "", 0
	case strings.Contains(cmd, "control.sh"):
		var outs []string
		for _, seg := range strings.Split(cmd, " && ") {
			var args []string
			for _, q := range quotedRE.FindAllStringSubmatch(seg, -1) {
				args = append(args, q[1])
			}
			if len(args) == 0 { // unquoted form used by preview-asset downloads
				if i := strings.Index(seg, "control.sh"); i >= 0 {
					args = strings.Fields(seg[i+len("control.sh"):])
				}
			}
			if len(args) == 0 {
				return "no command", 1
			}
			f.mu.Lock()
			f.calls = append(f.calls, args)
			fail, reply, hook := f.failOn, f.replies[args[0]], f.hook
			f.mu.Unlock()
			if fail != "" && args[0] == fail {
				return "ERR " + args[0], 1
			}
			if hook != nil {
				if out, ok := hook(args); ok {
					outs = append(outs, out)
					continue
				}
			}
			if reply == "" {
				reply = "OK"
			}
			outs = append(outs, reply)
		}
		return strings.Join(outs, "\n"), 0
	}
	return "unknown command", 127
}

func (f *fakeShellDevice) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (f *fakeShellDevice) saw(prefix string) bool {
	for _, c := range f.commands() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (f *fakeShellDevice) file(path string) []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.files[path]
}

func (f *fakeShellDevice) reset() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}
