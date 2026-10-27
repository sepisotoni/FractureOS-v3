package main

import (
	"encoding/binary"
	"unsafe"
)

func appIcon(instance uintptr) uintptr {
	icon, _, _ := user32.NewProc("LoadIconW").Call(instance, uintptr(unsafe.Pointer(wide("APP"))))
	if icon != 0 {
		return icon
	}
	if len(brandICO) > 6 {
		count := int(binary.LittleEndian.Uint16(brandICO[4:6]))
		for i := 0; i < count && 6+(i+1)*16 <= len(brandICO); i++ {
			entry := brandICO[6+i*16 : 6+(i+1)*16]
			if entry[0] != 32 {
				continue
			}
			size := int(binary.LittleEndian.Uint32(entry[8:12]))
			offset := int(binary.LittleEndian.Uint32(entry[12:16]))
			if size > 0 && offset >= 0 && offset+size <= len(brandICO) {
				icon, _, _ = user32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&brandICO[offset])), uintptr(size), 1, 0x30000, 32, 32, 0)
				if icon != 0 {
					return icon
				}
			}
		}
	}
	icon, _, _ = user32.NewProc("LoadIconW").Call(0, 32512)
	return icon
}
