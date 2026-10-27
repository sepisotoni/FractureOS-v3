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
	previousNet := map[string][2]uint64{}
	netAt := time.Now()
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
			_, _ = hubCall(c, fmt.Sprintf("stats-temp %d hub", cpuTemperature()))
			disk := -1 // Disk was replaced by CPU temperature on the clock.

			samples, err := networkSample(c.MetricsInterface)
			down, up := -1, -1
			now := time.Now()
			if err == nil {
				var rx, tx uint64
				valid := false
				for name, n := range samples {
					if old, ok := previousNet[name]; ok && n[0] >= old[0] && n[1] >= old[1] {
						rx += n[0] - old[0]
						tx += n[1] - old[1]
						valid = true
					}
				}
				if valid {
					rate := func(v uint64) int {
						r := float64(v) / 1024 / now.Sub(netAt).Seconds()
						if r > 2147483647 {
							r = 2147483647
						}
						return int(r)
					}
					down = rate(rx)
					up = rate(tx)
				}
			}
			previousNet = samples
			netAt = now
			_, _ = hubCall(c, fmt.Sprintf("stats-extra %d %d,%d", disk, down, up))
		}
	}
}

// Default-route interfaces avoid counting the same bytes again on container
// bridges and the TomTom USB management link. An explicit interface is optional.
func networkSample(selected string) (map[string][2]uint64, error) {
	interfaces := map[string]bool{}
	if selected != "" {
		interfaces[selected] = true
	} else {
		b, e := os.ReadFile("/proc/net/route")
		if e != nil {
			return nil, e
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 4 && f[1] == "00000000" {
				flags, _ := strconv.ParseUint(f[3], 16, 32)
				if flags&1 != 0 {
					interfaces[f[0]] = true
				}
			}
		}
	}
	b, e := os.ReadFile("/proc/net/dev")
	if e != nil {
		return nil, e
	}
	out := map[string][2]uint64{}
	for _, line := range strings.Split(string(b), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || !interfaces[name] {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		rx, e1 := strconv.ParseUint(f[0], 10, 64)
		tx, e2 := strconv.ParseUint(f[8], 10, 64)
		if e1 == nil && e2 == nil {
			out[name] = [2]uint64{rx, tx}
		}
	}
	return out, nil
}

func localMediaWorker() error { return errors.New("Spotify client sharing runs on Windows") }

func platformDashboard() error { return settings(true) }

// Only CPU-specific drivers, never an NVMe, motherboard or arbitrary ACPI sensor.
func cpuTemperature() int {
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	best := -1000
	bestPriority := -1
	for _, dir := range dirs {
		data, e := os.ReadFile(filepath.Join(dir, "name"))
		if e != nil {
			continue
		}
		name := strings.TrimSpace(string(data))
		switch name {
		case "coretemp", "k10temp", "zenpower", "cpu_thermal":
		default:
			continue
		}
		paths, _ := filepath.Glob(filepath.Join(dir, "temp*_input"))
		for _, path := range paths {
			b, e := os.ReadFile(path)
			if e != nil {
				continue
			}
			n, e := strconv.Atoi(strings.TrimSpace(string(b)))
			if e != nil || n < -50000 || n > 150000 {
				continue
			}
			label, _ := os.ReadFile(strings.TrimSuffix(path, "_input") + "_label")
			kind := strings.ToLower(strings.TrimSpace(string(label)))
			priority := 0
			if strings.HasPrefix(kind, "package") || kind == "tdie" {
				priority = 2
			} else if kind == "tctl" {
				priority = 1
			}
			if priority > bestPriority || (priority == bestPriority && n/100 > best) {
				best = n / 100
				bestPriority = priority
			}
		}
	}
	return best
}

func windowsTemperatureWorker(check bool) error {
	return errors.New("PawnIO temperature collection runs on Windows x64")
}

func authorizedTemperatureWorker() error { return errors.New("Sensor authorization runs on Windows") }
func sensorHelper(endpoint, token, modules string) error {
	return errors.New("Sensor helper runs on Windows")
}
