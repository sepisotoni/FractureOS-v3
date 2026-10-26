package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Read-only sensor requests through the official PawnIO device IO-control ABI. No MSR writes,
// custom kernel code, driver bypass, or fallback to WinRing0.
func registryString(key, name string) string {
	adv := syscall.NewLazyDLL("advapi32.dll").NewProc("RegGetValueW")
	var kind uint32
	buf := make([]uint16, 512)
	size := uint32(len(buf) * 2)
	r, _, _ := adv.Call(0x80000002, uintptr(unsafe.Pointer(wide(key))), uintptr(unsafe.Pointer(wide(name))), 0x2, uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

type pawnSensor struct {
	handle syscall.Handle
	amd    bool
	model  string
}

func openPawnSensor() (*pawnSensor, error) {
	if runtime.GOARCH != "amd64" {
		return nil, errors.New("PawnIO temperature currently supports Windows x64")
	}
	vendor := registryString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "VendorIdentifier")
	module := "IntelMSR.bin"
	amd := vendor == "AuthenticAMD"
	if amd {
		module = "AMDFamily17.bin"
	} else if vendor != "GenuineIntel" {
		return nil, errors.New("unsupported CPU temperature provider")
	}
	blob, e := os.ReadFile(filepath.Join(configDir(), "pawnio", module))
	if e != nil || len(blob) == 0 || len(blob) > 65536 {
		return nil, errors.New("download the signed sensor modules from Settings first")
	}
	handle, e := syscall.CreateFile(wide(`\\?\GLOBALROOT\Device\PawnIO`), syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return nil, fmt.Errorf("install the official signed PawnIO driver and check access: %w", e)
	}
	var written uint32
	e = syscall.DeviceIoControl(handle, 41394<<16|0x821<<2, &blob[0], uint32(len(blob)), nil, 0, &written, nil)
	runtime.KeepAlive(blob)
	if e != nil {
		syscall.CloseHandle(handle)
		return nil, fmt.Errorf("PawnIO rejected sensor module: %w", e)
	}
	return &pawnSensor{handle: handle, amd: amd, model: registryString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString")}, nil
}
func (s *pawnSensor) release() { syscall.CloseHandle(s.handle) }
func (s *pawnSensor) read(name string, address uint64) (uint64, error) {
	var in [40]byte
	var out [8]byte
	var written uint32
	copy(in[:32], name)
	binary.LittleEndian.PutUint64(in[32:], address)
	e := syscall.DeviceIoControl(s.handle, 41394<<16|0x841<<2, &in[0], uint32(len(in)), &out[0], uint32(len(out)), &written, nil)
	if e != nil || written != 8 {
		return 0, errors.New("temperature register unavailable")
	}
	return binary.LittleEndian.Uint64(out[:]), nil
}
func (s *pawnSensor) temperature() (int, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var result int
	if s.amd {
		// Upstream requires the shared PCI bus mutex for indexed SMN reads.
		mutex, _, e := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(wide(`Global\Access_PCI`))))
		if mutex == 0 {
			return -1000, e
		}
		defer kernel32.NewProc("CloseHandle").Call(mutex)
		wait, _, _ := kernel32.NewProc("WaitForSingleObject").Call(mutex, 100)
		if wait != 0 && wait != 0x80 {
			return -1000, errors.New("CPU sensor bus is busy")
		}
		defer kernel32.NewProc("ReleaseMutex").Call(mutex)
		v, e := s.read("ioctl_read_smn", 0x59800)
		if e != nil {
			return -1000, e
		}
		millis := int((v>>21)&0x7ff) * 125
		if v&(1<<19) != 0 || v&(3<<16) == 3<<16 {
			millis -= 49000
		}
		switch {
		case strings.Contains(s.model, "1600X") || strings.Contains(s.model, "1700X") || strings.Contains(s.model, "1800X"):
			millis -= 20000
		case strings.Contains(s.model, "Threadripper 19") || strings.Contains(s.model, "Threadripper 29"):
			millis -= 27000
		case strings.Contains(s.model, "2700X"):
			millis -= 10000
		}
		result = millis / 100
	} else {
		thread, _, _ := kernel32.NewProc("GetCurrentThread").Call()
		old, _, e := kernel32.NewProc("SetThreadAffinityMask").Call(thread, 1)
		if old == 0 {
			return -1000, e
		}
		defer kernel32.NewProc("SetThreadAffinityMask").Call(thread, old)
		target, e := s.read("ioctl_read_msr", 0x1a2)
		if e != nil {
			return -1000, e
		}
		tj := int((target >> 16) & 255)
		if tj < 50 || tj > 125 {
			return -1000, errors.New("CPU temperature target is unavailable")
		}
		v, e := s.read("ioctl_read_msr", 0x1b1)
		if e != nil || v&(1<<31) == 0 {
			v, e = s.read("ioctl_read_msr", 0x19c)
		}
		if e != nil || v&(1<<31) == 0 {
			return -1000, errors.New("CPU temperature sample is invalid")
		}
		result = (tj - int((v>>16)&127)) * 10
	}
	if result < -500 || result > 1500 {
		return -1000, errors.New("CPU temperature outside sensor range")
	}
	return result, nil
}
func windowsTemperatureWorker(check bool) error {
	c, e := readConfig()
	if e != nil {
		return e
	}
	sensor, e := openPawnSensor()
	if e != nil {
		return e
	}
	defer sensor.release()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		temp, e := sensor.temperature()
		if check {
			if e != nil {
				return e
			}
			fmt.Printf("CPU temperature: %.1f C (PawnIO)\n", float64(temp)/10)
			return nil
		}
		if e != nil {
			temp = -1000
		}
		if !c.WindowsTemperature {
			return nil
		}
		_, _ = hubCall(c, fmt.Sprintf("stats-temp %d pc", temp))
		<-ticker.C
		c, e = readConfig()
		if e != nil {
			return e
		}
	}
}
func windowsTemperatureSupervisor(ctx context.Context) {
	timer := time.NewTicker(3 * time.Second)
	defer timer.Stop()
	var cancel context.CancelFunc
	var done <-chan error
	retry := time.Time{}
	stop := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
			done = nil
		}
	}
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			cancel()
			cancel = nil
			done = nil
			retry = time.Now().Add(time.Minute)
		case <-timer.C:
			c, e := readConfig()
			if e != nil || !c.WindowsTemperature {
				stop()
				continue
			}
			raw, e := hubCall(c, "status")
			var s struct {
				Menu      int `json:"menu"`
				Telemetry int `json:"telemetry"`
			}
			if e != nil || json.Unmarshal([]byte(raw), &s) != nil || s.Menu != 0 || s.Telemetry == 0 {
				stop()
				continue
			}
			if cancel != nil || time.Now().Before(retry) {
				continue
			}
			exe, _ := os.Executable()
			child, stopChild := context.WithCancel(ctx)
			cmd := exec.CommandContext(child, exe, "temperature-worker")
			if e = cmd.Start(); e != nil {
				stopChild()
				retry = time.Now().Add(time.Minute)
				continue
			}
			ch := make(chan error, 1)
			go func() { ch <- cmd.Wait() }()
			done = ch
			cancel = stopChild
		}
	}
}
