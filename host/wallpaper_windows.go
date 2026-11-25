package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const spiGetDeskWallpaper = 0x0073

func readPCWallpaper() ([]byte, string, error) {
	buf := make([]uint16, 260)
	ret, _, _ := user32.NewProc("SystemParametersInfoW").Call(
		spiGetDeskWallpaper,
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&buf[0])),
		0,
	)
	var candidates []string
	if ret != 0 {
		if path := syscall.UTF16ToString(buf); path != "" {
			candidates = append(candidates, path)
		}
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		candidates = append(candidates, filepath.Join(appData, "Microsoft", "Windows", "Themes", "TranscodedWallpaper"))
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			sum := sha256.Sum256(data)
			return data, hex.EncodeToString(sum[:]), nil
		}
	}
	return nil, "", errors.New("Windows desktop wallpaper unavailable")
}
