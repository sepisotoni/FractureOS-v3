package main

// ABI layouts come from Microsoft's Windows SDK metadata (windows-rs,
// Windows.UI.Notifications.Management and Windows.ApplicationModel).
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
	"unsafe"
)

type notificationState struct {
	Message     string `json:"message"`
	At          int64  `json:"at"`
	LastSuccess int64  `json:"last_success"`
	Sent        int    `json:"sent"`
}

func writeNotificationState(message string, sent int) {
	writeNotificationStateWithDelivery(message, sent, false)
}

func writeNotificationDelivered(message string, sent int) {
	writeNotificationStateWithDelivery(message, sent, true)
}

func recordNotificationTestDelivered() {
	var state notificationState
	if b, err := os.ReadFile(filepath.Join(configDir(), "notifications-status.json")); err == nil {
		_ = json.Unmarshal(b, &state)
	}
	writeNotificationDelivered("Test notification delivered", state.Sent)
}

func writeNotificationStateWithDelivery(message string, sent int, delivered bool) {
	if os.MkdirAll(configDir(), 0700) != nil {
		return
	}
	message = noticeText(message, 256)
	p := filepath.Join(configDir(), "notifications-status.json")
	state := notificationState{Message: message, At: time.Now().Unix(), Sent: sent}
	if existing, err := os.ReadFile(p); err == nil {
		var old notificationState
		if json.Unmarshal(existing, &old) == nil {
			state.LastSuccess = old.LastSuccess
		}
	}
	if delivered {
		state.LastSuccess = state.At
	}
	b, _ := json.Marshal(state)
	f, e := os.CreateTemp(configDir(), "notifications-status-*.tmp")
	if e != nil {
		return
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		_ = f.Close()
		return
	}
	if e = f.Close(); e == nil {
		_ = replaceFile(f.Name(), p)
	}
}

func notificationStatus() string {
	b, e := os.ReadFile(filepath.Join(configDir(), "notifications-status.json"))
	var s notificationState
	if e != nil || json.Unmarshal(b, &s) != nil {
		return "Not connected. Set up Windows notification access first."
	}
	lastSuccess := ""
	if s.LastSuccess > 0 {
		lastSuccess = " Last delivered " + time.Unix(s.LastSuccess, 0).Local().Format("2006-01-02 15:04") + "."
	} else {
		lastSuccess = " No successful delivery recorded yet."
	}
	if time.Now().Unix()-s.At > 30 {
		c, e := readConfig()
		if e == nil && !c.WindowsNotifications {
			return "Forwarding is off." + lastSuccess
		}
		return "Listener stopped. Reopen the tray or check Windows notification access." + lastSuccess
	}
	return s.Message + lastSuccess
}

func notificationListener() (uintptr, error) {
	f, e := factory("Windows.UI.Notifications.Management.UserNotificationListener", "ff6123cf-4386-4aa3-b73d-b804e5b63b23")
	if e != nil {
		return 0, fmt.Errorf("Windows notification access is unavailable. Install the MSIX package with the userNotificationListener capability: %w", e)
	}
	defer comRelease(f)
	return comGet(f, 6)
}

func notificationAccess(p uintptr) (int32, error) {
	var status int32
	e := comCall(p, 7, uintptr(unsafe.Pointer(&status)))
	return status, e
}

// A separate foreground STA window owns the permission request and pumps Windows
// messages throughout the asynchronous consent operation. It never runs in the
// background collector or asks permission merely because the tray starts.
func notificationPermission() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := winrt.NewProc("RoInitialize").Call(0)
	if int32(hr) < 0 {
		return fmt.Errorf("Windows UI initialization failed: 0x%08x", uint32(hr))
	}
	defer winrt.NewProc("RoUninitialize").Call()
	p, e := notificationListener()
	if e != nil {
		writeNotificationState(e.Error(), 0)
		return e
	}
	defer comRelease(p)
	var op, info uintptr
	defer func() {
		if info != 0 {
			_ = comCall(info, 9)
		}
		comRelease(info)
		comRelease(op)
	}()
	var result error
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	callback := syscall.NewCallback(func(w uintptr, m uint32, wp, lp uintptr) uintptr {
		switch m {
		case 0x113:
			if op == 0 {
				var err error
				op, err = comGet(p, 6)
				if err == nil {
					info, err = comQuery(op, "00000036-0000-0000-c000-000000000046")
				}
				if err != nil {
					result = err
					user32.NewProc("DestroyWindow").Call(w)
					return 0
				}
			}
			var state int32
			if err := comCall(info, 7, uintptr(unsafe.Pointer(&state))); err != nil {
				result = err
				user32.NewProc("DestroyWindow").Call(w)
				return 0
			}
			if state != 0 {
				var access int32
				if state == 1 {
					result = comCall(op, 8, uintptr(unsafe.Pointer(&access)))
				} else {
					result = errors.New("Windows notification permission request failed")
				}
				if result == nil && access != 1 {
					result = errors.New("Notification access was not allowed. Enable FractureOS in Windows Settings > Privacy > Notifications")
				}
				_ = comCall(info, 10)
				comRelease(info)
				info = 0
				comRelease(op)
				op = 0
				user32.NewProc("DestroyWindow").Call(w)
			}
			return 0
		case 0x10:
			result = errors.New("Notification permission request cancelled")
			user32.NewProc("DestroyWindow").Call(w)
			return 0
		case 2:
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		r, _, _ := user32.NewProc("DefWindowProcW").Call(w, uintptr(m), wp, lp)
		return r
	})
	name := wide("FractureOSNotificationPermission")
	wc := wndclass{Proc: callback, Instance: instance, Icon: appIcon(instance), Name: name, Background: 6}
	if a, _, err := user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc))); a == 0 {
		return err
	}
	w, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(wide("FractureOS notification access"))), 0x10c80000, 200, 200, 560, 160, 0, 0, instance, 0)
	if w == 0 {
		return err
	}
	user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide("STATIC"))), uintptr(unsafe.Pointer(wide("When forwarding is enabled, new notification text and app logos are sent to your paired hub and TomTom.\r\nThis app does not save notification contents."))), 0x50000000, 20, 20, 510, 90, w, 0, instance, 0)
	user32.NewProc("SetForegroundWindow").Call(w)
	user32.NewProc("SetTimer").Call(w, 1, 100, 0)
	var m msg
	for {
		r, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	if result != nil {
		writeNotificationState(result.Error(), 0)
	} else {
		writeNotificationState("Windows access allowed. Enable forwarding and keep the tray running.", 0)
	}
	return result
}

func readWindowsNotice(p uintptr) (windowsNotice, error) {
	var n windowsNotice
	if e := comCall(p, 8, uintptr(unsafe.Pointer(&n.ID))); e != nil {
		return n, e
	}
	_ = comCall(p, 9, uintptr(unsafe.Pointer(&n.Created)))
	a, e := comGet(p, 7)
	if e != nil {
		return n, e
	}
	defer comRelease(a)
	n.AppID = comText(a, 7)
	n.AppID = noticeText(n.AppID, 512)
	d, e := comGet(a, 8)
	if e != nil {
		return n, e
	}
	defer comRelease(d)
	n.App = noticeText(comText(d, 6), 160)
	if n.App == "" {
		n.App = noticeText(n.AppID, 160)
	}
	item, e := comGet(p, 6)
	if e != nil {
		return n, e
	}
	defer comRelease(item)
	v, e := comGet(item, 8)
	if e != nil {
		return n, e
	}
	defer comRelease(v)
	h, e := hstring("ToastGeneric")
	if e != nil {
		return n, e
	}
	defer winrt.NewProc("WindowsDeleteString").Call(h)
	var binding uintptr
	if e = comCall(v, 9, h, uintptr(unsafe.Pointer(&binding))); e != nil || binding == 0 {
		return n, errors.New("Notification has no text binding")
	}
	defer comRelease(binding)
	texts, e := comGet(binding, 11)
	if e != nil {
		return n, e
	}
	defer comRelease(texts)
	var count uint32
	if e = comCall(texts, 7, uintptr(unsafe.Pointer(&count))); e != nil {
		return n, e
	}
	if count > 16 {
		count = 16
	}
	for i := uint32(0); i < count; i++ {
		var text uintptr
		if comCall(texts, 6, uintptr(i), uintptr(unsafe.Pointer(&text))) == nil && text != 0 {
			s := comText(text, 6)
			comRelease(text)
			if i == 0 {
				n.Title = noticeText(s, 256)
			} else if s != "" {
				if n.Body != "" {
					n.Body += "\n"
				}
				n.Body = noticeText(n.Body+s, 1024)
			}
		}
	}
	n.Body = noticeText(n.Body, 1024)
	return n, nil
}

func windowsNotices(listener uintptr) ([]windowsNotice, error) {
	var op uintptr
	if e := comCall(listener, 10, 1, uintptr(unsafe.Pointer(&op))); e != nil {
		return nil, e
	}
	var vector uintptr
	if e := awaitMedia(op, 8, uintptr(unsafe.Pointer(&vector))); e != nil {
		return nil, e
	}
	defer comRelease(vector)
	var count uint32
	if e := comCall(vector, 7, uintptr(unsafe.Pointer(&count))); e != nil {
		return nil, e
	}
	start := uint32(0)
	if count > maxObservedNotices {
		start = count - maxObservedNotices
	}
	notices := make([]windowsNotice, 0, count-start)
	for i := start; i < count; i++ {
		var p uintptr
		if comCall(vector, 6, uintptr(i), uintptr(unsafe.Pointer(&p))) != nil || p == 0 {
			continue
		}
		n, e := readWindowsNotice(p)
		comRelease(p)
		if e == nil {
			notices = append(notices, n)
		}
	}
	sort.SliceStable(notices, func(i, j int) bool { return notices[i].Created < notices[j].Created })
	return notices, nil
}

// Ask the Windows shell for the registered application logo. This avoids
// hardcoded WhatsApp artwork and uses the same path for other installed apps.
// SIZE contains two integers, which is safe for both x64 and ARM64 ABI calls.
func notificationLogo(appID string) ([]byte, error) {
	if appID == "" || len(appID) > 512 {
		return nil, errors.New("App logo unavailable")
	}
	id := guid("bcc18b79-ba16-442f-80c4-8a59c30c463b") // IShellItemImageFactory
	var item uintptr
	hr, _, _ := shell32.NewProc("SHCreateItemFromParsingName").Call(uintptr(unsafe.Pointer(wide("shell:AppsFolder\\"+appID))), 0, uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&item)))
	if int32(hr) < 0 {
		return nil, errors.New("App logo is not registered with Windows")
	}
	defer comRelease(item)
	var bitmap uintptr
	if e := comCall(item, 3, 48|uintptr(48)<<32, 1|4, uintptr(unsafe.Pointer(&bitmap))); e != nil {
		return nil, e
	}
	defer gdi32.NewProc("DeleteObject").Call(bitmap)
	var info struct {
		Size                   uint32
		Width, Height          int32
		Planes, Bits           uint16
		Compression, ImageSize uint32
		X, Y                   int32
		Used, Important        uint32
	}
	info.Size = 40
	info.Width = 48
	info.Height = -48
	info.Planes = 1
	info.Bits = 32
	pixels := make([]byte, 48*48*4)
	dc, _, _ := user32.NewProc("GetDC").Call(0)
	defer user32.NewProc("ReleaseDC").Call(0, dc)
	rows, _, _ := gdi32.NewProc("GetDIBits").Call(dc, bitmap, 0, 48, uintptr(unsafe.Pointer(&pixels[0])), uintptr(unsafe.Pointer(&info)), 0)
	if rows != 48 {
		return nil, errors.New("App logo bitmap unavailable")
	}
	hasAlpha := false
	for i := 3; i < len(pixels); i += 4 {
		if pixels[i] != 0 {
			hasAlpha = true
			break
		}
	}
	im := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for i := 0; i < len(pixels); i += 4 {
		im.Pix[i] = pixels[i+2]
		im.Pix[i+1] = pixels[i+1]
		im.Pix[i+2] = pixels[i]
		if hasAlpha {
			im.Pix[i+3] = pixels[i+3]
		} else {
			im.Pix[i+3] = 255
		}
	}
	var out bytes.Buffer
	if e := png.Encode(&out, im); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}

func windowsNotificationWorker(check bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := winrt.NewProc("RoInitialize").Call(1)
	if int32(hr) < 0 {
		e := errors.New("Cannot initialize Windows notification API")
		writeNotificationState(e.Error(), 0)
		return e
	}
	defer winrt.NewProc("RoUninitialize").Call()
	p, e := notificationListener()
	if e != nil {
		writeNotificationState("Set up Windows notification access: "+e.Error(), 0)
		return e
	}
	defer comRelease(p)
	access, e := notificationAccess(p)
	if e != nil {
		writeNotificationState("Cannot check Windows notification permission: "+e.Error(), 0)
		return e
	}
	if access != 1 {
		writeNotificationState("Windows access is not allowed. Open Notifications settings and allow FractureOS, then use Set up Windows access.", 0)
		return errors.New("Windows notification access not allowed")
	}
	if check {
		writeNotificationState("Windows notification access allowed", 0)
		return nil
	}
	c, e := readConfig()
	if e != nil {
		writeNotificationState("Cannot read forwarding settings: "+e.Error(), 0)
		return e
	}
	if !c.WindowsNotifications {
		writeNotificationState("Windows access allowed. Forwarding is off.", 0)
		return nil
	}
	var queue noticeQueue
	sent := 0
	const (
		noticePollInterval          = 250 * time.Millisecond
		noticeStatusRefreshInterval = 5 * time.Second
		noticeStateWriteInterval    = 5 * time.Second
	)
	type cachedLogo struct {
		data []byte
		err  error
		at   time.Time
	}
	logoCache := make(map[string]cachedLogo)
	lookupLogo := func(appID string) ([]byte, error) {
		if appID == "" {
			return nil, errors.New("App logo unavailable")
		}
		if entry, ok := logoCache[appID]; ok && (entry.err == nil || time.Since(entry.at) < time.Minute) {
			return entry.data, entry.err
		}
		if len(logoCache) >= 64 {
			clear(logoCache)
		}
		data, err := notificationLogo(appID)
		logoCache[appID] = cachedLogo{data: data, err: err, at: time.Now()}
		return data, err
	}

	var (
		cachedSettings     IntegrationSettings
		deviceNotifEnabled bool
		lastRemoteCheck    time.Time
		lastConfigKey      string
		forceRemoteRefresh = true
		lastWrittenMessage string
		lastWrittenAt      time.Time
	)
	recordState := func(msg string, delivered bool) {
		if delivered {
			writeNotificationDelivered(msg, sent)
			lastWrittenMessage = msg
			lastWrittenAt = time.Now()
			return
		}
		if msg != lastWrittenMessage || time.Since(lastWrittenAt) >= noticeStateWriteInterval {
			writeNotificationState(msg, sent)
			lastWrittenMessage = msg
			lastWrittenAt = time.Now()
		}
	}

	for {
		c, e = readConfig()
		if e != nil {
			writeNotificationState("Cannot read forwarding settings: "+e.Error(), sent)
			return e
		}
		if !c.WindowsNotifications {
			writeNotificationState("Forwarding is off.", sent)
			return nil
		}
		if access, e = notificationAccess(p); e != nil {
			writeNotificationState("Cannot check Windows notification permission: "+e.Error(), sent)
			return e
		}
		if access != 1 {
			writeNotificationState("Windows notification access was revoked. Use Set up Windows access to allow it again.", sent)
			return errors.New("Windows notification access was revoked")
		}
		configKey := c.ConnectionMode + "|" + c.Hub + "|" + c.Device
		if forceRemoteRefresh || configKey != lastConfigKey || lastRemoteCheck.IsZero() || time.Since(lastRemoteCheck) >= noticeStatusRefreshInterval {
			settings, err := integrationGet(c)
			if err != nil {
				forceRemoteRefresh = true
				recordState("Cannot reach the hub; retrying", false)
				time.Sleep(2 * time.Second)
				continue
			}
			raw, err := hubCall(c, "status")
			var ds struct {
				Notifications int `json:"notifications"`
			}
			if err != nil || json.Unmarshal([]byte(raw), &ds) != nil {
				forceRemoteRefresh = true
				recordState("TomTom unavailable; retrying", false)
				time.Sleep(2 * time.Second)
				continue
			}
			cachedSettings = settings
			deviceNotifEnabled = ds.Notifications != 0
			lastRemoteCheck = time.Now()
			lastConfigKey = configKey
			forceRemoteRefresh = false
		}
		if !cachedSettings.Notifications.Enabled || !deviceNotifEnabled {
			queue.reset()
			forceRemoteRefresh = true
			recordState("Paused: enable the authenticated notification webhook and TomTom notifications", false)
			time.Sleep(2 * time.Second)
			continue
		}
		if quietHoursActive(cachedSettings.Notifications.QuietHoursEnabled, cachedSettings.Notifications.QuietStart, cachedSettings.Notifications.QuietEnd, time.Now()) {
			queue.reset()
			recordState("Quiet hours are active (Windows local time).", false)
			time.Sleep(2 * time.Second)
			continue
		}
		notices, e := windowsNotices(p)
		if e != nil {
			recordState("Windows notification read failed: "+e.Error(), false)
			time.Sleep(2 * time.Second)
			continue
		}
		queue.observe(notices, cachedSettings.Notifications, time.Now())
		message := fmt.Sprintf("Listening for new Windows notifications; sent %d", sent)
		delivered := false
		deliveryFailed := false
		for len(queue.pending) > 0 {
			n := queue.pending[0].notice
			var logoErr error
			n.Artwork, logoErr = lookupLogo(n.AppID)
			n.Notification.AppID = n.AppID
			out, err := hubRequest(c, "/api/notifications", n.Notification)
			if err != nil {
				message = "Notification delivery failed; retrying: " + err.Error()
				forceRemoteRefresh = true
				deliveryFailed = true
				break
			}
			if out != "OK" {
				message = "Hub rejected notification; retrying: " + out
				forceRemoteRefresh = true
				deliveryFailed = true
				break
			}
			queue.pending[0] = pendingNotice{}
			queue.pending = queue.pending[1:]
			sent++
			if logoErr != nil {
				message = fmt.Sprintf("Forwarded %s; app logo unavailable; sent %d", n.App, sent)
			} else {
				message = fmt.Sprintf("Forwarded %s; sent %d", n.App, sent)
			}
			delivered = true
		}
		recordState(message, delivered)
		if deliveryFailed {
			time.Sleep(750 * time.Millisecond)
		} else {
			time.Sleep(noticePollInterval)
		}
	}
}

func windowsNotificationSupervisor(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done chan error
	defer func() {
		if cancel != nil {
			cancel()
			<-done
		}
	}()
	retry := time.Time{}
	lastStatus := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			cancel()
			cancel = nil
			done = nil
			retry = time.Now().Add(5 * time.Second)
		case <-ticker.C:
			c, e := readConfig()
			if e != nil {
				if lastStatus != "Cannot read Windows forwarding settings." {
					writeNotificationState("Cannot read Windows forwarding settings.", 0)
					lastStatus = "Cannot read Windows forwarding settings."
				}
				continue
			}
			if !c.WindowsNotifications {
				if cancel != nil {
					cancel()
					<-done
					cancel = nil
					done = nil
				}
				retry = time.Time{}
				if lastStatus != "Forwarding is off." {
					writeNotificationState("Forwarding is off.", 0)
					lastStatus = "Forwarding is off."
				}
				continue
			}
			lastStatus = ""
			if done == nil && time.Now().After(retry) {
				exe, e := os.Executable()
				if e != nil {
					writeNotificationState("Cannot locate the Windows app to start notification forwarding: "+e.Error(), 0)
					retry = time.Now().Add(30 * time.Second)
					continue
				}
				child, stop := context.WithCancel(ctx)
				cmd := exec.CommandContext(child, exe, "notifications-worker")
				cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
				if e = cmd.Start(); e != nil {
					stop()
					writeNotificationState("Cannot start Windows notification forwarding: "+e.Error(), 0)
					retry = time.Now().Add(30 * time.Second)
					continue
				}
				cancel = stop
				done = make(chan error, 1)
				out := done
				go func() { out <- cmd.Wait() }()
			}
		}
	}
}
