package main

// Native Win32/GDI dashboard. This is an on-demand process: no embedded browser,
// WebView runtime, UI polling in the tray, or retained dashboard after closing.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var gdi32 = syscall.NewLazyDLL("gdi32.dll")

type winRect struct{ Left, Top, Right, Bottom int32 }
type winPoint struct{ X, Y int32 }
type winMinMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize winPoint
}
type winDrawItem struct {
	Type, ID, Item, Action, State uint32
	Window, DC                    uintptr
	Rect                          winRect
	Data                          uintptr
}
type winPaint struct {
	DC                 uintptr
	Erase              int32
	Rect               winRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type dashboardStatus struct {
	Menu           int    `json:"menu"`
	NoticeSeconds  int    `json:"notification_seconds"`
	MenuTimeout    int    `json:"menu_timeout"`
	TemperaturePC  bool   `json:"cpu_temp_pc"`
	TemperatureCPU int    `json:"cpu_temp_tenths"`
	Version        string `json:"version"`
	Profile        string `json:"profile"`
	Face           string `json:"face"`
	Theme          string `json:"theme"`
	Brightness     int    `json:"brightness"`
	Offset         int    `json:"offset"`
	Timer          int    `json:"timer_seconds"`
	CPU            int    `json:"cpu"`
	RAM            int    `json:"ram"`
	Disk           int    `json:"disk"`
	Traffic        int    `json:"traffic"`
	WeatherLive    bool   `json:"weather_live"`
	WeatherCode    int    `json:"weather_code"`
	Temperature    int    `json:"temperature_tenths"`
	Unusual        bool   `json:"weather_unusual"`
	Telemetry      int    `json:"telemetry"`
	Weather        int    `json:"weather"`
	Notifications  int    `json:"notifications"`
	Media          int    `json:"media_enabled"`
	Appearance     int    `json:"appearance"`
}

func platformDashboard() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32.NewProc("SetProcessDPIAware").Call()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	brandIcon := appIcon(instance)
	color := func(r, g, b uintptr) uintptr { return r | g<<8 | b<<16 }
	bg := color(9, 13, 20)
	surface := color(20, 28, 40)
	raised := color(29, 40, 56)
	inputBg := color(14, 20, 30)
	primaryBg := color(23, 56, 78)
	borderCol := color(48, 64, 86)
	foreground := color(235, 242, 250)
	muted := color(152, 171, 191)
	accent := color(98, 209, 248)
	brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(bg)
	defer gdi32.NewProc("DeleteObject").Call(brush)
	cardBrush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(surface)
	defer gdi32.NewProc("DeleteObject").Call(cardBrush)
	raisedBrush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(raised)
	defer gdi32.NewProc("DeleteObject").Call(raisedBrush)
	inputBrush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(inputBg)
	defer gdi32.NewProc("DeleteObject").Call(inputBrush)
	primaryBrush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(primaryBg)
	defer gdi32.NewProc("DeleteObject").Call(primaryBrush)
	borderPen, _, _ := gdi32.NewProc("CreatePen").Call(0, 1, borderCol)
	defer gdi32.NewProc("DeleteObject").Call(borderPen)
	accentPen, _, _ := gdi32.NewProc("CreatePen").Call(0, 1, accent)
	defer gdi32.NewProc("DeleteObject").Call(accentPen)
	font := func(height, weight int32) uintptr {
		v, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
		return v
	}
	normal := font(-17, 400)
	navFont := font(-14, 600)
	sectionFont := font(-22, 600)
	titleFont := font(-30, 600)
	clockFont := font(-66, 600)
	defer gdi32.NewProc("DeleteObject").Call(normal)
	defer gdi32.NewProc("DeleteObject").Call(navFont)
	defer gdi32.NewProc("DeleteObject").Call(sectionFont)
	defer gdi32.NewProc("DeleteObject").Call(titleFont)
	defer gdi32.NewProc("DeleteObject").Call(clockFont)
	var hwnd uintptr
	var children []uintptr
	childBounds := map[uintptr]dashboardBounds{}
	inCard := map[uintptr]bool{}
	fields := map[int]uintptr{}
	pageIndex := 0
	var renderer homeRenderer
	defer renderer.close()
	var preview homePreviewPacket
	previewVisible := true
	previewGeneration := 0
	timerTicks := 0
	c, err := readConfig()
	if err != nil {
		return err
	}
	integrations := IntegrationSettings{Weather: defaultWeather()}
	var state dashboardStatus
	var mu sync.Mutex
	busy := false
	alive := true
	message := "Connecting to your hub..."
	lastSuccess := time.Time{}
	invalidate := func() { user32.NewProc("InvalidateRect").Call(hwnd, 0, 0) }
	async := func(job func() (string, error)) {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		busy = true
		mu.Unlock()
		go func() {
			result, e := job()
			mu.Lock()
			if e != nil {
				message = e.Error()
			} else if result != "" {
				message = result
			}
			busy = false
			ok := alive
			mu.Unlock()
			if ok {
				user32.NewProc("PostMessageW").Call(hwnd, 0x8002, 0, 0)
			}
		}()
	}
	refresh := func() {
		mu.Lock()
		wantPreview := previewVisible
		generation := previewGeneration
		previous := preview
		mu.Unlock()
		async(func() (string, error) {
			conf, e := readConfig()
			if e != nil {
				return "", e
			}
			out, e := hubCall(conf, "status")
			if e != nil {
				return "", e
			}
			var next dashboardStatus
			if e = json.Unmarshal([]byte(out), &next); e != nil {
				return "", e
			}
			var packet homePreviewPacket
			if wantPreview {
				packet, e = fetchHomePreview(conf, previous)
			}
			mu.Lock()
			if wantPreview && previewVisible && generation == previewGeneration && e == nil {
				preview = packet
			}
			state = next
			lastSuccess = time.Now()
			mu.Unlock()
			if e != nil {
				return "", e
			}
			return "Connected to FractureOS " + next.Version, nil
		})
	}
	control := func(class, label string, id, x, y, w, h int, style uintptr) uintptr {
		v, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(label))), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, uintptr(id), instance, 0)
		controlFont := normal
		if id >= 100 && id <= 105 {
			controlFont = navFont
		}
		user32.NewProc("SendMessageW").Call(v, 0x30, controlFont, 1)
		children = append(children, v)
		fields[id] = v
		childBounds[v] = dashboardBounds{x: x, y: y, width: w, height: h}
		if id < 100 || id > 105 {
			inCard[v] = true
		}
		return v
	}
	layoutChildren := func(clientWidth int) {
		for i, v := range children {
			bounds := childBounds[v]
			if i < 6 {
				bounds = dashboardTabBounds(i, clientWidth)
			} else {
				bounds = dashboardControlBounds(bounds.x, bounds.y, bounds.width, bounds.height, clientWidth)
			}
			user32.NewProc("MoveWindow").Call(v, uintptr(bounds.x), uintptr(bounds.y), uintptr(bounds.width), uintptr(bounds.height), 1)
		}
	}
	label := func(s string, x, y int) { control("STATIC", s, 0, x, y, 390, 24, 0) }
	labelWide := func(s string, x, y, w int) { control("STATIC", s, 0, x, y, w, 24, 0) }
	edit := func(id int, value string, x, y, w int, password bool) {
		style := uintptr(0x10000 | 0x800000 | 0x80)
		if password {
			style |= 0x20
		}
		control("EDIT", value, id, x, y, w, 30, style)
	}
	button := func(id int, s string, x, y, w int) { control("BUTTON", s, id, x, y, w, 34, 0x10000|0xb) }
	check := func(id int, s string, on bool, x, y, w int) {
		v := control("BUTTON", s, id, x, y, w, 26, 0x10000|3)
		if on {
			user32.NewProc("SendMessageW").Call(v, 0xf1, 1, 0)
		}
	}
	combo := func(id int, values []string, current string, x, y, w int) {
		v := control("COMBOBOX", "", id, x, y, w, 260, 0x10000|0x200000|3)
		sel := 0
		for i, s := range values {
			user32.NewProc("SendMessageW").Call(v, 0x143, 0, uintptr(unsafe.Pointer(wide(s))))
			if s == current {
				sel = i
			}
		}
		user32.NewProc("SendMessageW").Call(v, 0x14e, uintptr(sel), 0)
	}
	get := func(id int) string {
		v := fields[id]
		size, _, _ := user32.NewProc("GetWindowTextLengthW").Call(v)
		if size > 4096 {
			size = 4096
		}
		buf := make([]uint16, size+1)
		user32.NewProc("GetWindowTextW").Call(v, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		return syscall.UTF16ToString(buf)
	}
	checked := func(id int) bool {
		v, _, _ := user32.NewProc("SendMessageW").Call(fields[id], 0xf0, 0, 0)
		return v == 1
	}
	var buildPage func()
	buildPage = func() {
		mu.Lock()
		previewVisible = pageIndex == 0
		previewGeneration++
		if !previewVisible {
			preview = homePreviewPacket{}
		}
		mu.Unlock()
		if pageIndex != 0 {
			renderer.close()
		}

		for _, v := range children {
			user32.NewProc("DestroyWindow").Call(v)
		}
		children = nil
		childBounds = map[uintptr]dashboardBounds{}
		inCard = map[uintptr]bool{}
		fields = map[int]uintptr{}
		for i, s := range []string{"Dashboard", "Clock & faces", "Connection", "Weather", "Notifications", "Updates"} {
			button(100+i, s, 20+i*146, 76, 138)
		}
		mu.Lock()
		st := state
		q := integrations
		mu.Unlock()
		switch pageIndex {
		case 0:
			button(201, "Minimal", 42, 428, 120)
			button(202, "Desk", 172, 428, 120)
			button(203, "Bedside", 302, 428, 120)
			button(213, "Refresh", 42, 476, 184)
			button(214, "Web dashboard", 238, 476, 184)
			edit(210, "25", 482, 486, 86, false)
			button(211, "Start timer", 580, 484, 134)
			button(212, "Cancel", 726, 484, 134)
		case 1:
			label("Face", 42, 144)
			combo(220, []string{"nunito", "neon", "blue-outline", "weather-blue", "aqua", "sunset", "numerals-duo", "lavender", "roboto", "ubuntu"}, st.Face, 42, 172, 250)
			label("Theme", 322, 144)
			combo(221, []string{"midnight", "teal", "amber", "blue"}, st.Theme, 322, 172, 230)
			label("Appearance", 582, 144)
			appearance := []string{"Night", "Day", "Auto (07:00-19:00)"}
			a := st.Appearance
			if a < 0 || a > 2 {
				a = 0
			}
			combo(222, appearance, appearance[a], 582, 172, 280)
			label("Brightness (1-100)", 42, 224)
			edit(223, strconv.Itoa(st.Brightness), 42, 252, 200, false)
			label("UTC offset (minutes)", 322, 224)
			edit(224, strconv.Itoa(st.Offset), 322, 252, 230, false)
			label("Menu timeout (s)", 582, 224)
			edit(228, strconv.Itoa(st.MenuTimeout), 582, 252, 132, false)
			label("Notify (s)", 730, 224)
			edit(229, strconv.Itoa(st.NoticeSeconds), 730, 252, 132, false)
			check(225, "Show telemetry on the TomTom", st.Telemetry != 0, 42, 316, 380)
			check(226, "Enable media controls on the TomTom", st.Media != 0, 42, 348, 380)
			check(262, "Look up lyrics when the Lyrics page is open", q.LyricsEnabled == nil || *q.LyricsEnabled, 452, 316, 410)
			button(227, "Apply to TomTom", 42, 384, 220)
			wpOpts := []string{"PC wallpaper (auto)", "Legacy black (#000000)", "Preset: 01 Purple Blue", "Preset: 02 Midnight Sweep", "Preset: 03 Dual Current", "Custom (web studio)"}
			wpCur := wpOpts[0]
			switch c.Wallpaper.Mode {
			case "color":
				wpCur = wpOpts[1]
			case "preset":
				switch c.Wallpaper.Preset {
				case "02-midnight-sweep.jpg":
					wpCur = wpOpts[3]
				case "03-dual-current.jpg":
					wpCur = wpOpts[4]
				default:
					wpCur = wpOpts[2]
				}
			case "custom":
				wpCur = wpOpts[5]
			}
			label("Wallpaper", 42, 434)
			combo(2201, wpOpts, wpCur, 42, 460, 260)
			button(2202, "Apply wallpaper", 316, 458, 175)
			button(2203, "Custom / Crop studio...", 505, 458, 205)
			labelWide("Preview faces and wallpapers live on the Dashboard tab.", 42, 504, 820)
		case 2:
			modeLabel := "Remote Linux hub"
			if effectiveConnectionMode(c) == "direct" {
				modeLabel = "Direct USB to this PC"
			}
			label("Connection mode", 42, 144)
			combo(290, []string{"Direct USB to this PC", "Remote Linux hub"}, modeLabel, 42, 170, 256)
			label("TomTom USB IP", 316, 144)
			edit(291, c.Device, 316, 170, 204, false)
			label("Hub URL", 538, 144)
			edit(230, c.Hub, 538, 170, 326, false)
			label("Certificate fingerprint (SHA-256)", 42, 212)
			edit(231, c.Fingerprint, 42, 238, 410, false)
			label("Access token (blank keeps saved token)", 472, 212)
			edit(232, "", 472, 238, 392, true)
			check(292, "Collect CPU & RAM metrics while Desk mode is visible", c.HubMetrics, 42, 282, 540)
			button(235, "Save connection", 42, 324, 210)
			button(214, "Web dashboard", 270, 324, 210)
			check(233, "Share Spotify from this PC", c.MediaEnabled, 42, 410, 390)
			check(234, "Show active track in clock bar", c.MediaStatus, 456, 410, 400)
			check(236, "Use this PC's CPU temperature (optional PawnIO driver)", c.WindowsTemperature, 42, 448, 620)
			button(237, "Get sensor modules", 42, 496, 195)
			button(238, "Install PawnIO", 252, 496, 185)
			button(239, "Check sensor", 452, 496, 185)
			button(261, "Authorize sensor", 652, 496, 212)
		case 3:
			check(240, "Enable weather on the host hub", q.Weather.Enabled, 42, 142, 480)
			label("Latitude", 42, 182)
			edit(241, fmt.Sprint(q.Weather.Latitude), 42, 210, 240, false)
			label("Longitude", 322, 182)
			edit(242, fmt.Sprint(q.Weather.Longitude), 322, 210, 240, false)
			label("Show weather on the TomTom", 590, 182)
			mode := []string{"Off", "Unusual conditions only", "Always"}
			a := st.Weather
			if a < 0 || a > 2 {
				a = 1
			}
			combo(246, mode, mode[a], 590, 210, 274)
			label("Hot threshold (C)", 42, 260)
			edit(243, fmt.Sprint(q.Weather.Hot), 42, 288, 220, false)
			label("Cold threshold (C)", 322, 260)
			edit(244, fmt.Sprint(q.Weather.Cold), 322, 288, 220, false)
			label("Strong gusts (km/h)", 590, 260)
			edit(245, fmt.Sprint(q.Weather.Wind), 590, 288, 274, false)
			button(247, "Save weather", 42, 342, 220)
			labelWide("Thunderstorms, heavy rain/snow and your thresholds count as unusual.", 42, 420, 820)
			labelWide("Weather data: Open-Meteo.com (CC BY 4.0). Not official emergency alerts.", 42, 454, 820)
			labelWide("Initial location: Johannesburg. Change coordinates above for your location.", 42, 488, 820)
		case 4:
			quietStart, quietEnd := q.Notifications.QuietStart, q.Notifications.QuietEnd
			if quietStart == "" {
				quietStart = "22:00"
			}
			if quietEnd == "" {
				quietEnd = "07:00"
			}
			check(250, "Enable authenticated notification webhook", q.Notifications.Enabled, 42, 140, 440)
			label("Block these apps (names or Windows app IDs)", 42, 182)
			control("EDIT", strings.Join(q.Notifications.Excluded, "\r\n"), 251, 42, 210, 396, 118, 0x10000|0x800000|0x200000|4|0x40|0x1000)
			label("Allow-list apps", 464, 182)
			control("EDIT", strings.Join(q.Notifications.Allowed, "\r\n"), 284, 464, 210, 398, 118, 0x10000|0x800000|0x200000|4|0x40|0x1000)
			check(280, "Only forward apps in allow list", q.Notifications.AllowListOnly, 42, 342, 390)
			check(281, "Quiet hours (Windows local time)", q.Notifications.QuietHoursEnabled, 464, 342, 390)
			label("From", 464, 382)
			edit(282, quietStart, 520, 378, 110, false)
			label("Until", 654, 382)
			edit(283, quietEnd, 710, 378, 110, false)
			check(252, "Allow notifications on the TomTom", st.Notifications != 0, 42, 424, 380)
			label("Display seconds", 464, 424)
			edit(254, strconv.Itoa(st.NoticeSeconds), 620, 420, 120, false)
			check(255, "Forward new Windows notifications (opt-in)", c.WindowsNotifications, 42, 462, 410)
			button(253, "Save notifications", 42, 502, 210)
			button(256, "Windows access", 264, 502, 185)
			button(257, "Permission settings", 462, 502, 195)
			button(258, "Send test", 670, 502, 192)
		case 5:
			labelWide("Installed app: "+version+"  ·  TomTom: "+st.Version, 42, 476, 520)
			button(270, "Check for updates", 42, 512, 220)
			button(271, "Update FractureOS", 280, 512, 230)
			labelWide("Updates verify release SHA-256 checksums, close the app, and reopen when ready.", 42, 560, 820)
		}
		invalidate()
	}
	runCommand := func(command string) {
		async(func() (string, error) {
			conf, e := readConfig()
			if e != nil {
				return "", e
			}
			return hubCall(conf, command)
		})
	}
	callback := syscall.NewCallback(func(wnd uintptr, m uint32, wp, lp uintptr) uintptr {
		switch m {
		case 0x24:
			info := (*winMinMaxInfo)(unsafe.Pointer(lp))
			info.MinTrackSize = winPoint{X: dashboardMinimumWidth, Y: 710}
			return 0
		case 0x111:
			id := int(wp & 0xffff)
			notifyCode := wp >> 16
			if id == 290 && notifyCode == 1 {
				if get(290) == "Direct USB to this PC" {
					curHub := strings.TrimSpace(get(230))
					if curHub == "" || !strings.HasPrefix(curHub, "https://127.0.0.1:") {
						user32.NewProc("SetWindowTextW").Call(fields[230], uintptr(unsafe.Pointer(wide("https://127.0.0.1:47831"))))
					}
				}
				return 0
			}
			if notifyCode != 0 {
				return 0
			}
			if id >= 100 && id <= 105 {
				pageIndex = id - 100
				buildPage()
				return 0
			}
			switch id {
			case 201, 202, 203:
				runCommand("profile " + []string{"minimal", "desk", "bedside"}[id-201])
			case 211:
				v, e := strconv.Atoi(get(210))
				if e == nil && v > 0 && v <= 1440 {
					runCommand(fmt.Sprintf("timer %d", v*60))
				}
			case 212:
				runCommand("timer 0")
			case 213:
				refresh()
			case 214:
				exe, _ := os.Executable()
				cmd := exec.Command(exe, "web")
				if cmd.Start() == nil {
					go cmd.Wait()
				}
			case 227:
				appearance := 0
				if get(222) == "Day" {
					appearance = 1
				} else if strings.HasPrefix(get(222), "Auto") {
					appearance = 2
				}
				bit := func(b bool) int {
					if b {
						return 1
					}
					return 0
				}
				allowLyrics := checked(262)
				commands := []string{"face " + get(220), "theme " + get(221), "brightness " + get(223), "offset " + get(224), fmt.Sprintf("feature appearance %d", appearance), fmt.Sprintf("feature telemetry %d", bit(checked(225))), fmt.Sprintf("feature media %d", bit(checked(226))), "ui-timeout menu " + get(228), "ui-timeout notification " + get(229)}
				async(func() (string, error) {
					conf, e := readConfig()
					if e != nil {
						return "", e
					}
					for _, cmd := range commands {
						if _, e = hubCall(conf, cmd); e != nil {
							return "", e
						}
					}
					q, e := integrationGet(conf)
					if e != nil {
						return "", e
					}
					q.LyricsEnabled = &allowLyrics
					if e = integrationSave(conf, q); e != nil {
						return "", e
					}
					return "Clock updated", nil
				})
			case 2202:
				sel := get(2201)
				payload := map[string]any{"mode": "pc", "color": "000000"}
				switch sel {
				case "Legacy black (#000000)":
					payload["mode"] = "color"
				case "Preset: 01 Purple Blue":
					payload["mode"] = "preset"
					payload["preset"] = "01-purple-blue-ribbon-flow.jpg"
				case "Preset: 02 Midnight Sweep":
					payload["mode"] = "preset"
					payload["preset"] = "02-midnight-sweep.jpg"
				case "Preset: 03 Dual Current":
					payload["mode"] = "preset"
					payload["preset"] = "03-dual-current.jpg"
				case "Custom (web studio)":
					if c.Wallpaper.CustomID != "" {
						payload["mode"] = "custom"
						payload["custom_id"] = c.Wallpaper.CustomID
					} else {
						mu.Lock()
						message = "Open Custom / Crop studio to upload your first custom wallpaper."
						mu.Unlock()
						invalidate()
						return 0
					}
				}
				async(func() (string, error) {
					conf, e := readConfig()
					if e != nil {
						return "", e
					}
					raw, e := hubRequest(conf, "/api/wallpaper", payload)
					if e != nil {
						return "", e
					}
					if updated, e2 := readConfig(); e2 == nil {
						c = updated
					}
					var resp struct {
						Warning string `json:"warning"`
					}
					_ = json.Unmarshal([]byte(raw), &resp)
					if resp.Warning != "" {
						return resp.Warning, nil
					}
					return "Wallpaper applied to TomTom", nil
				})
			case 2203:
				exe, _ := os.Executable()
				cmd := exec.Command(exe, "web")
				if cmd.Start() == nil {
					go cmd.Wait()
				}
				mu.Lock()
				message = "Opened Web Dashboard — select Wallpaper in the sidebar to crop & upload."
				mu.Unlock()
				invalidate()
			case 235:
				next := c
				if get(290) == "Direct USB to this PC" {
					next.ConnectionMode = "direct"
				} else {
					next.ConnectionMode = "hub"
				}
				if dev := strings.TrimSpace(get(291)); dev != "" {
					next.Device = dev
				}
				next.Hub = strings.TrimSpace(get(230))
				next.Fingerprint = strings.TrimSpace(get(231))
				if v := get(232); v != "" {
					next.Token = v
				}
				next.HubMetrics = checked(292)
				next.MediaEnabled = checked(233)
				next.MediaStatus = checked(234)
				next.WindowsTemperature = checked(236)
				if next.ConnectionMode == "direct" {
					if next.Listen == "" {
						next.Listen = "127.0.0.1:47831"
					}
					if next.Hub == "" || !strings.HasPrefix(next.Hub, "https://127.0.0.1:") {
						next.Hub = "https://" + next.Listen
						user32.NewProc("SetWindowTextW").Call(fields[230], uintptr(unsafe.Pointer(wide(next.Hub))))
					}
				}
				if e := saveConfig(next); e != nil {
					mu.Lock()
					message = e.Error()
					mu.Unlock()
				} else {
					if next.ConnectionMode == "direct" && (next.Token == "" || next.Fingerprint == "") {
						_ = identity(&next)
						user32.NewProc("SetWindowTextW").Call(fields[231], uintptr(unsafe.Pointer(wide(next.Fingerprint))))
					}
					c = next
					user32.NewProc("SetWindowTextW").Call(fields[232], uintptr(unsafe.Pointer(wide(""))))
					mu.Lock()
					message = "Connection saved"
					mu.Unlock()
				}
				invalidate()
			case 237:
				async(func() (string, error) {
					e := installPawnModules()
					return "Signed sensor modules ready. Install PawnIO, then check the sensor.", e
				})
			case 238:
				_ = openURL("https://pawnio.eu/")
			case 261:
				c.WindowsTemperature = true
				user32.NewProc("SendMessageW").Call(fields[236], 0xf1, 1, 0)
				async(func() (string, error) {
					e := authorizeSensor()
					return "Approve the sensor helper in Windows, then use Check sensor. The app stays open.", e
				})
			case 270:
				async(func() (string, error) {
					offer, e := checkUpdate(context.Background())
					if e != nil {
						return "", e
					}
					if offer.Available {
						return "Available: " + offer.Version + ". Click Update FractureOS to install.", nil
					}
					return "Already up to date (" + version + ")", nil
				})
			case 271:
				async(func() (string, error) { return installUpdate(context.Background()) })
			case 239:
				if authorizedSensorRunning() {
					mu.Lock()
					message = sensorStatusText()
					mu.Unlock()
					invalidate()
					return 0
				}
				async(func() (string, error) {
					exe, _ := os.Executable()
					out, e := exec.Command(exe, "pawnio-check").CombinedOutput()
					if e != nil {
						return "", fmt.Errorf("%s", strings.TrimSpace(string(out)))
					}
					return strings.TrimSpace(string(out)), nil
				})
			case 247:
				mu.Lock()
				q := integrations
				mu.Unlock()
				q.Weather.Enabled = checked(240)
				vals := []*float64{&q.Weather.Latitude, &q.Weather.Longitude, &q.Weather.Hot, &q.Weather.Cold, &q.Weather.Wind}
				for i, dst := range vals {
					v, e := strconv.ParseFloat(get(241+i), 64)
					if e != nil {
						mu.Lock()
						message = "Enter valid weather numbers"
						mu.Unlock()
						invalidate()
						return 0
					}
					*dst = v
				}
				mode := 1
				if get(246) == "Off" {
					mode = 0
				} else if get(246) == "Always" {
					mode = 2
				}
				async(func() (string, error) {
					conf, e := readConfig()
					if e != nil {
						return "", e
					}
					if e = integrationSave(conf, q); e != nil {
						return "", e
					}
					if _, e = hubCall(conf, fmt.Sprintf("feature weather %d", mode)); e != nil {
						return "", e
					}
					mu.Lock()
					integrations = q
					mu.Unlock()
					return "Weather saved", nil
				})
			case 253:
				notificationDuration := get(254)
				mu.Lock()
				q := integrations
				mu.Unlock()
				q.Notifications.Enabled = checked(250)
				q.Notifications.Excluded = nil
				for _, v := range strings.Split(get(251), "\n") {
					if v = strings.TrimSpace(v); v != "" {
						q.Notifications.Excluded = append(q.Notifications.Excluded, v)
					}
				}
				q.Notifications.Allowed = nil
				for _, v := range strings.Split(get(284), "\n") {
					if v = strings.TrimSpace(v); v != "" {
						q.Notifications.Allowed = append(q.Notifications.Allowed, v)
					}
				}
				q.Notifications.AllowListOnly = checked(280)
				q.Notifications.QuietHoursEnabled = checked(281)
				q.Notifications.QuietStart = strings.TrimSpace(get(282))
				q.Notifications.QuietEnd = strings.TrimSpace(get(283))
				if e := validateIntegrations(q.Weather, q.Notifications); e != nil {
					mu.Lock()
					message = e.Error()
					mu.Unlock()
					invalidate()
					return 0
				}
				bit := 0
				if checked(252) {
					bit = 1
				}
				next := c
				next.WindowsNotifications = checked(255)
				if e := saveConfig(next); e != nil {
					mu.Lock()
					message = e.Error()
					mu.Unlock()
					invalidate()
					return 0
				}
				c = next
				async(func() (string, error) {
					conf, e := readConfig()
					if e != nil {
						return "", e
					}
					if e = integrationSave(conf, q); e != nil {
						return "", e
					}
					if _, e = hubCall(conf, fmt.Sprintf("feature notifications %d", bit)); e != nil {
						return "", e
					}
					mu.Lock()
					integrations = q
					mu.Unlock()
					if _, e = hubCall(conf, "ui-timeout notification "+notificationDuration); e != nil {
						return "", e
					}
					return "Notification settings saved", nil
				})
			case 256:
				async(func() (string, error) {
					e := notificationPermission()
					if e != nil {
						return "", e
					}
					return "Windows access allowed. Enable forwarding above and save notification settings.", nil
				})
			case 257:
				if e := openURL("ms-settings:privacy-notifications"); e != nil {
					mu.Lock()
					message = e.Error()
					mu.Unlock()
					invalidate()
				}
			case 258:
				async(func() (string, error) {
					if e := notificationTestForward(); e != nil {
						return "", e
					}
					recordNotificationTestDelivered()
					return "Test notification sent to the hub and TomTom", nil
				})
			}
			return 0
		case 5: // WM_SIZE: release the optional preview while minimized.
			var client winRect
			user32.NewProc("GetClientRect").Call(wnd, uintptr(unsafe.Pointer(&client)))
			if client.Right > 0 {
				layoutChildren(int(client.Right))
			}
			mu.Lock()
			wasVisible := previewVisible
			previewVisible = pageIndex == 0 && wp != 1
			if wasVisible != previewVisible {
				previewGeneration++
			}
			if !previewVisible && wasVisible {
				preview = homePreviewPacket{}
			}
			mu.Unlock()
			if wp == 1 {
				renderer.close()
			}
			return 0
		case 0x113:
			if minimized, _, _ := user32.NewProc("IsIconic").Call(wnd); minimized == 0 && pageIndex == 0 {
				timerTicks++
				if timerTicks%3 == 0 {
					refresh()
				} else {
					invalidate()
				}
			} else if minimized == 0 && pageIndex == 4 {
				timerTicks++
				if timerTicks%3 == 0 {
					invalidate()
				}
			}
			return 0
		case 0x8002:
			invalidate()
			return 0
		case 0x2b:
			item := (*winDrawItem)(unsafe.Pointer(lp))
			if item.Type != 4 {
				return 0
			}
			dc := item.DC
			r := item.Rect
			mu.Lock()
			activeProfile := state.Profile
			mu.Unlock()
			isPrimary := item.ID == uint32(100+pageIndex) || item.ID == 211 || item.ID == 227 || item.ID == 235 || item.ID == 247 || item.ID == 253 || item.ID == 271
			isProfileActive := (item.ID == 201 && activeProfile == "minimal") || (item.ID == 202 && activeProfile == "desk") || (item.ID == 203 && activeProfile == "bedside")
			fill := raisedBrush
			if isPrimary || isProfileActive || item.State&1 != 0 {
				fill = primaryBrush
			}
			pen := borderPen
			if item.ID == uint32(100+pageIndex) || isProfileActive || item.State&1 != 0 {
				pen = accentPen
			}
			outer := brush
			if item.ID < 100 || item.ID > 105 {
				outer = cardBrush
			}
			oldBrush, _, _ := gdi32.NewProc("SelectObject").Call(dc, fill)
			oldPen, _, _ := gdi32.NewProc("SelectObject").Call(dc, pen)
			user32.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), outer)
			gdi32.NewProc("RoundRect").Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), 14, 14)
			gdi32.NewProc("SelectObject").Call(dc, oldBrush)
			gdi32.NewProc("SelectObject").Call(dc, oldPen)
			oldFont, _, _ := gdi32.NewProc("SelectObject").Call(dc, normal)
			if item.ID >= 100 && item.ID <= 105 {
				gdi32.NewProc("SelectObject").Call(dc, navFont)
			}
			gdi32.NewProc("SetBkMode").Call(dc, 1)
			col := foreground
			if item.ID == uint32(100+pageIndex) || isProfileActive || item.State&1 != 0 {
				col = accent
			}
			gdi32.NewProc("SetTextColor").Call(dc, col)
			buf := make([]uint16, 128)
			user32.NewProc("GetWindowTextW").Call(item.Window, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			user32.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(&buf[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x825)
			if item.State&0x10 != 0 {
				r.Left += 4
				r.Top += 4
				r.Right -= 4
				r.Bottom -= 4
				user32.NewProc("DrawFocusRect").Call(dc, uintptr(unsafe.Pointer(&r)))
			}
			gdi32.NewProc("SelectObject").Call(dc, oldFont)
			return 1
		case 0x135, 0x138:
			gdi32.NewProc("SetTextColor").Call(wp, foreground)
			if inCard[lp] {
				gdi32.NewProc("SetBkColor").Call(wp, surface)
				return cardBrush
			}
			gdi32.NewProc("SetBkColor").Call(wp, bg)
			return brush
		case 0x133:
			gdi32.NewProc("SetTextColor").Call(wp, foreground)
			gdi32.NewProc("SetBkColor").Call(wp, inputBg)
			return inputBrush
		case 0x14:
			return 1
		case 0xf:
			var ps winPaint
			dc, _, _ := user32.NewProc("BeginPaint").Call(wnd, uintptr(unsafe.Pointer(&ps)))
			var client winRect
			user32.NewProc("GetClientRect").Call(wnd, uintptr(unsafe.Pointer(&client)))
			user32.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&client)), brush)
			clientWidth, clientHeight := int(client.Right), int(client.Bottom)
			draw := func(s string, x, y, w, h int, f, col uintptr) {
				old, _, _ := gdi32.NewProc("SelectObject").Call(dc, f)
				gdi32.NewProc("SetTextColor").Call(dc, col)
				gdi32.NewProc("SetBkMode").Call(dc, 1)
				r := winRect{int32(x), int32(y), int32(x + w), int32(y + h)}
				user32.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(wide(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x800|0x20|0x10)
				gdi32.NewProc("SelectObject").Call(dc, old)
			}
			card := func(x, y, w, h int) {
				old, _, _ := gdi32.NewProc("SelectObject").Call(dc, cardBrush)
				oldPen, _, _ := gdi32.NewProc("SelectObject").Call(dc, borderPen)
				gdi32.NewProc("RoundRect").Call(dc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 18, 18)
				gdi32.NewProc("SelectObject").Call(dc, old)
				gdi32.NewProc("SelectObject").Call(dc, oldPen)
			}
			user32.NewProc("DrawIconEx").Call(dc, 25, 22, brandIcon, 34, 34, 0, 0, 3)
			draw("FractureOS", 72, 19, 450, 44, titleFont, foreground)
			draw("Your desk, at a glance  ·  v"+version, max(72, clientWidth-330), 29, 305, 28, normal, muted)
			mu.Lock()
			st := state
			note := message
			last := lastSuccess
			mu.Unlock()
			switch pageIndex {
			case 0:
				leftCard, rightCard := dashboardCardBounds(clientWidth)
				bottomRightCard := dashboardRightBottomCardBounds(clientWidth)
				card(leftCard.x, leftCard.y, leftCard.width, leftCard.height)
				card(rightCard.x, rightCard.y, rightCard.width, rightCard.height)
				card(bottomRightCard.x, bottomRightCard.y, bottomRightCard.width, bottomRightCard.height)
				mu.Lock()
				packet := preview
				mu.Unlock()
				previewX := leftCard.x + (leftCard.width-320)/2
				if e := renderer.draw(dc, previewX, 142, packet); e != nil {
					draw(e.Error(), leftCard.x+20, 210, leftCard.width-40, 100, normal, muted)
				}
				summary := "Live home preview · rendered on this PC"
				if last.IsZero() || time.Since(last) > 10*time.Second {
					summary = "TomTom offline · last cached home preview"
				}
				draw(summary, leftCard.x+18, 394, leftCard.width-36, 24, normal, muted)
				value := func(n int, unit string) string {
					if last.IsZero() || time.Since(last) > 10*time.Second || n < 0 {
						return "Unavailable"
					}
					return fmt.Sprintf("%d%s", n, unit)
				}
				contentX := rightCard.x + 22
				contentWidth := rightCard.width - 44
				hostHeading := "Hub resources"
				hostTarget := "Remote hub · " + c.Hub
				if effectiveConnectionMode(c) == "direct" {
					hostHeading = "Direct USB host"
					hostTarget = "This PC · TomTom " + c.Device
				}
				draw(hostHeading, contentX, 140, contentWidth, 32, sectionFont, foreground)
				draw(hostTarget, contentX, 176, contentWidth, 24, navFont, muted)
				draw("CPU   "+value(st.CPU, "%")+"      RAM   "+value(st.RAM, "%"), contentX, 212, contentWidth, 28, normal, accent)
				draw(func() string {
					if st.TemperaturePC {
						return "PC temp   "
					}
					return "CPU temp   "
				}()+value(func() int {
					if st.TemperatureCPU <= -1000 {
						return -1
					}
					return st.TemperatureCPU / 10
				}(), " C"), contentX, 246, contentWidth, 28, normal, muted)
				draw("Traffic   "+value(st.Traffic, " KiB/s"), contentX, 278, contentWidth, 28, normal, muted)
				weather := "Weather hidden or unavailable"
				if st.WeatherLive && time.Since(last) < 10*time.Second {
					weather = fmt.Sprintf("%.1f C  ·  %s", float64(st.Temperature)/10, weatherDescription(st.WeatherCode))
					if st.Unusual {
						weather += "  !"
					}
				}
				draw(weather, contentX, 314, contentWidth, 38, normal, accent)
				draw("Countdown timer", bottomRightCard.x+22, bottomRightCard.y+16, bottomRightCard.width-44, 30, sectionFont, foreground)
				timerText := "No active countdown"
				if st.Timer > 0 {
					timerText = fmt.Sprintf("Remaining: %d:%02d", st.Timer/60, st.Timer%60)
				}
				draw(timerText, bottomRightCard.x+22, bottomRightCard.y+54, bottomRightCard.width-44, 28, normal, accent)
			case 1:
				c1 := dashboardFullCardBounds(clientWidth, 126, 256)
				c2 := dashboardFullCardBounds(clientWidth, 396, 118)
				card(c1.x, c1.y, c1.width, c1.height)
				card(c2.x, c2.y, c2.width, c2.height)
			case 2:
				c1 := dashboardFullCardBounds(clientWidth, 126, 250)
				c2 := dashboardFullCardBounds(clientWidth, 392, 162)
				card(c1.x, c1.y, c1.width, c1.height)
				card(c2.x, c2.y, c2.width, c2.height)
			case 3:
				c1 := dashboardFullCardBounds(clientWidth, 126, 268)
				c2 := dashboardFullCardBounds(clientWidth, 406, 124)
				card(c1.x, c1.y, c1.width, c1.height)
				card(c2.x, c2.y, c2.width, c2.height)
			case 4:
				c1 := dashboardFullCardBounds(clientWidth, 126, 434)
				card(c1.x, c1.y, c1.width, c1.height)
			case 5:
				c1 := dashboardFullCardBounds(clientWidth, 126, 320)
				c2 := dashboardFullCardBounds(clientWidth, 460, 140)
				card(c1.x, c1.y, c1.width, c1.height)
				card(c2.x, c2.y, c2.width, c2.height)
				draw("Recent changes", 42, 138, clientWidth-84, 30, sectionFont, foreground)
				y := 174
				for _, entry := range releaseNotes[:2] {
					if y > 400 {
						break
					}
					draw(entry.Version+"  ·  "+entry.Status, 42, y, clientWidth-84, 26, normal, accent)
					y += 28
					for idx, change := range entry.Changes {
						if idx >= 3 || y > 410 {
							break
						}
						draw("• "+change, 54, y, clientWidth-108, 28, navFont, foreground)
						y += 26
					}
					y += 8
				}
			}
			if pageIndex == 4 {
				draw(notificationStatus(), 25, clientHeight-80, clientWidth-50, 26, navFont, accent)
			}
			draw(note, 25, clientHeight-48, clientWidth-50, 38, normal, muted)
			user32.NewProc("EndPaint").Call(wnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		case 2:
			mu.Lock()
			alive = false
			mu.Unlock()
			user32.NewProc("KillTimer").Call(wnd, 1)
			user32.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		r, _, _ := user32.NewProc("DefWindowProcW").Call(wnd, uintptr(m), wp, lp)
		return r
	})
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	name := wide("FractureOSNativeDashboard")
	wc := wndclass{Proc: callback, Instance: instance, Icon: brandIcon, Cursor: cursor, Background: brush, Name: name}
	if atom, _, e := user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return e
	}
	var work winRect
	user32.NewProc("SystemParametersInfoW").Call(0x30, 0, uintptr(unsafe.Pointer(&work)), 0)
	x, y := int(work.Left), int(work.Top)
	if work.Right-work.Left > 922 {
		x += (int(work.Right-work.Left) - 922) / 2
	}
	if work.Bottom-work.Top > 710 {
		y += (int(work.Bottom-work.Top) - 710) / 2
	}
	var e error
	hwnd, _, e = user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(wide("FractureOS — Dashboard"))), 0x02ca0000, uintptr(x), uintptr(y), 922, 710, 0, 0, instance, 0)
	if hwnd == 0 {
		return e
	}
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if dwm.Find() == nil {
		on := int32(1)
		dwm.Call(hwnd, 20, uintptr(unsafe.Pointer(&on)), 4)
	}
	buildPage()
	user32.NewProc("ShowWindow").Call(hwnd, 1)
	user32.NewProc("UpdateWindow").Call(hwnd)
	var initialClient winRect
	user32.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&initialClient)))
	layoutChildren(int(initialClient.Right))
	user32.NewProc("SetTimer").Call(hwnd, 1, 1000, 0)
	initialConfig := c
	async(func() (string, error) {
		q, e := integrationGet(initialConfig)
		if e != nil {
			return "", e
		}
		mu.Lock()
		integrations = q
		mu.Unlock()
		out, e := hubCall(initialConfig, "status")
		if e == nil {
			var st dashboardStatus
			e = json.Unmarshal([]byte(out), &st)
			if e == nil {
				mu.Lock()
				state = st
				lastSuccess = time.Now()
				mu.Unlock()
			}
		}
		return "Connected", e
	})
	var m msg
	for {
		r, _, e := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) == -1 {
			return e
		}
		if r == 0 {
			break
		}
		handled, _, _ := user32.NewProc("IsDialogMessageW").Call(hwnd, uintptr(unsafe.Pointer(&m)))
		if handled == 0 {
			user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
			user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
		}
	}
	return nil
}
