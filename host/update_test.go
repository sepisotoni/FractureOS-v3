package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
)

func fakeReleaseServer(t *testing.T, releasesJSON string, status int, sums func(name string) string, archive []byte) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		if status != 200 {
			http.Error(w, "boom", status)
			return
		}
		fmt.Fprint(w, releasesJSON)
	})
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/SHA256SUMS"):
			parts := strings.Split(r.URL.Path, "/")
			name := "fracture-" + parts[2] + "-" + updateSuffix()
			fmt.Fprint(w, sums(name))
		case strings.Contains(r.URL.Path, "fracture-"):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewTLSServer(mux)
	oldList, oldDL, oldTr := releaseListURL, releaseDownloadURL, updateTransport
	releaseListURL, releaseDownloadURL, updateTransport = srv.URL+"/releases", srv.URL+"/dl/", srv.Client().Transport
	t.Cleanup(func() { srv.Close(); releaseListURL, releaseDownloadURL, updateTransport = oldList, oldDL, oldTr })
}

func updateSuffix() string {
	if runtime.GOOS == "windows" {
		return "windows-" + runtime.GOARCH + "-setup.exe"
	}
	return "linux-" + runtime.GOARCH + ".tar.gz"
}

func TestCheckUpdateOffers(t *testing.T) {
	cur := "v" + version
	cases := []struct {
		name, json string
		status     int
		wantAvail  bool
		wantTag    string
		wantErr    bool
	}{
		{"newer release", `[{"tag_name":"v99.0.0","draft":false},{"tag_name":"v1.0.0","draft":false}]`, 200, true, "v99.0.0", false},
		{"four-part hotfix", `[{"tag_name":"v99.0.0.1","draft":false},{"tag_name":"v99.0.0","draft":false}]`, 200, true, "v99.0.0.1", false},
		{"incomplete release skipped", `[{"tag_name":"v99.0.0.1","assets":[{"name":"SHA256SUMS","state":"uploaded"}]},{"tag_name":"v99.0.0","assets":[{"name":"SHA256SUMS","state":"uploaded"},{"name":"fracture-v99.0.0-` + updateSuffix() + `","state":"uploaded"}]}]`, 200, true, "v99.0.0", false},
		{"draft ignored", `[{"tag_name":"v99.0.0","draft":true}]`, 200, false, cur, false},
		{"junk tag ignored", `[{"tag_name":"nightly","draft":false},{"tag_name":"v99.x","draft":false}]`, 200, false, cur, false},
		{"already current", `[{"tag_name":"` + cur + `","draft":false}]`, 200, false, cur, false},
		{"older only", `[{"tag_name":"v0.0.1","draft":false}]`, 200, false, cur, false},
		{"server error", ``, 500, false, "", true},
		{"malformed json", `{`, 200, false, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeReleaseServer(t, c.json, c.status, nil, nil)
			offer, err := checkUpdate(context.Background())
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if err == nil && (offer.Available != c.wantAvail || offer.Version != c.wantTag) {
				t.Fatalf("offer=%+v", offer)
			}
		})
	}
}

func TestDownloadUpdateVerifiesChecksum(t *testing.T) {
	archive := []byte("pretend this is a release archive")
	sum := sha256.Sum256(archive)
	good := hex.EncodeToString(sum[:])
	offer := updateOffer{Version: "v99.0.0", Available: true}
	cases := []struct {
		name    string
		sums    func(string) string
		wantErr string
	}{
		{"valid", func(n string) string { return good + "  " + n + "\n" }, ""},
		{"tampered archive", func(n string) string { return strings.Repeat("0", 64) + "  " + n + "\n" }, "checksum mismatch"},
		{"missing entry", func(n string) string { return good + "  other-file\n" }, "missing update checksum"},
		{"ambiguous entry", func(n string) string { return good + "  " + n + "\n" + good + "  " + n + "\n" }, "ambiguous"},
		{"short digest", func(n string) string { return "abcd  " + n + "\n" }, "missing update checksum"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakeReleaseServer(t, `[]`, 200, c.sums, archive)
			path, stage, err := downloadUpdate(context.Background(), offer)
			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				data, _ := os.ReadFile(path)
				if string(data) != string(archive) {
					t.Fatal("staged file differs")
				}
				_ = os.RemoveAll(stage)
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) || path != "" {
				t.Fatalf("want %q, got path=%q err=%v", c.wantErr, path, err)
			}
		})
	}
	if _, _, err := downloadUpdate(context.Background(), updateOffer{Version: "v99.0.0", Available: false}); err == nil {
		t.Fatal("no-update offer must not download")
	}
	if _, _, err := downloadUpdate(context.Background(), updateOffer{Version: "../../etc", Available: true}); err == nil {
		t.Fatal("malformed version must not download")
	}
}

func TestUpdateRouteActionsAndUpToDateInstall(t *testing.T) {
	e := newRouteEnv(t)
	fakeReleaseServer(t, `[{"tag_name":"v99.0.0","draft":false}]`, 200, nil, nil)
	if code, body := uiDo(t, e, "POST", "/update", `{"action":"check"}`, uiSession); code != 200 || !strings.Contains(body, `"available":true`) {
		t.Fatalf("check: %d %s", code, body)
	}
	if code, _ := uiDo(t, e, "POST", "/update", `{"action":"explode"}`, uiSession); code != 400 {
		t.Fatalf("unknown action = %d", code)
	}
	if code, _ := uiDo(t, e, "POST", "/update", `nope`, uiSession); code != 400 {
		t.Fatalf("bad json = %d", code)
	}
	fakeReleaseServer(t, `[{"tag_name":"v0.0.1","draft":false}]`, 200, nil, nil)
	if code, body := uiDo(t, e, "POST", "/update", `{"action":"install"}`, uiSession); code != 200 || !strings.Contains(body, "Already up to date") {
		t.Fatalf("install when current: %d %s", code, body)
	}
	fakeReleaseServer(t, ``, 503, nil, nil)
	if code, body := uiDo(t, e, "POST", "/update", `{"action":"check"}`, uiSession); code != 502 || !strings.Contains(body, "503") {
		t.Fatalf("release server down: %d %s", code, body)
	}
}
