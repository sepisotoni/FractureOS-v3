package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
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
func deviceManagement(_ []string) error {
	return errors.New("run fracture device install on the Linux machine physically connected to the TomTom")
}
func metricsWorker() error {
	return errors.New("Windows resource collection is not included in 0.1.9; Linux hub metrics are available")
}
func platformUninstall() error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "unins000.exe")); err == nil {
		return exec.Command(filepath.Join(filepath.Dir(exe), "unins000.exe")).Start()
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(filepath.Dir(exe), "uninstall.ps1"))
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

func platformTray() error {
	mediaContext, stopMedia := context.WithCancel(context.Background())
	mediaDone := make(chan struct{})
	defer func() { stopMedia(); <-mediaDone }()
	go func() { defer close(mediaDone); localMediaSupervisor(mediaContext) }()
	tempDone := make(chan struct{})
	go func() { defer close(tempDone); windowsTemperatureSupervisor(mediaContext) }()
	defer func() { stopMedia(); <-tempDone }()
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
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := user32.NewProc("LoadIconW").Call(0, 32512)
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
