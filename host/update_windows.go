package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func platformInstallUpdate(offer updateOffer, installer, stage string) (string, error) {
	exe, e := os.Executable()
	if e != nil {
		return "", e
	}
	helper := filepath.Join(stage, "fracture-updater.exe")
	src, e := os.Open(exe)
	if e != nil {
		return "", e
	}
	defer src.Close()
	dst, e := os.OpenFile(helper, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if e != nil {
		return "", e
	}
	_, e = io.Copy(dst, src)
	closeError := dst.Close()
	if e != nil {
		return "", e
	}
	if closeError != nil {
		return "", closeError
	}
	destination := filepath.Dir(exe)
	if _, e = os.Stat(filepath.Join(destination, "unins000.exe")); e != nil {
		destination = filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "FractureOS")
	}
	cmd := exec.Command(helper, "apply-update", installer, destination, exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if e = cmd.Start(); e != nil {
		return "", e
	}
	go cmd.Wait()
	return "Installing " + offer.Version + ". The app will close and reopen.", nil
}
func applyUpdate(args []string) (result error) {
	if len(args) != 3 || !filepath.IsAbs(args[0]) || !filepath.IsAbs(args[1]) || !filepath.IsAbs(args[2]) {
		return fmt.Errorf("invalid updater arguments")
	}
	cfg, configError := readConfig()
	resumeTemperature := configError == nil && cfg.WindowsTemperature
	if resumeTemperature {
		cfg.WindowsTemperature = false
		if e := saveConfig(cfg); e != nil {
			return e
		}
		time.Sleep(5 * time.Second)
	}
	defer func() {
		if resumeTemperature {
			if c, e := readConfig(); e == nil {
				c.WindowsTemperature = true
				_ = saveConfig(c)
			}
		}
		if result != nil {
			for _, mode := range []string{"tray", "dashboard"} {
				if _, e := os.Stat(args[2]); e == nil {
					cmd := exec.Command(args[2], mode)
					if cmd.Start() == nil {
						go cmd.Wait()
					}
				}
			}
			user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(wide("Update failed: "+result.Error()))), uintptr(unsafe.Pointer(wide("FractureOS update"))), 0x10)
		}
	}()
	// Close only this installation's dashboard/tray windows. The helper has its
	// own filename and is outside Restart Manager's fracture.exe file filter.
	for _, name := range []string{"FractureOSNativeDashboard", "FractureOSv3Tray"} {
		for tries := 0; tries < 8; tries++ {
			window, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(wide(name))), 0)
			if window == 0 {
				break
			}
			var pid uint32
			user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
			handle, _, _ := kernel32.NewProc("OpenProcess").Call(0x1000, 0, uintptr(pid))
			if handle == 0 {
				break
			}
			buf := make([]uint16, 1024)
			n := uint32(len(buf))
			ok, _, _ := kernel32.NewProc("QueryFullProcessImageNameW").Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
			kernel32.NewProc("CloseHandle").Call(handle)
			if ok == 0 || !sameWindowsPath(syscall.UTF16ToString(buf[:n]), args[2]) {
				break
			}
			user32.NewProc("PostMessageW").Call(window, 0x10, 0, 0)
			time.Sleep(250 * time.Millisecond)
		}
	}
	setup := exec.Command(args[0], "/SILENT", "/SUPPRESSMSGBOXES", "/NORESTART", "/CLOSEAPPLICATIONS", "/DIR="+args[1])
	if output, e := setup.CombinedOutput(); e != nil {
		return fmt.Errorf("update setup failed: %w %s", e, output)
	}
	if resumeTemperature {
		if c, e := readConfig(); e == nil {
			c.WindowsTemperature = true
			_ = saveConfig(c)
		}
	}
	target := filepath.Join(args[1], "fracture.exe")
	for _, mode := range []string{"tray", "dashboard"} {
		cmd := exec.Command(target, mode)
		if e := cmd.Start(); e != nil {
			return e
		}
		go cmd.Wait()
	}
	_ = os.Remove(args[0])
	return nil
}
func sameWindowsPath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}
