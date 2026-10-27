package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func platformInstallUpdate(offer updateOffer, archive, stage string) (string, error) {
	defer os.Remove(archive)
	defer os.Remove(stage)
	// The existing versioned installer validates the downloaded archive again.
	// Fetch the matching source script, then point it at the public release.
	scriptURL := "https://raw.githubusercontent.com/sepisotoni/FractureOS-v3/" + offer.Version + "/install/install.sh"
	script, e := exec.Command("curl", "-fsSL", "--proto", "=https", scriptURL).Output()
	if e != nil {
		return "", e
	}
	if len(script) > 65536 {
		return "", fmt.Errorf("installer source too large")
	}
	path := filepath.Join(stage, "install.sh")
	if e = os.WriteFile(path, script, 0600); e != nil {
		return "", e
	}
	defer os.Remove(path)
	cmd := exec.Command("sh", path)
	cmd.Env = append(os.Environ(), "FRACTURE_VERSION="+offer.Version)
	output, e := cmd.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("update failed: %w: %s", e, strings.TrimSpace(string(output)))
	}
	return "Updated to " + offer.Version + ". Reopen settings to load the new app.", nil
}
func applyUpdate(args []string) error { return fmt.Errorf("Windows installer helper only") }
