package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var shell32 = syscall.NewLazyDLL("shell32.dll")

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func replaceFile(a, b string) error {
	ok, _, e := kernel32.NewProc("MoveFileExW").Call(uintptr(unsafe.Pointer(wide(a))), uintptr(unsafe.Pointer(wide(b))), 9)
	if ok == 0 {
		return e
	}
	return nil
}
func openURL(url string) error {
	r, _, _ := shell32.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(url))), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("browser launch failed (%d)", r)
	}
	return nil
}
func platformDefault() error { return platformTray() }

func deviceManagement(args []string) error {
	if len(args) != 1 || (args[0] != "install" && args[0] != "restore") {
		return errors.New("usage: fracture device install|restore")
	}
	c, e := readConfig()
	if e != nil {
		return e
	}
	exe, _ := os.Executable()
	candidates := []string{filepath.Dir(exe)}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	for _, root := range candidates {
		script := filepath.Join(root, "tools", "deploy-device.py")
		if _, err := os.Stat(script); err == nil {
			pythonExe, lookErr := exec.LookPath("python")
			var cmdArgs []string
			if lookErr == nil {
				cmdArgs = []string{script, args[0], "--host", c.Device, "--package", root}
			} else if pyExe, pyErr := exec.LookPath("py"); pyErr == nil {
				pythonExe = pyExe
				cmdArgs = []string{"-3", script, args[0], "--host", c.Device, "--package", root}
			} else {
				break
			}
			cmd := exec.Command(pythonExe, cmdArgs...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin
			return cmd.Run()
		}
	}
	return fmt.Errorf("connect the TomTom over USB (device IP %s) or deploy device payloads from a Linux host with Python 3 and tools/deploy-device.py", c.Device)
}

type winFileTime struct{ Low, High uint32 }

func fileTimeUint64(t winFileTime) uint64 { return uint64(t.High)<<32 | uint64(t.Low) }

type winMemoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func windowsCPUSample() (uint64, uint64, error) {
	var idle, kernel, user winFileTime
	ok, _, err := kernel32.NewProc("GetSystemTimes").Call(
		uintptr(unsafe.Pointer(&idle)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if ok == 0 {
		return 0, 0, err
	}
	idleTicks := fileTimeUint64(idle)
	totalTicks := fileTimeUint64(kernel) + fileTimeUint64(user)
	return totalTicks, idleTicks, nil
}

func windowsMemoryPercent() (int, error) {
	var mem winMemoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	ok, _, err := kernel32.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&mem)))
	if ok == 0 {
		return 0, err
	}
	if mem.MemoryLoad > 100 {
		return 100, nil
	}
	return int(mem.MemoryLoad), nil
}

func metricsWorker() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	total, idle, e := windowsCPUSample()
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
			newTotal, newIdle, e := windowsCPUSample()
			if e != nil {
				continue
			}
			usage := 0
			if newTotal > total && newIdle >= idle && newIdle-idle <= newTotal-total {
				usage = int(100 * (newTotal - total - (newIdle - idle)) / (newTotal - total))
			}
			total, idle = newTotal, newIdle
			memory, e := windowsMemoryPercent()
			if e != nil {
				continue
			}
			_, _ = hubCall(c, fmt.Sprintf("stats-bundle %d,%d,-1000,-1,-1 pc", usage, memory))
		}
	}
}

func platformUninstall() error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe")); err == nil {
		return exec.Command(filepath.Join(filepath.Dir(exe), "unins000.exe")).Start()
	}
	legacyScript := filepath.Join(filepath.Dir(exe), "uninstall.ps1")
	if _, err := os.Stat(legacyScript); err == nil {
		return exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", legacyScript).Start()
	}
	script := `$exe = [IO.Path]::GetFullPath($env:FRACTURE_EXE)
$packages = @(Get-AppxPackage | Where-Object {
  $root = [IO.Path]::GetFullPath($_.InstallLocation).TrimEnd('\') + '\'
  $exe.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)
})
if ($packages.Count -ne 1) { throw 'FractureOS Store package was not found' }
Remove-AppxPackage -Package $packages[0].PackageFullName`
	cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	cmd.Env = append(os.Environ(), "FRACTURE_EXE="+exe)
	return cmd.Start()
}

type point struct{ X, Y int32 }
type msg struct {
	Wnd            uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type wndclass struct {
	Style                              uint32
	Proc                               uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
}
type notify struct {
	Size                uint32
	Wnd                 uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	Title               [64]uint16
	InfoFlags           uint32
	Guid                [16]byte
	Balloon             uintptr
}

func directHubSupervisor(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var hubCancel context.CancelFunc
	var hubDone chan error
	var activeListen, activeDevice string
	stopHub := func() {
		if hubCancel != nil {
			hubCancel()
			<-hubDone
			hubCancel = nil
			hubDone = nil
		}
	}
	defer stopHub()
	check := func() {
		c, err := readConfig()
		if err != nil || effectiveConnectionMode(c) != "direct" {
			stopHub()
			activeListen, activeDevice = "", ""
			return
		}
		if hubCancel != nil && (activeListen != c.Listen || activeDevice != c.Device) {
			stopHub()
		}
		if hubCancel != nil {
			return
		}
		if c.Listen == "" {
			c.Listen = "127.0.0.1:47831"
		}
		if c.Hub == "" || !strings.HasPrefix(c.Hub, "https://127.0.0.1:") {
			c.Hub = "https://" + c.Listen
		}
		_ = identity(&c)
		hubCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		activeListen, activeDevice = c.Listen, c.Device
		hubCancel, hubDone = cancel, done
		go func() { done <- serveWithContext(hubCtx, false) }()
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-hubDone:
			if hubCancel != nil {
				hubCancel()
			}
			hubCancel, hubDone = nil, nil
			if err != nil && ctx.Err() == nil {
				log.Printf("direct usb hub: %v", err)
			}
		case <-ticker.C:
			check()
		}
	}
}

func platformTray() error {
	// Take the single-instance mutex before starting any supervisor: a second
	// launch must not spawn workers or bind the hub before it notices it is a duplicate.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mutex, _, last := kernel32.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(wide("Local\\FractureOS-v3-tray"))))
	if mutex == 0 {
		return last
	}
	defer kernel32.NewProc("CloseHandle").Call(mutex)
	if last == syscall.Errno(183) {
		return nil
	}
	mediaContext, stopMedia := context.WithCancel(context.Background())
	mediaDone := make(chan struct{})
	audioDone := make(chan struct{})
	tempDone := make(chan struct{})
	notificationDone := make(chan struct{})
	hubDone := make(chan struct{})
	defer func() {
		stopMedia()
		<-mediaDone
		<-audioDone
		<-tempDone
		<-notificationDone
		<-hubDone
	}()
	go func() { defer close(mediaDone); localMediaSupervisor(mediaContext) }()
	// The tray owns the only Windows audio supervisor. In Direct USB mode the
	// in-process hub is started with startAudio=false (directHubSupervisor); this
	// supervisor reaches that hub over HTTPS at c.Hub, so Direct mode is covered.
	go func() { defer close(audioDone); audioSupervisor(mediaContext, nil, "windows") }()
	go func() { defer close(tempDone); windowsTemperatureSupervisor(mediaContext) }()
	go func() { defer close(notificationDone); windowsNotificationSupervisor(mediaContext) }()
	go func() { defer close(hubDone); directHubSupervisor(mediaContext) }()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	icon := appIcon(instance)
	var data notify
	notifyProc := shell32.NewProc("Shell_NotifyIconW")
	taskbar, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(wide("TaskbarCreated"))))
	settingsLaunch := func() {
		exe, e := os.Executable()
		if e == nil {
			cmd := exec.Command(exe, "dashboard")
			if cmd.Start() == nil {
				go cmd.Wait()
			}
		}
	}
	callback := syscall.NewCallback(func(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
		if uintptr(message) == taskbar {
			notifyProc.Call(0, uintptr(unsafe.Pointer(&data)))
			return 0
		}
		switch message {
		case 0x8001:
			if lparam == 0x203 {
				settingsLaunch()
				return 0
			}
			if lparam == 0x205 {
				menu, _, _ := user32.NewProc("CreatePopupMenu").Call()
				status := strings.NewReplacer("\r", " ", "\n", " ").Replace(notificationStatus())
				status = noticeText(status, 96)
				user32.NewProc("AppendMenuW").Call(menu, 0x3, 0, uintptr(unsafe.Pointer(wide("Notifications: "+status))))
				for i, label := range []string{"Dashboard & settings", "Minimal clock", "Desk clock", "Bedside clock", "Exit"} {
					user32.NewProc("AppendMenuW").Call(menu, 0, uintptr(i+1), uintptr(unsafe.Pointer(wide(label))))
				}
				var pt point
				user32.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
				user32.NewProc("SetForegroundWindow").Call(hwnd)
				choice, _, _ := user32.NewProc("TrackPopupMenu").Call(menu, 0x100, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
				user32.NewProc("DestroyMenu").Call(menu)
				switch choice {
				case 1:
					settingsLaunch()
				case 2, 3, 4:
					profile := []string{"minimal", "desk", "bedside"}[int(choice)-2]
					go func() {
						c, e := readConfig()
						if e == nil {
							_, e = hubCall(c, "profile "+profile)
						}
						if e != nil {
							user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(wide(e.Error()))), uintptr(unsafe.Pointer(wide("FractureOS"))), 0x10)
						}
					}()
				case 5:
					user32.NewProc("DestroyWindow").Call(hwnd)
				}
				return 0
			}
		case 0x10:
			user32.NewProc("DestroyWindow").Call(hwnd)
			return 0
		case 2:
			notifyProc.Call(2, uintptr(unsafe.Pointer(&data)))
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		ret, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, uintptr(message), wparam, lparam)
		return ret
	})
	name := wide("FractureOSv3Tray")
	wc := wndclass{Proc: callback, Instance: instance, Icon: icon, Name: name}
	atom, _, e := user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return e
	}
	hwnd, _, e := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(wide("FractureOS"))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return e
	}
	data = notify{Size: uint32(unsafe.Sizeof(notify{})), Wnd: hwnd, ID: 1, Flags: 7, Callback: 0x8001, Icon: icon}
	copy(data.Tip[:], syscall.StringToUTF16("FractureOS v3 - right click for controls"))
	ok, _, e := notifyProc.Call(0, uintptr(unsafe.Pointer(&data)))
	if ok == 0 {
		return e
	}
	var m msg
	for {
		r, _, e := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			return e
		}
		if r == 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}
