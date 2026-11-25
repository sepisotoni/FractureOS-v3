//go:build !windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

func readPCWallpaper() ([]byte, string, error) {
	// Best-effort GNOME / XFCE / KDE desktop wallpaper lookup on Linux.
	for _, key := range []string{"picture-uri-dark", "picture-uri"} {
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.background", key).Output()
		if err == nil {
			raw := strings.Trim(strings.TrimSpace(string(out)), "'\"")
			if strings.HasPrefix(raw, "file://") {
				if u, uErr := url.Parse(raw); uErr == nil {
					raw = u.Path
				}
			}
			if raw != "" {
				if data, rErr := os.ReadFile(raw); rErr == nil && len(data) > 0 {
					sum := sha256.Sum256(data)
					return data, hex.EncodeToString(sum[:]), nil
				}
			}
		}
	}
	return nil, "", errors.New("desktop wallpaper unavailable on this host")
}
