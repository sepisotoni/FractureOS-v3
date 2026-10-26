package main

// Native Win32/GDI dashboard. This is an on-demand process: no embedded browser,
// WebView runtime, UI polling in the tray, or retained dashboard after closing.
import (
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
type winPaint struct {
	DC                 uintptr
	Erase              int32
	Rect               winRect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type dashboardStatus struct {
	Version       string `json:"version"`
	Profile       string `json:"profile"`
	Face          string `json:"face"`
	Theme         string `json:"theme"`
	Brightness    int    `json:"brightness"`
	Offset        int    `json:"offset"`
	Timer         int    `json:"timer_seconds"`
	CPU           int    `json:"cpu"`
	RAM           int    `json:"ram"`
	Disk          int    `json:"disk"`
	Traffic       int    `json:"traffic"`
	WeatherLive   bool   `json:"weather_live"`
	WeatherCode   int    `json:"weather_code"`
	Temperature   int    `json:"temperature_tenths"`
	Unusual       bool   `json:"weather_unusual"`
	Telemetry     int    `json:"telemetry"`
	Weather       int    `json:"weather"`
	Notifications int    `json:"notifications"`
	Media         int    `json:"media_enabled"`
	Appearance    int    `json:"appearance"`
}

func platformDashboard() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32.NewProc("SetProcessDPIAware").Call()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	color := func(r, g, b uintptr) uintptr { return r | g<<8 | b<<16 }
	bg := color(9, 13, 20)
	surface := color(22, 31, 44)
	foreground := color(232, 240, 250)
	muted := color(154, 173, 193)
	accent := color(98, 209, 248)
	brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(bg)
	defer gdi32.NewProc("DeleteObject").Call(brush)
	cardBrush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(surface)
	defer gdi32.NewProc("DeleteObject").Call(cardBrush)
	font := func(height, weight int32) uintptr {
		v, _, _ := gdi32.NewProc("CreateFontW").Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(wide("Segoe UI"))))
		return v
	}
	normal := font(-18, 400)
	titleFont := font(-32, 600)
	clockFont := font(-66, 600)
	defer gdi32.NewProc("DeleteObject").Call(normal)
	defer gdi32.NewProc("DeleteObject").Call(titleFont)
	defer gdi32.NewProc("DeleteObject").Call(clockFont)
	var hwnd uintptr
	var children []uintptr
	fields := map[int]uintptr{}
	pageIndex := 0
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
			mu.Lock()
			state = next
			lastSuccess = time.Now()
			mu.Unlock()
			return "Connected to FractureOS " + next.Version, nil
		})
	}
	control := func(class, label string, id, x, y, w, h int, style uintptr) uintptr {
		v, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(wide(class))), uintptr(unsafe.Pointer(wide(label))), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, uintptr(id), instance, 0)
		user32.NewProc("SendMessageW").Call(v, 0x30, normal, 1)
		children = append(children, v)
		fields[id] = v
		return v
	}
	label := func(s string, x, y int) { control("STATIC", s, 0, x, y, 620, 25, 0) }
	edit := func(id int, value string, x, y, w int, password bool) {
		style := uintptr(0x10000 | 0x800000 | 0x80)
		if password {
			style |= 0x20
		}
		control("EDIT", value, id, x, y, w, 30, style)
	}
	button := func(id int, s string, x, y, w int) { control("BUTTON", s, id, x, y, w, 34, 0x10000) }
	check := func(id int, s string, on bool, x, y int) {
		v := control("BUTTON", s, id, x, y, 700, 28, 0x10000|3)
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
		user32.NewProc("GetWindowTextW").Call(v, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
		return syscall.UTF16ToString(buf)
	}
	checked := func(id int) bool {
		v, _, _ := user32.NewProc("SendMessageW").Call(fields[id], 0xf0, 0, 0)
		return v == 1
	}
	var buildPage func()
	buildPage = func() {
		for _, v := range children {
			user32.NewProc("DestroyWindow").Call(v)
		}
		children = nil
		fields = map[int]uintptr{}
		for i, s := range []string{"Dashboard", "Clock & faces", "Connection", "Weather", "Notifications"} {
			button(100+i, s, 20+i*176, 76, 168)
		}
		mu.Lock()
		st := state
		q := integrations
		mu.Unlock()
		switch pageIndex {
		case 0:
			button(201, "Minimal", 36, 410, 126)
			button(202, "Desk", 172, 410, 126)
			button(203, "Bedside", 308, 410, 126)
			label("Timer (minutes)", 36, 470)
			edit(210, "25", 36, 499, 95, false)
			button(211, "Start", 143, 497, 95)
			button(212, "Cancel", 250, 497, 95)
			button(213, "Refresh", 36, 565, 130)
			button(214, "Web dashboard", 180, 565, 180)
		case 1:
			label("Face", 36, 140)
			combo(220, []string{"nunito", "neon", "blue-outline", "weather-blue", "aqua", "sunset", "numerals-duo", "lavender", "roboto", "ubuntu"}, st.Face, 36, 172, 250)
			label("Theme", 340, 140)
			combo(221, []string{"midnight", "teal", "amber", "blue"}, st.Theme, 340, 172, 210)
			label("Appearance", 590, 140)
			appearance := []string{"Night", "Day", "Auto (07:00-19:00)"}
			a := st.Appearance
			if a < 0 || a > 2 {
				a = 0
			}
			combo(222, appearance, appearance[a], 590, 172, 270)
			label("Brightness (1-100)", 36, 230)
			edit(223, strconv.Itoa(st.Brightness), 36, 260, 180, false)
			label("UTC offset (minutes)", 340, 230)
			edit(224, strconv.Itoa(st.Offset), 340, 260, 210, false)
			check(225, "Telemetry on the TomTom", st.Telemetry != 0, 36, 330)
			check(226, "Media on the TomTom", st.Media != 0, 36, 368)
			button(227, "Apply to TomTom", 36, 430, 220)
			label("Preview faces directly: TomTom Settings > Faces. Hold Power to cancel.", 36, 490)
		case 2:
			label("Hub URL", 36, 130)
			edit(230, c.Hub, 36, 160, 824, false)
			label("Certificate fingerprint (SHA-256)", 36, 204)
			edit(231, c.Fingerprint, 36, 234, 824, false)
			label("Access token (blank keeps saved token)", 36, 278)
			edit(232, "", 36, 308, 824, true)
			check(233, "Share Spotify from this PC", c.MediaEnabled, 36, 365)
			check(234, "Show Spotify in the clock status bar", c.MediaStatus, 36, 403)
			button(235, "Save connection", 36, 462, 220)
			button(214, "Advanced web settings", 280, 462, 240)
			label("Your Linux hub keeps running when this window closes.", 36, 530)
		case 3:
			check(240, "Enable weather on the Linux hub", q.Weather.Enabled, 36, 135)
			label("Latitude", 36, 180)
			edit(241, fmt.Sprint(q.Weather.Latitude), 36, 210, 230, false)
			label("Longitude", 340, 180)
			edit(242, fmt.Sprint(q.Weather.Longitude), 340, 210, 230, false)
			label("Hot (C)", 36, 260)
			edit(243, fmt.Sprint(q.Weather.Hot), 36, 290, 180, false)
			label("Cold (C)", 340, 260)
			edit(244, fmt.Sprint(q.Weather.Cold), 340, 290, 180, false)
			label("Strong gusts (km/h)", 590, 260)
			edit(245, fmt.Sprint(q.Weather.Wind), 590, 290, 220, false)
			label("Show weather on the TomTom", 36, 345)
			mode := []string{"Off", "Unusual conditions only", "Always"}
			a := st.Weather
			if a < 0 || a > 2 {
				a = 1
			}
			combo(246, mode, mode[a], 36, 375, 400)
			button(247, "Save weather", 36, 435, 220)
			label("Thunderstorms, heavy rain/snow and your thresholds count as unusual.", 36, 494)
			label("Weather data: Open-Meteo.com (CC BY 4.0). Not official emergency alerts.", 36, 528)
			label("Initial location: Johannesburg. Change the coordinates here for your location.", 36, 562)
		case 4:
			check(250, "Enable authenticated notification webhook", q.Notifications.Enabled, 36, 140)
			label("Excluded applications (one exact app name per line)", 36, 190)
			control("EDIT", strings.Join(q.Notifications.Excluded, "\r\n"), 251, 36, 225, 824, 170, 0x10000|0x800000|0x200000|4|0x40|0x1000)
			check(252, "Allow notifications on the TomTom", st.Notifications != 0, 36, 420)
			button(253, "Save notifications", 36, 470, 230)
			label("Forward using fracture notify APP TITLE BODY, or the authenticated API.", 36, 530)
			label("Silent. Automatic Windows toast capture is not included in this build.", 36, 564)
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
		case 0x111:
			id := int(wp & 0xffff)
			if wp>>16 != 0 {
				return 0
			}
			if id >= 100 && id <= 104 {
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
				commands := []string{"face " + get(220), "theme " + get(221), "brightness " + get(223), "offset " + get(224), fmt.Sprintf("feature appearance %d", appearance), fmt.Sprintf("feature telemetry %d", bit(checked(225))), fmt.Sprintf("feature media %d", bit(checked(226)))}
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
					return "Clock updated", nil
				})
			case 235:
				next := c
				next.Hub = get(230)
				next.Fingerprint = strings.TrimSpace(get(231))
				if v := get(232); v != "" {
					next.Token = v
				}
				next.MediaEnabled = checked(233)
				next.MediaStatus = checked(234)
				if e := saveConfig(next); e != nil {
					mu.Lock()
					message = e.Error()
					mu.Unlock()
				} else {
					c = next
					user32.NewProc("SetWindowTextW").Call(fields[232], uintptr(unsafe.Pointer(wide(""))))
					mu.Lock()
					message = "Connection saved"
					mu.Unlock()
				}
				invalidate()
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
				bit := 0
				if checked(252) {
					bit = 1
				}
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
					return "Notification settings saved", nil
				})
			}
			return 0
		case 0x113:
			if minimized, _, _ := user32.NewProc("IsIconic").Call(wnd); minimized == 0 && pageIndex == 0 {
				refresh()
			}
			return 0
		case 0x8002:
			invalidate()
			return 0
		case 0x135, 0x138, 0x133:
			gdi32.NewProc("SetTextColor").Call(wp, foreground)
			gdi32.NewProc("SetBkColor").Call(wp, bg)
			return brush
		case 0x14:
			return 1
		case 0xf:
			var ps winPaint
			dc, _, _ := user32.NewProc("BeginPaint").Call(wnd, uintptr(unsafe.Pointer(&ps)))
			var client winRect
			user32.NewProc("GetClientRect").Call(wnd, uintptr(unsafe.Pointer(&client)))
			user32.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&client)), brush)
			draw := func(s string, x, y, w, h int, f, col uintptr) {
				old, _, _ := gdi32.NewProc("SelectObject").Call(dc, f)
				gdi32.NewProc("SetTextColor").Call(dc, col)
				gdi32.NewProc("SetBkMode").Call(dc, 1)
				r := winRect{int32(x), int32(y), int32(x + w), int32(y + h)}
				user32.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(wide(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x800|0x20)
				gdi32.NewProc("SelectObject").Call(dc, old)
			}
			card := func(x, y, w, h int) {
				old, _, _ := gdi32.NewProc("SelectObject").Call(dc, cardBrush)
				pen, _, _ := gdi32.NewProc("GetStockObject").Call(8)
				oldPen, _, _ := gdi32.NewProc("SelectObject").Call(dc, pen)
				gdi32.NewProc("RoundRect").Call(dc, uintptr(x), uintptr(y), uintptr(x+w), uintptr(y+h), 24, 24)
				gdi32.NewProc("SelectObject").Call(dc, old)
				gdi32.NewProc("SelectObject").Call(dc, oldPen)
			}
			draw("FractureOS", 25, 20, 450, 48, titleFont, foreground)
			draw("Your desk, at a glance", 575, 31, 300, 28, normal, muted)
			mu.Lock()
			st := state
			note := message
			last := lastSuccess
			mu.Unlock()
			if pageIndex == 0 {
				card(25, 135, 420, 247)
				card(465, 135, 410, 247)
				now := time.Now().UTC().Add(time.Duration(st.Offset) * time.Minute)
				draw(now.Format("15:04"), 46, 154, 370, 86, clockFont, foreground)
				draw(now.Format("Monday, 02 January"), 49, 247, 360, 30, normal, muted)
				summary := st.Profile + "  /  " + st.Face
				if last.IsZero() || time.Since(last) > 10*time.Second {
					summary = "TomTom offline — waiting for connection"
				}
				draw(summary, 49, 295, 360, 30, normal, accent)
				value := func(n int, unit string) string {
					if last.IsZero() || time.Since(last) > 10*time.Second || n < 0 {
						return "Unavailable"
					}
					return fmt.Sprintf("%d%s", n, unit)
				}
				draw("Hub resources", 488, 153, 350, 40, titleFont, foreground)
				draw("CPU   "+value(st.CPU, "%")+"      RAM   "+value(st.RAM, "%"), 488, 212, 365, 30, normal, accent)
				draw("Disk   "+value(st.Disk, "%")+"      Traffic   "+value(st.Traffic, " KiB/s"), 488, 253, 365, 30, normal, muted)
				weather := "Weather hidden or unavailable"
				if st.WeatherLive && time.Since(last) < 10*time.Second {
					weather = fmt.Sprintf("%.1f C  ·  %s", float64(st.Temperature)/10, weatherDescription(st.WeatherCode))
					if st.Unusual {
						weather += "  !"
					}
				}
				draw(weather, 488, 302, 365, 50, normal, accent)
				draw(fmt.Sprintf("Countdown: %d:%02d", st.Timer/60, st.Timer%60), 490, 417, 350, 36, normal, foreground)
			}
			draw(note, 25, 640, 850, 44, normal, muted)
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
	wc := wndclass{Proc: callback, Instance: instance, Cursor: cursor, Background: brush, Name: name}
	if atom, _, e := user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return e
	}
	var e error
	hwnd, _, e = user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(wide("FractureOS — Dashboard"))), 0x02ca0000, 0x80000000, 0x80000000, 922, 740, 0, 0, instance, 0)
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
	user32.NewProc("SetTimer").Call(hwnd, 1, 3000, 0)
	async(func() (string, error) {
		q, e := integrationGet(c)
		if e != nil {
			return "", e
		}
		mu.Lock()
		integrations = q
		mu.Unlock()
		out, e := hubCall(c, "status")
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
