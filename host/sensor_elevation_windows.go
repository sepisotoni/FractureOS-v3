package main

// The UI and hub bridge stay unelevated. A separately approved helper exposes
// only signed-module temperature reads to its temporary loopback receiver.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            uintptr
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          uintptr
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          uintptr
	Hotkey                            uint32
	Icon, Process                     uintptr
}

func launchSensorAdmin(endpoint, token, modules string) (uintptr, error) {
	exe, e := os.Executable()
	if e != nil {
		return 0, e
	}
	args := []string{"temperature-helper", endpoint, token, modules}
	for i := range args {
		args[i] = syscall.EscapeArg(args[i])
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x400, Verb: wide("runas"), File: wide(exe), Parameters: wide(strings.Join(args, " ")), Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	r, _, e := shell32.NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if errors.Is(e, syscall.Errno(1223)) {
			return 0, errors.New("Administrator approval was cancelled")
		}
		return 0, e
	}
	return info.Process, nil
}

type sensorSample struct {
	Temperature int       `json:"temperature"`
	Error       string    `json:"error,omitempty"`
	At          time.Time `json:"at"`
}

func writeSensorStatus(sample sensorSample) {
	data, _ := json.Marshal(sample)
	tmp := filepath.Join(configDir(), "sensor-status.new")
	if os.WriteFile(tmp, data, 0600) == nil {
		_ = replaceFile(tmp, filepath.Join(configDir(), "sensor-status.json"))
	}
}
func sensorStatusText() string {
	data, e := os.ReadFile(filepath.Join(configDir(), "sensor-status.json"))
	var s sensorSample
	if e != nil || len(data) > 4096 || json.Unmarshal(data, &s) != nil || time.Since(s.At) > time.Minute {
		return "No recent authorized reading"
	}
	if s.Error != "" {
		return s.Error
	}
	if s.Temperature < -500 {
		return "Authorized sensor is waiting for Desk mode"
	}
	return fmt.Sprintf("CPU temperature: %.1f C (authorized PawnIO)", float64(s.Temperature)/10)
}
func authorizeSensor() error {
	c, e := readConfig()
	if e != nil {
		return e
	}
	c.WindowsTemperature = true
	if e = saveConfig(c); e != nil {
		return e
	}
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	cmd := exec.Command(exe, "temperature-authorize")
	if e = cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	return nil
}
func authorizedTemperatureWorker() error {
	// A named mutex prevents an ordinary tray worker competing with this session.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	mutex, _, e := kernel32.NewProc("CreateMutexW").Call(0, 1, uintptr(unsafe.Pointer(wide(`Local\FractureOS-v3-authorized-temperature`))))
	if mutex == 0 {
		return e
	}
	defer kernel32.NewProc("CloseHandle").Call(mutex)
	if errors.Is(e, syscall.Errno(183)) {
		return errors.New("An authorized sensor session is already running")
	}
	defer kernel32.NewProc("ReleaseMutex").Call(mutex)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	token, e := randomToken()
	if e != nil {
		listener.Close()
		return e
	}
	var mu sync.Mutex
	sample := sensorSample{Temperature: -1000}
	needed := true
	keep := true
	lastContact := time.Now()
	first := true
	srv := &http.Server{ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/sensor" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		var next sensorSample
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if dec.Decode(&next) != nil || next.Temperature < -1000 || next.Temperature > 1500 || len(next.Error) > 512 {
			http.Error(w, "invalid sensor sample", 400)
			return
		}
		next.At = time.Now()
		mu.Lock()
		sample = next
		lastContact = time.Now()
		response := struct {
			Need bool `json:"need"`
			Keep bool `json:"keep"`
		}{needed, keep}
		mu.Unlock()
		writeSensorStatus(next)
		_ = json.NewEncoder(w).Encode(response)
	})}
	defer srv.Close()
	go func() { _ = srv.Serve(listener) }()
	handle, e := launchSensorAdmin("http://"+listener.Addr().String()+"/sensor", token, filepath.Join(configDir(), "pawnio"))
	if e != nil {
		writeSensorStatus(sensorSample{Temperature: -1000, At: time.Now(), Error: e.Error()})
		return e
	}
	defer kernel32.NewProc("CloseHandle").Call(handle)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	idle := time.Time{}
	defer func() {
		mu.Lock()
		keep = false
		mu.Unlock()
		kernel32.NewProc("WaitForSingleObject").Call(handle, 4000)
	}()
	for range ticker.C {
		c, e := readConfig()
		if e != nil || !c.WindowsTemperature {
			return nil
		}
		raw, err := hubCall(c, "status")
		var s dashboardStatus
		if err == nil {
			err = json.Unmarshal([]byte(raw), &s)
		}
		tray, _, _ := user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(wide("FractureOSv3Tray"))), 0)
		use := tray != 0 && err == nil && s.Telemetry != 0 && s.Profile == "desk" // A short menu visit may resume without a second UAC prompt.
		if use {
			idle = time.Time{}
		} else if idle.IsZero() {
			idle = time.Now()
		}
		if !idle.IsZero() && time.Since(idle) > 30*time.Second {
			return nil
		}
		mu.Lock()
		needed = use && s.Menu == 0
		v := sample
		contact := lastContact
		mu.Unlock()
		if v.Error != "" {
			return errors.New(v.Error)
		}
		if time.Since(contact) > 45*time.Second {
			return errors.New("Sensor helper stopped responding")
		}
		if use && time.Since(v.At) < 6*time.Second && v.Temperature >= -500 {
			_, _ = hubCall(c, fmt.Sprintf("stats-temp %d pc", v.Temperature))
		}
		if first && !v.At.IsZero() {
			first = false
			fmt.Println(sensorStatusText())
		}
		// Exit if the approved helper has closed; never automatically raise UAC again.
		if state, _, _ := kernel32.NewProc("WaitForSingleObject").Call(handle, 0); state == 0 {
			return nil
		}
	}
	return nil
}
func sensorHelper(endpoint, token, moduleDir string) error {
	u, e := http.NewRequest("POST", endpoint, nil)
	if e != nil || u.URL.Scheme != "http" || u.URL.Hostname() != "127.0.0.1" || u.URL.Path != "/sensor" || u.URL.RawQuery != "" || u.URL.User != nil {
		return errors.New("Invalid sensor receiver")
	}
	port, e := strconv.Atoi(u.URL.Port())
	if e != nil || port < 1024 || port > 65535 {
		return errors.New("Invalid receiver port")
	}
	decoded, e := hex.DecodeString(token)
	if e != nil || len(decoded) != 32 || !filepath.IsAbs(moduleDir) || len(moduleDir) > 512 {
		return errors.New("Invalid sensor session")
	}
	client := http.Client{Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("Receiver redirects refused") }}
	defer client.CloseIdleConnections()
	var sensor *pawnSensor
	defer func() {
		if sensor != nil {
			sensor.release()
		}
	}()
	need := true
	idle := time.Time{}
	start := time.Now()
	for time.Since(start) < 24*time.Hour {
		s := sensorSample{Temperature: -1000}
		if need {
			if sensor == nil {
				sensor, e = openPawnSensorAt(moduleDir)
			}
			if e == nil {
				s.Temperature, e = sensor.temperature()
			}
			if e != nil {
				s.Error = e.Error()
			}
		} else if sensor != nil {
			sensor.release()
			sensor = nil
		}
		data, _ := json.Marshal(s)
		req, err := http.NewRequestWithContext(context.Background(), "POST", endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return nil
		}
		body, readError := io.ReadAll(io.LimitReader(res.Body, 2048))
		res.Body.Close()
		if readError != nil || res.StatusCode != 200 {
			return nil
		}
		var command struct {
			Need bool `json:"need"`
			Keep bool `json:"keep"`
		}
		if json.Unmarshal(body, &command) != nil || !command.Keep || s.Error != "" {
			return nil
		}
		need = command.Need
		if need {
			idle = time.Time{}
		} else if idle.IsZero() {
			idle = time.Now()
		}
		if !idle.IsZero() && time.Since(idle) > 45*time.Second {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}
func authorizedSensorRunning() bool {
	mutex, _, _ := kernel32.NewProc("OpenMutexW").Call(0x100000, 0, uintptr(unsafe.Pointer(wide(`Local\FractureOS-v3-authorized-temperature`))))
	if mutex == 0 {
		return false
	}
	kernel32.NewProc("CloseHandle").Call(mutex)
	return true
}

var signedModuleDigests = map[string]string{"IntelMSR.bin": "d6ed85d65ab17a22f813ef98207d6d537155ee2ded5976a21cb48413c9b92e5f", "AMDFamily17.bin": "dae74615761b78bdf064dfb3e136252ddcc6fc727d88f14738d0e5800d427a91"}

func validateSensorModule(name string, data []byte) error {
	if fmt.Sprintf("%x", sha256.Sum256(data)) != signedModuleDigests[name] {
		return errors.New("Sensor module differs from the verified official version; download modules again")
	}
	return nil
}
