package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func replaceFile(a, b string) error { return os.Rename(a, b) }
func openURL(url string) error {
	cmd := exec.Command("xdg-open", url)
	if e := cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	return nil
}
func platformDefault() error { help(); return nil }
func platformTray() error    { return errors.New("Linux uses the CLI or fracture settings") }
func assetsPath() string {
	if p := os.Getenv("FRACTURE_ASSETS"); p != "" {
		return p
	}
	exe, _ := os.Executable()
	p := filepath.Join(filepath.Dir(exe), "..", "share", "fracture-v3", "current")
	if _, e := os.Stat(filepath.Join(p, "tools", "deploy-device.py")); e == nil {
		return p
	}
	cwd, _ := os.Getwd()
	return cwd
}
func deviceManagement(args []string) error {
	if len(args) != 1 || (args[0] != "install" && args[0] != "restore") {
		return errors.New("usage: fracture device install|restore")
	}
	c, e := readConfig()
	if e != nil {
		return e
	}
	p := assetsPath()
	cmd := exec.Command("python3", filepath.Join(p, "tools", "deploy-device.py"), args[0], "--host", c.Device, "--package", p)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func platformUninstall() error {
	p := filepath.Join(assetsPath(), "install", "uninstall.sh")
	cmd := exec.Command("sh", p)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func cpuSample() (uint64, uint64, error) {
	b, e := os.ReadFile("/proc/stat")
	if e != nil {
		return 0, 0, e
	}
	fields := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(fields) < 5 {
		return 0, 0, errors.New("invalid proc stat")
	}
	var total, idle uint64
	for i := 1; i < len(fields) && i <= 8; i++ {
		n, e := strconv.ParseUint(fields[i], 10, 64)
		if e != nil {
			return 0, 0, e
		}
		total += n
		if i == 4 || i == 5 {
			idle += n
		}
	}
	return total, idle, nil
}
func memoryPercent() (int, error) {
	f, e := os.Open("/proc/meminfo")
	if e != nil {
		return 0, e
	}
	defer f.Close()
	var total, available uint64
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		v := strings.Fields(scan.Text())
		if len(v) > 1 {
			n, _ := strconv.ParseUint(v[1], 10, 64)
			if v[0] == "MemTotal:" {
				total = n
			}
			if v[0] == "MemAvailable:" {
				available = n
			}
		}
	}
	if total == 0 {
		return 0, errors.New("missing memory totals")
	}
	return int((total - available) * 100 / total), scan.Err()
}
func metricsWorker() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	total, idle, e := cpuSample()
	if e != nil {
		return e
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c, e := readConfig()
			if e != nil || !c.HubMetrics {
				return nil
			}
			newTotal, newIdle, e := cpuSample()
			if e != nil {
				continue
			}
			usage := 0
			if newTotal > total && newIdle >= idle && newIdle-idle <= newTotal-total {
				usage = int(100 * (newTotal - total - (newIdle - idle)) / (newTotal - total))
			}
			total, idle = newTotal, newIdle
			memory, e := memoryPercent()
			if e != nil {
				continue
			}
			_, _ = hubCall(c, fmt.Sprintf("stats %d %d", usage, memory))
		}
	}
}
