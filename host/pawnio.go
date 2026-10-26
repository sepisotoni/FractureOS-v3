package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Official signed modules, pinned to the upstream release asset's SHA-256.
// Downloaded only from Settings on request; never shipped to the TomTom.
func installPawnModules() error {
	client := http.Client{Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()
	res, e := client.Get("https://github.com/namazso/PawnIO.Modules/releases/download/0.2.11/release_0_2_11.zip")
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("module download: HTTP %d", res.StatusCode)
	}
	data, e := io.ReadAll(io.LimitReader(res.Body, 1048577))
	if e != nil {
		return e
	}
	if len(data) > 1048576 || fmt.Sprintf("%x", sha256.Sum256(data)) != "43608cb89bc84247fef1368a139013f7d043e17db6d6c8dfc9b46bf0905a81f4" {
		return fmt.Errorf("PawnIO module checksum mismatch")
	}
	archive, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		return e
	}
	dir := filepath.Join(configDir(), "pawnio")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	found := 0
	for _, f := range archive.File {
		base := filepath.Base(f.Name)
		if base != "IntelMSR.bin" && base != "AMDFamily17.bin" {
			continue
		}
		if f.UncompressedSize64 > 65536 {
			return fmt.Errorf("oversized module")
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(r, 65537))
		r.Close()
		if e != nil {
			return e
		}
		if len(b) > 65536 {
			return fmt.Errorf("oversized module")
		}
		tmp := filepath.Join(dir, base+".new")
		if e = os.WriteFile(tmp, b, 0600); e != nil {
			return e
		}
		if e = replaceFile(tmp, filepath.Join(dir, base)); e != nil {
			return e
		}
		found++
	}
	if found != 2 {
		return fmt.Errorf("expected two official modules, found %d", found)
	}
	return nil
}
