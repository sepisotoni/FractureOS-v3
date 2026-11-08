//go:build windows

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

const (
	audioClientLoopback     = 0x00020000
	audioBufferFlagsSilence = 0x00000002
	audioFormatPCM          = 1
	audioFormatFloat        = 3
	audioFormatExtensible   = 0xfffe
)

type windowsAudioFormat struct {
	rate       int
	channels   int
	bits       int
	blockAlign int
	float      bool
}

type audioResampler struct {
	pending [][2]float64
	phase   float64
}

func launchAudioWorker(ctx context.Context) (<-chan error, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, "audio-worker")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	return done, nil
}

func localAudioWorker(ctx context.Context) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	c, err := readConfig()
	if err != nil {
		return err
	}
	settings, err := integrationGet(c)
	if err != nil {
		return err
	}
	if settings.Audio == nil || !settings.Audio.Enabled || settings.Audio.Source != "windows" {
		return nil
	}
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0)
	if int32(hr) < 0 {
		return fmt.Errorf("WASAPI initialization failed (0x%08x)", uint32(hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()

	enumerator, err := createAudioEnumerator()
	if err != nil {
		return err
	}
	defer comRelease(enumerator)
	var device uintptr
	if err = comCall(enumerator, 4, 0, 1, uintptr(unsafe.Pointer(&device))); err != nil {
		return fmt.Errorf("default Windows output unavailable: %w", err)
	}
	defer comRelease(device)
	audioClientID := guid("1cb9ad4c-dbfa-4c32-b178-c2f568a703b2")
	var client uintptr
	if err = comCall(device, 3, uintptr(unsafe.Pointer(&audioClientID)), 23, 0, uintptr(unsafe.Pointer(&client))); err != nil {
		return err
	}
	defer comRelease(client)
	var mix uintptr
	if err = comCall(client, 8, uintptr(unsafe.Pointer(&mix))); err != nil {
		return fmt.Errorf("default output format unavailable: %w", err)
	}
	defer ole32.NewProc("CoTaskMemFree").Call(mix)
	mixFormat, err := parseWindowsAudioFormat(mix)
	if err != nil {
		return err
	}

	var requested [18]byte
	binary.LittleEndian.PutUint16(requested[0:2], audioFormatPCM)
	binary.LittleEndian.PutUint16(requested[2:4], audioChannels)
	binary.LittleEndian.PutUint32(requested[4:8], audioRate)
	binary.LittleEndian.PutUint32(requested[8:12], audioRate*audioChannels*2)
	binary.LittleEndian.PutUint16(requested[12:14], audioChannels*2)
	binary.LittleEndian.PutUint16(requested[14:16], 16)
	format := mixFormat
	formatPtr := mix
	var closest uintptr
	if comCall(client, 7, 0, uintptr(unsafe.Pointer(&requested[0])), uintptr(unsafe.Pointer(&closest))) == nil && closest == 0 {
		format = windowsAudioFormat{rate: audioRate, channels: audioChannels, bits: 16, blockAlign: audioChannels * 2}
		formatPtr = uintptr(unsafe.Pointer(&requested[0]))
	}
	if closest != 0 {
		ole32.NewProc("CoTaskMemFree").Call(closest)
	}
	if err = comCall(client, 3, 0, audioClientLoopback, 200000, 0, formatPtr, 0); err != nil {
		return fmt.Errorf("WASAPI loopback could not start: %w", err)
	}
	captureID := guid("c8adbd64-e71e-48a0-a4de-185c395cd317")
	var capture uintptr
	if err = comCall(client, 14, uintptr(unsafe.Pointer(&captureID)), uintptr(unsafe.Pointer(&capture))); err != nil {
		return err
	}
	defer comRelease(capture)
	if err = comCall(client, 10); err != nil {
		return err
	}
	defer comCall(client, 11)

	pipeReader, pipeWriter := io.Pipe()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamDone := make(chan error, 1)
	go func() { streamDone <- hubAudioStream(streamCtx, c, "windows", pipeReader) }()
	defer pipeReader.Close()
	resampler := audioResampler{}
	silence := make([]byte, audioFrameBytes)
	lastOutput := time.Now()
	streamFinished := false
capture:
	for ctx.Err() == nil {
		select {
		case streamErr := <-streamDone:
			streamFinished = true
			if streamErr == nil {
				err = errors.New("audio hub stream ended")
			} else {
				err = streamErr
			}
			break capture
		default:
		}
		var packetFrames uint32
		if err = comCall(capture, 5, uintptr(unsafe.Pointer(&packetFrames))); err != nil {
			break
		}
		if packetFrames == 0 {
			if time.Since(lastOutput) >= time.Duration(audioFrameMS)*time.Millisecond {
				if _, err = pipeWriter.Write(silence); err != nil {
					break
				}
				lastOutput = time.Now()
			}
			time.Sleep(time.Millisecond)
			continue
		}
		var data uintptr
		var frames, flags uint32
		var devicePosition, qpcPosition uint64
		if err = comCall(capture, 3,
			uintptr(unsafe.Pointer(&data)),
			uintptr(unsafe.Pointer(&frames)),
			uintptr(unsafe.Pointer(&flags)),
			uintptr(unsafe.Pointer(&devicePosition)),
			uintptr(unsafe.Pointer(&qpcPosition))); err != nil {
			break
		}
		var pcm []byte
		if flags&audioBufferFlagsSilence != 0 {
			pcm, err = resampler.convert(nil, int(frames), format, true)
		} else if data == 0 || frames > 1<<20 {
			err = errors.New("invalid WASAPI audio buffer")
		} else {
			raw := unsafe.Slice((*byte)(unsafe.Pointer(data)), int(frames)*format.blockAlign)
			pcm, err = resampler.convert(raw, int(frames), format, false)
		}
		releaseErr := comCall(capture, 4, uintptr(frames))
		if err != nil {
			break
		}
		if releaseErr != nil {
			err = releaseErr
			break
		}
		if len(pcm) > 0 {
			if _, err = pipeWriter.Write(pcm); err != nil {
				break
			}
			lastOutput = time.Now()
		}
	}
	if ctx.Err() != nil {
		_ = pipeWriter.Close()
		if !streamFinished {
			<-streamDone
		}
		return nil
	}
	var streamErr error
	if !streamFinished {
		_ = pipeWriter.CloseWithError(err)
		streamErr = <-streamDone
	}
	if err != nil {
		return err
	}
	if streamErr != nil {
		return streamErr
	}
	return errors.New("Windows system-output capture stopped")
}

//go:uintptrescapes
func createAudioEnumerator() (uintptr, error) {
	classID := guid("bcde0395-e52f-467c-8e3d-c4579291692e")
	interfaceID := guid("a95664d2-9614-4f35-a746-de8db63617e6")
	var out uintptr
	hr, _, _ := ole32.NewProc("CoCreateInstance").Call(
		uintptr(unsafe.Pointer(&classID)), 0, 23,
		uintptr(unsafe.Pointer(&interfaceID)), uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(classID)
	runtime.KeepAlive(interfaceID)
	runtime.KeepAlive(&out)
	if int32(hr) < 0 {
		return 0, fmt.Errorf("WASAPI device enumerator unavailable (0x%08x)", uint32(hr))
	}
	return out, nil
}

func parseWindowsAudioFormat(ptr uintptr) (windowsAudioFormat, error) {
	var out windowsAudioFormat
	if ptr == 0 {
		return out, errors.New("empty Windows audio format")
	}
	raw := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 18)
	tag := int(binary.LittleEndian.Uint16(raw[0:2]))
	out.channels = int(binary.LittleEndian.Uint16(raw[2:4]))
	out.rate = int(binary.LittleEndian.Uint32(raw[4:8]))
	out.blockAlign = int(binary.LittleEndian.Uint16(raw[12:14]))
	out.bits = int(binary.LittleEndian.Uint16(raw[14:16]))
	if tag == audioFormatExtensible {
		if binary.LittleEndian.Uint16(raw[16:18]) < 22 {
			return out, errors.New("unsupported extensible Windows audio format")
		}
		raw = unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 40)
		subtype := binary.LittleEndian.Uint32(raw[24:28])
		switch subtype {
		case audioFormatPCM:
			tag = audioFormatPCM
		case audioFormatFloat:
			tag = audioFormatFloat
		default:
			return out, errors.New("unsupported Windows audio sample format")
		}
	}
	if out.channels < 1 || out.channels > 8 || out.rate < 8000 || out.rate > 192000 ||
		(out.bits != 8 && out.bits != 16 && out.bits != 24 && out.bits != 32) ||
		out.blockAlign != out.channels*((out.bits+7)/8) {
		return out, errors.New("unsupported Windows audio format parameters")
	}
	if tag == audioFormatFloat {
		if out.bits != 32 {
			return out, errors.New("unsupported Windows floating-point audio format")
		}
		out.float = true
	} else if tag != audioFormatPCM {
		return out, errors.New("unsupported Windows audio encoding")
	}
	return out, nil
}

func (r *audioResampler) convert(data []byte, frames int, format windowsAudioFormat, silent bool) ([]byte, error) {
	if frames < 0 || frames > 1<<20 || (!silent && len(data) != frames*format.blockAlign) {
		return nil, errors.New("invalid Windows audio frame")
	}
	if format.rate == audioRate && format.channels == audioChannels && format.bits == 16 && !format.float {
		if silent {
			return make([]byte, frames*audioChannels*2), nil
		}
		return append([]byte(nil), data...), nil
	}
	for i := 0; i < frames; i++ {
		var left, right float64
		if !silent {
			left = windowsAudioSample(data, i, 0, format)
			channel := 0
			if format.channels > 1 {
				channel = 1
			}
			right = windowsAudioSample(data, i, channel, format)
		}
		r.pending = append(r.pending, [2]float64{left, right})
	}
	step := float64(format.rate) / audioRate
	out := make([]byte, 0, frames*audioChannels*2*audioRate/format.rate+8)
	for r.phase+1 < float64(len(r.pending)) {
		index := int(r.phase)
		fraction := r.phase - float64(index)
		left := r.pending[index][0]*(1-fraction) + r.pending[index+1][0]*fraction
		right := r.pending[index][1]*(1-fraction) + r.pending[index+1][1]*fraction
		var sample [2]byte
		binary.LittleEndian.PutUint16(sample[:], uint16(floatToPCM16((left+right)/2)))
		out = append(out, sample[:]...)
		r.phase += step
	}
	discard := int(r.phase)
	if discard > 0 && discard < len(r.pending) {
		r.pending = append(r.pending[:0], r.pending[discard:]...)
		r.phase -= float64(discard)
	}
	return out, nil
}

func windowsAudioSample(data []byte, frame, channel int, format windowsAudioFormat) float64 {
	at := frame*format.blockAlign + channel*((format.bits+7)/8)
	switch {
	case format.float:
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[at : at+4])))
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return math.Max(-1, math.Min(1, v))
	case format.bits == 8:
		return (float64(data[at]) - 128) / 128
	case format.bits == 16:
		return float64(int16(binary.LittleEndian.Uint16(data[at:at+2]))) / 32768
	case format.bits == 24:
		v := int32(data[at]) | int32(data[at+1])<<8 | int32(data[at+2])<<16
		if v&0x800000 != 0 {
			v |= ^0xffffff
		}
		return float64(v) / 8388608
	default:
		return float64(int32(binary.LittleEndian.Uint32(data[at:at+4]))) / 2147483648
	}
}

func floatToPCM16(v float64) int16 {
	if v <= -1 {
		return -32768
	}
	if v >= 1 {
		return 32767
	}
	return int16(math.Round(v * 32767))
}
