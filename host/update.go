package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type updateOffer struct {
	Version   string `json:"version"`
	Current   string `json:"current"`
	Available bool   `json:"available"`
	Page      string `json:"page"`
}

var releaseTag = regexp.MustCompile(`^v([0-9]{1,6})\.([0-9]{1,6})\.([0-9]{1,6})$`)

func compareVersions(a, b string) int {
	aa := releaseTag.FindStringSubmatch("v" + strings.TrimPrefix(a, "v"))
	bb := releaseTag.FindStringSubmatch("v" + strings.TrimPrefix(b, "v"))
	if aa == nil || bb == nil {
		return 0
	}
	for i := 1; i < 4; i++ {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x > y {
			return 1
		}
		if x < y {
			return -1
		}
	}
	return 0
}
func updateClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 6 || req.URL.Scheme != "https" {
			return errors.New("update redirect refused")
		}
		host := req.URL.Hostname()
		if host != "github.com" && host != "api.github.com" && host != "raw.githubusercontent.com" && host != "release-assets.githubusercontent.com" && host != "objects.githubusercontent.com" {
			return errors.New("unexpected update download host")
		}
		return nil
	}}
}
func checkUpdate(ctx context.Context) (updateOffer, error) {
	offer := updateOffer{Current: version, Version: "v" + version, Page: "https://github.com/sepisotoni/FractureOS-v3/releases"}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/sepisotoni/FractureOS-v3/releases?per_page=20", nil)
	if e != nil {
		return offer, e
	}
	req.Header.Set("User-Agent", "FractureOS/"+version)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := updateClient(20 * time.Second)
	defer client.CloseIdleConnections()
	res, e := client.Do(req)
	if e != nil {
		return offer, e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return offer, fmt.Errorf("release check returned HTTP %d", res.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 512*1024+1))
	if e != nil {
		return offer, e
	}
	if len(data) > 512*1024 {
		return offer, errors.New("release list too large")
	}
	var releases []struct {
		Tag   string `json:"tag_name"`
		Draft bool   `json:"draft"`
	}
	if e = json.Unmarshal(data, &releases); e != nil {
		return offer, e
	}
	for _, r := range releases {
		if !r.Draft && releaseTag.MatchString(r.Tag) && compareVersions(r.Tag, offer.Version) > 0 {
			offer.Version = r.Tag
		}
	}
	offer.Available = compareVersions(offer.Version, version) > 0
	offer.Page = "https://github.com/sepisotoni/FractureOS-v3/releases/tag/" + offer.Version
	return offer, nil
}
func downloadUpdate(ctx context.Context, offer updateOffer) (string, string, error) {
	if !offer.Available || !releaseTag.MatchString(offer.Version) {
		return "", "", errors.New("no newer release")
	}
	suffix := ""
	switch runtime.GOOS {
	case "windows":
		suffix = "windows-" + runtime.GOARCH + "-setup.exe"
	case "linux":
		suffix = "linux-" + runtime.GOARCH + ".tar.gz"
	default:
		return "", "", errors.New("unsupported update platform")
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", "", errors.New("unsupported architecture")
	}
	name := "fracture-" + offer.Version + "-" + suffix
	base := "https://github.com/sepisotoni/FractureOS-v3/releases/download/" + url.PathEscape(offer.Version) + "/"
	client := updateClient(90 * time.Second)
	defer client.CloseIdleConnections()
	fetch := func(target string, limit int64) ([]byte, error) {
		req, e := http.NewRequestWithContext(ctx, "GET", base+target, nil)
		if e != nil {
			return nil, e
		}
		res, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("download %s: HTTP %d", target, res.StatusCode)
		}
		data, e := io.ReadAll(io.LimitReader(res.Body, limit+1))
		if e != nil {
			return nil, e
		}
		if int64(len(data)) > limit {
			return nil, errors.New("update file too large")
		}
		return data, nil
	}
	manifest, e := fetch("SHA256SUMS", 16384)
	if e != nil {
		return "", "", e
	}
	digest := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == name {
			if digest != "" {
				return "", "", errors.New("ambiguous update checksum")
			}
			digest = parts[0]
		}
	}
	expected, e := hex.DecodeString(digest)
	if e != nil || len(expected) != 32 {
		return "", "", errors.New("missing update checksum")
	}
	data, e := fetch(name, 64*1024*1024)
	if e != nil {
		return "", "", e
	}
	actual := sha256.Sum256(data)
	if !strings.EqualFold(digest, hex.EncodeToString(actual[:])) {
		return "", "", errors.New("update checksum mismatch")
	}
	stage, e := os.MkdirTemp("", "fracture-update-")
	if e != nil {
		return "", "", e
	}
	path := filepath.Join(stage, name)
	if e = os.WriteFile(path, data, 0600); e != nil {
		os.Remove(stage)
		return "", "", e
	}
	return path, stage, nil
}
func installUpdate(ctx context.Context) (string, error) {
	offer, e := checkUpdate(ctx)
	if e != nil {
		return "", e
	}
	if !offer.Available {
		return "Already up to date (" + version + ")", nil
	}
	path, stage, e := downloadUpdate(ctx, offer)
	if e != nil {
		return "", e
	}
	return platformInstallUpdate(offer, path, stage)
}
