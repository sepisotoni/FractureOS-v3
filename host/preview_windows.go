package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Loaded only by a visible home dashboard. Closing/changing tabs releases the
// frame, atlas, font, notification thumbnail and the renderer DLL itself.
type homeRenderer struct {
	dll                             *syscall.DLL
	assets, notice, render, release *syscall.Proc
	pixels                          []byte
	atlasHash, fontHash, noticeHash uint32
}

func (p *homeRenderer) close() {
	if p.dll != nil {
		_, _, _ = p.release.Call()
		_ = p.dll.Release()
	}
	*p = homeRenderer{}
}
func (p *homeRenderer) draw(dc uintptr, x, y int, packet homePreviewPacket) error {
	if packet.Model == "" {
		return errors.New("Waiting for home preview")
	}
	if p.dll == nil {
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		dll, e := syscall.LoadDLL(filepath.Join(filepath.Dir(exe), "fracture-preview.dll"))
		if e != nil {
			return errors.New("Home renderer is missing; install the complete FractureOS package")
		}
		p.dll = dll
		for name, dest := range map[string]**syscall.Proc{"FracturePreviewAssets": &p.assets, "FracturePreviewNotice": &p.notice, "FracturePreviewRender": &p.render, "FracturePreviewClose": &p.release} {
			proc, e := dll.FindProc(name)
			if e != nil {
				_ = dll.Release()
				*p = homeRenderer{}
				return e
			}
			*dest = proc
		}
		p.pixels = make([]byte, 320*240*4)
	}
	if p.atlasHash != packet.AtlasHash || p.fontHash != packet.FontHash {
		if len(packet.Atlas) != 170000 {
			return errors.New("Home atlas unavailable")
		}
		font := uintptr(0)
		if len(packet.Font) > 0 {
			font = uintptr(unsafe.Pointer(&packet.Font[0]))
		}
		ok, _, _ := p.assets.Call(uintptr(unsafe.Pointer(&packet.Atlas[0])), uintptr(len(packet.Atlas)), font, uintptr(len(packet.Font)))
		runtime.KeepAlive(packet)
		if ok == 0 {
			return errors.New("Invalid home assets")
		}
		p.atlasHash, p.fontHash = packet.AtlasHash, packet.FontHash
	}
	if p.noticeHash != packet.NoticeHash {
		data := uintptr(0)
		if len(packet.Notice) > 0 {
			data = uintptr(unsafe.Pointer(&packet.Notice[0]))
		}
		ok, _, _ := p.notice.Call(data, uintptr(len(packet.Notice)))
		runtime.KeepAlive(packet)
		if ok == 0 {
			return errors.New("Invalid notification artwork")
		}
		p.noticeHash = packet.NoticeHash
	}
	model, e := syscall.BytePtrFromString(packet.Model)
	if e != nil {
		return e
	}
	age := time.Since(packet.Received).Milliseconds()
	if age < 0 {
		age = 0
	}
	if age > 120000 {
		age = 120000
	}
	ok, _, _ := p.render.Call(uintptr(unsafe.Pointer(model)), uintptr(age), uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(len(p.pixels)))
	runtime.KeepAlive(model)
	if ok == 0 {
		return errors.New("Invalid home drawing model")
	}
	type bitmapInfo struct {
		Size                        uint32
		Width, Height               int32
		Planes, BitCount            uint16
		Compression, ImageSize      uint32
		XPels, YPels                int32
		ColorsUsed, ColorsImportant uint32
	}
	info := bitmapInfo{Size: 40, Width: 320, Height: -240, Planes: 1, BitCount: 32, ImageSize: 320 * 240 * 4}
	gdi32.NewProc("SetStretchBltMode").Call(dc, 3)
	gdi32.NewProc("StretchDIBits").Call(dc, uintptr(x), uintptr(y), 320, 240, 0, 0, 320, 240, uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, 0xcc0020)
	return nil
}
