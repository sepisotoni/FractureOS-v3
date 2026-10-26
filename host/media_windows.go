package main

// Windows Runtime ABI calls, confined to an optional Go worker on one MTA
// thread. uintptr values converted below are foreign COM/HSTRING buffers,
// never Go heap addresses; they remain valid until their owning COM release.
// Interface layouts follow Microsoft's Windows SDK metadata.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var winrt = syscall.NewLazyDLL("combase.dll")
var ole32 = syscall.NewLazyDLL("ole32.dll")

type mediaGUID struct {
	A    uint32
	B, C uint16
	D    [8]byte
}

func guid(s string) mediaGUID {
	var g mediaGUID
	_, e := fmt.Sscanf(s, "%08x-%04x-%04x-%02x%02x-%02x%02x%02x%02x%02x%02x", &g.A, &g.B, &g.C, &g.D[0], &g.D[1], &g.D[2], &g.D[3], &g.D[4], &g.D[5], &g.D[6], &g.D[7])
	if e != nil {
		panic("invalid internal GUID")
	}
	return g
}

//go:uintptrescapes
func comCall(p uintptr, slot int, args ...uintptr) error {
	if p == 0 {
		return errors.New("Windows media interface unavailable")
	}
	table := *(*uintptr)(unsafe.Pointer(p))
	fn := *(*uintptr)(unsafe.Pointer(table + uintptr(slot)*unsafe.Sizeof(p)))
	params := append([]uintptr{p}, args...)
	hr, _, _ := syscall.SyscallN(fn, params...)
	if int32(hr) < 0 {
		return fmt.Errorf("Windows media HRESULT 0x%08x", uint32(hr))
	}
	return nil
}
func comRelease(p uintptr) {
	if p != 0 {
		_ = comCall(p, 2)
	}
}
func comQuery(p uintptr, id string) (uintptr, error) {
	g := guid(id)
	var out uintptr
	e := comCall(p, 0, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&out)))
	return out, e
}
func comGet(p uintptr, slot int) (uintptr, error) {
	var out uintptr
	e := comCall(p, slot, uintptr(unsafe.Pointer(&out)))
	return out, e
}
func hstring(s string) (uintptr, error) {
	u, e := syscall.UTF16FromString(s)
	if e != nil {
		return 0, e
	}
	var h uintptr
	hr, _, _ := winrt.NewProc("WindowsCreateString").Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if int32(hr) < 0 {
		return 0, errors.New("Windows string allocation failed")
	}
	return h, nil
}
func comText(p uintptr, slot int) string {
	h, e := comGet(p, slot)
	if e != nil || h == 0 {
		return ""
	}
	defer winrt.NewProc("WindowsDeleteString").Call(h)
	var n uint32
	ptr, _, _ := winrt.NewProc("WindowsGetStringRawBuffer").Call(h, uintptr(unsafe.Pointer(&n)))
	if n > 4096 || ptr == 0 {
		return ""
	}
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), int(n)))
}
func factory(class, id string) (uintptr, error) {
	h, e := hstring(class)
	if e != nil {
		return 0, e
	}
	defer winrt.NewProc("WindowsDeleteString").Call(h)
	g := guid(id)
	var p uintptr
	hr, _, _ := winrt.NewProc("RoGetActivationFactory").Call(h, uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&p)))
	if int32(hr) < 0 {
		return 0, fmt.Errorf("Windows media API unavailable (0x%08x)", uint32(hr))
	}
	return p, nil
}

//go:uintptrescapes
func awaitMedia(op uintptr, resultSlot int, out uintptr) error {
	if op == 0 {
		return errors.New("no Windows async operation")
	}
	defer comRelease(op)
	info, e := comQuery(op, "00000036-0000-0000-c000-000000000046")
	if e != nil {
		return e
	}
	defer comRelease(info)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var status int32
		if e = comCall(info, 7, uintptr(unsafe.Pointer(&status))); e != nil {
			return e
		}
		if status == 1 {
			e = comCall(op, resultSlot, out)
			_ = comCall(info, 10)
			return e
		}
		if status != 0 {
			return errors.New("Windows media operation failed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = comCall(info, 9)
	return errors.New("Windows media operation timed out")
}
func mediaManager() (uintptr, error) {
	f, e := factory("Windows.Media.Control.GlobalSystemMediaTransportControlsSessionManager", "2050c4ee-11a0-57de-aed7-c97c70338245")
	if e != nil {
		return 0, e
	}
	defer comRelease(f)
	op, e := comGet(f, 6)
	if e != nil {
		return 0, e
	}
	var manager uintptr
	e = awaitMedia(op, 8, uintptr(unsafe.Pointer(&manager)))
	return manager, e
}
func spotifySession(manager uintptr) (uintptr, error) {
	current, e := comGet(manager, 6)
	if e == nil && current != 0 {
		if strings.Contains(strings.ToLower(comText(current, 6)), "spotify") {
			return current, nil
		}
		comRelease(current)
	}
	sessions, e := comGet(manager, 7)
	if e != nil {
		return 0, e
	}
	defer comRelease(sessions)
	var count uint32
	if e = comCall(sessions, 7, uintptr(unsafe.Pointer(&count))); e != nil {
		return 0, e
	}
	if count > 32 {
		count = 32
	}
	for i := uint32(0); i < count; i++ {
		var s uintptr
		if comCall(sessions, 6, uintptr(i), uintptr(unsafe.Pointer(&s))) == nil {
			if strings.Contains(strings.ToLower(comText(s, 6)), "spotify") {
				return s, nil
			}
			comRelease(s)
		}
	}
	return 0, errors.New("Open the Spotify desktop app and start playback")
}
func boolProperty(p uintptr, slot int) bool {
	var b byte
	return comCall(p, slot, uintptr(unsafe.Pointer(&b))) == nil && b != 0
}
func mediaProperties(session uintptr) (uintptr, error) {
	op, e := comGet(session, 7)
	if e != nil {
		return 0, e
	}
	var props uintptr
	e = awaitMedia(op, 8, uintptr(unsafe.Pointer(&props)))
	return props, e
}
func sessionSnapshot(session, props uintptr) mediaSnapshot {
	s := mediaSnapshot{Title: comText(props, 6), Artist: comText(props, 9), Message: "Spotify on Windows", Volume: -1}
	info, e := comGet(session, 9)
	if e == nil {
		defer comRelease(info)
		var status int32
		_ = comCall(info, 7, uintptr(unsafe.Pointer(&status)))
		s.Playing = status == 4
		controls, e := comGet(info, 6)
		if e == nil {
			defer comRelease(controls)
			s.PlayOK = boolProperty(controls, 6)
			if s.Playing {
				s.PlayOK = boolProperty(controls, 7)
			}
			s.NextOK = boolProperty(controls, 12)
			s.PreviousOK = boolProperty(controls, 13)
		}
	}
	timeline, e := comGet(session, 8)
	if e == nil {
		defer comRelease(timeline)
		var start, end, pos, updated int64
		_ = comCall(timeline, 6, uintptr(unsafe.Pointer(&start)))
		_ = comCall(timeline, 7, uintptr(unsafe.Pointer(&end)))
		_ = comCall(timeline, 10, uintptr(unsafe.Pointer(&pos)))
		_ = comCall(timeline, 11, uintptr(unsafe.Pointer(&updated)))
		if s.Playing && updated > 116444736000000000 {
			delta := time.Now().UnixMilli() - (updated-116444736000000000)/10000
			if delta > 0 && delta < 86400000 {
				pos += delta * 10000
			}
		}
		s.Duration = int((end - start) / 10000000)
		s.Position = int((pos - start) / 10000000)
		if s.Duration < 0 {
			s.Duration = 0
		}
		if s.Duration > 86400 {
			s.Duration = 86400
		}
		if s.Position < 0 {
			s.Position = 0
		}
		if s.Position > s.Duration {
			s.Position = s.Duration
		}
	}
	if volume, e := windowsVolume(-1); e == nil {
		s.Volume = volume
		s.VolumeOK = true
	}
	return s
}
func sessionAction(session uintptr, action string, value int) error {
	slot := 0
	switch action {
	case "play":
		slot = 10
	case "pause":
		slot = 11
	case "next":
		slot = 16
	case "previous":
		slot = 17
	case "volume":
		_, e := windowsVolume(value)
		return e
	default:
		return errors.New("unsupported media action")
	}
	op, e := comGet(session, slot)
	if e != nil {
		return e
	}
	var ok byte
	if e = awaitMedia(op, 8, uintptr(unsafe.Pointer(&ok))); e != nil {
		return e
	}
	if ok == 0 {
		return errors.New("Spotify declined this control")
	}
	return nil
}
func sessionArtwork(props uintptr) ([]byte, error) {
	ref, e := comGet(props, 15)
	if e != nil {
		return nil, e
	}
	defer comRelease(ref)
	op, e := comGet(ref, 6)
	if e != nil {
		return nil, e
	}
	var content uintptr
	if e = awaitMedia(op, 8, uintptr(unsafe.Pointer(&content))); e != nil {
		return nil, e
	}
	defer comRelease(content)
	stream, e := comQuery(content, "905a0fe1-bc53-11df-8c49-001e4fc686da")
	if e != nil {
		return nil, e
	}
	defer comRelease(stream)
	var size uint64
	if e = comCall(stream, 6, uintptr(unsafe.Pointer(&size))); e != nil || size < 1 || size > 2*1024*1024 {
		return nil, errors.New("unsupported album thumbnail size")
	}
	var input uintptr
	if e = comCall(stream, 8, 0, uintptr(unsafe.Pointer(&input))); e != nil {
		return nil, e
	}
	defer comRelease(input)
	f, e := factory("Windows.Storage.Streams.Buffer", "71af914d-c10f-484b-bc50-14bc623b3a27")
	if e != nil {
		return nil, e
	}
	defer comRelease(f)
	var buffer uintptr
	if e = comCall(f, 6, uintptr(size), uintptr(unsafe.Pointer(&buffer))); e != nil {
		return nil, e
	}
	defer comRelease(buffer)
	var read uintptr
	if e = comCall(input, 6, buffer, uintptr(size), 0, uintptr(unsafe.Pointer(&read))); e != nil {
		return nil, e
	}
	var result uintptr
	if e = awaitMedia(read, 10, uintptr(unsafe.Pointer(&result))); e != nil {
		return nil, e
	}
	defer comRelease(result)
	var length uint32
	if e = comCall(result, 7, uintptr(unsafe.Pointer(&length))); e != nil || uint64(length) > size {
		return nil, errors.New("invalid thumbnail length")
	}
	access, e := comQuery(result, "905a0fef-bc53-11df-8c49-001e4fc686da")
	if e != nil {
		return nil, e
	}
	defer comRelease(access)
	var ptr uintptr
	if e = comCall(access, 3, uintptr(unsafe.Pointer(&ptr))); e != nil || ptr == 0 {
		return nil, errors.New("thumbnail buffer unavailable")
	}
	return thumbnailRGB(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(length)))
}

// The volume editor deliberately targets the Windows default output, not an
// undocumented Spotify endpoint. Stepping avoids float-argument ABI differences
// between x64 and ARM64. The returned actual percentage may differ slightly.
func windowsVolume(target int) (int, error) {
	cls := guid("bcde0395-e52f-467c-8e3d-c4579291692e")
	iid := guid("a95664d2-9614-4f35-a746-de8db63617e6")
	var enumerator uintptr
	hr, _, _ := ole32.NewProc("CoCreateInstance").Call(uintptr(unsafe.Pointer(&cls)), 0, 1, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&enumerator)))
	if int32(hr) < 0 {
		return -1, errors.New("Windows volume unavailable")
	}
	defer comRelease(enumerator)
	var device uintptr
	if e := comCall(enumerator, 4, 0, 1, uintptr(unsafe.Pointer(&device))); e != nil {
		return -1, e
	}
	defer comRelease(device)
	volumeID := guid("5cdf2c82-841e-4546-9722-0cf74078229a")
	var volume uintptr
	if e := comCall(device, 3, uintptr(unsafe.Pointer(&volumeID)), 1, 0, uintptr(unsafe.Pointer(&volume))); e != nil {
		return -1, e
	}
	defer comRelease(volume)
	read := func() (int, error) {
		var n float32
		e := comCall(volume, 9, uintptr(unsafe.Pointer(&n)))
		return int(math.Round(float64(n) * 100)), e
	}
	current, e := read()
	if e != nil || target < 0 {
		return current, e
	}
	if target > 100 {
		return current, errors.New("invalid volume")
	}
	for i := 0; i < 200 && current != target; i++ {
		up := target > current
		slot := 19
		if up {
			slot = 18
		}
		if e = comCall(volume, slot, 0); e != nil {
			return current, e
		}
		next, e := read()
		if e != nil {
			return current, e
		}
		if next == current {
			return next, nil
		}
		if (up && next >= target) || (!up && next <= target) {
			return next, nil
		}
		current = next
	}
	return current, nil
}

func localMediaWorker() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := winrt.NewProc("RoInitialize").Call(1)
	if int32(hr) < 0 {
		return errors.New("Windows media initialization failed")
	}
	defer winrt.NewProc("RoUninitialize").Call()
	manager, e := mediaManager()
	if e != nil {
		return e
	}
	defer comRelease(manager)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	lastTrack := ""
	lastAction := ""
	for range ticker.C {
		c, e := readConfig()
		if e != nil || !c.MediaEnabled {
			return nil
		}
		raw, e := hubCall(c, "status")
		if e != nil {
			continue
		}
		var ds mediaDeviceStatus
		if json.Unmarshal([]byte(raw), &ds) != nil {
			continue
		}
		if !ds.View && !(c.MediaStatus && ds.Menu == 0) {
			return nil
		}
		session, e := spotifySession(manager)
		if e != nil {
			_, _ = hubRequest(c, "/api/media", mediaSnapshot{Clear: true, Message: e.Error()})
			lastTrack = ""
			continue
		}
		func() {
			defer comRelease(session)
			if ds.Pending {
				key := fmt.Sprintf("%d/%s/%d", ds.Sequence, ds.Action, ds.Argument)
				if key != lastAction {
					lastAction = key
					_, _ = hubCall(c, fmt.Sprintf("media-ack %d", ds.Sequence))
					if e = sessionAction(session, ds.Action, ds.Argument); e != nil {
						_, _ = hubCall(c, "media-text message "+asciiHex(e.Error()))
						return
					}
				}
			}
			props, err := mediaProperties(session)
			if err != nil {
				return
			}
			defer comRelease(props)
			snapshot := sessionSnapshot(session, props)
			track := snapshot.Title + "\n" + snapshot.Artist
			snapshot.ClearArt = track != lastTrack
			if ds.Menu == 12 && (track != lastTrack || ds.Art == 0) {
				snapshot.Artwork, _ = sessionArtwork(props)
			}
			if _, err = hubRequest(c, "/api/media", snapshot); err == nil {
				lastTrack = track
			}
		}()
	}
	return nil
}

func launchLocalMedia(ctx context.Context) (*exec.Cmd, context.CancelFunc, chan error, error) {
	exe, e := os.Executable()
	if e != nil {
		return nil, nil, nil, e
	}
	child, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(child, exe, "media-worker")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if e = cmd.Start(); e != nil {
		cancel()
		return nil, nil, nil, e
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return cmd, cancel, done, nil
}
func localMediaSupervisor(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	var worker *exec.Cmd
	var cancel context.CancelFunc
	var done chan error
	retry := time.Time{}
	stop := func() {
		if worker != nil {
			cancel()
			<-done
			worker = nil
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
			worker = nil
			done = nil
			retry = time.Now().Add(15 * time.Second)
		case <-ticker.C:
			c, e := readConfig()
			if e != nil || !c.MediaEnabled {
				stop()
				continue
			}
			raw, e := hubCall(c, "status")
			var ds mediaDeviceStatus
			if e != nil || json.Unmarshal([]byte(raw), &ds) != nil || (ds.Enabled != nil && *ds.Enabled == 0) || (!ds.View && !(c.MediaStatus && ds.Menu == 0)) {
				stop()
				continue
			}
			if worker == nil && time.Now().After(retry) {
				worker, cancel, done, e = launchLocalMedia(ctx)
				if e != nil {
					retry = time.Now().Add(30 * time.Second)
				}
			}
		}
	}
}
