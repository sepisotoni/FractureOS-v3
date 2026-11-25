package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestValidateAudioSource(t *testing.T) {
	for _, source := range []string{"windows", "linux"} {
		if err := validateAudio(AudioConfig{Source: source, Volume: 5}); err != nil {
			t.Fatalf("%q: %v", source, err)
		}
	}
	if err := validateAudio(AudioConfig{Source: "other", Volume: 5}); err == nil {
		t.Fatal("invalid source accepted")
	}
	for _, volume := range []int{0, 101} {
		if err := validateAudio(AudioConfig{Source: "linux", Volume: volume}); err == nil {
			t.Fatalf("invalid output volume %d accepted", volume)
		}
	}
}

func TestScaleAudioFrame(t *testing.T) {
	pcm := make([]byte, 8)
	for i, sample := range []int16{-32768, 32767, -1000, 1000} {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	scaleAudioFrame(pcm, 5)
	for i, want := range []int16{-1638, 1638, -50, 50} {
		got := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		if got != want {
			t.Fatalf("sample %d: got %d, want %d", i, got, want)
		}
	}
}

func TestAudioQueueDropsOldestFrame(t *testing.T) {
	queue := make(chan []byte, audioQueueDepth)
	for i := 0; i <= audioQueueDepth; i++ {
		enqueueAudioFrame(queue, []byte{byte(i)})
	}
	for i := 1; i <= audioQueueDepth; i++ {
		if got := <-queue; !bytes.Equal(got, []byte{byte(i)}) {
			t.Fatalf("frame %d: got %v", i, got)
		}
	}
}

func TestWriteDeviceAudioFrame(t *testing.T) {
	pcm := bytes.Repeat([]byte{0x5a}, audioFrameBytes)
	var packet bytes.Buffer
	if err := writeDeviceAudioFrame(&packet, 0x12345678, pcm); err != nil {
		t.Fatal(err)
	}
	got := packet.Bytes()
	if len(got) != 16+len(pcm) || string(got[:4]) != "FRA1" {
		t.Fatalf("invalid frame packet length or magic: %d bytes", len(got))
	}
	if seq := binary.BigEndian.Uint32(got[4:8]); seq != 0x12345678 {
		t.Fatalf("sequence: got %x", seq)
	}
	if !bytes.Equal(got[16:], pcm) {
		t.Fatal("PCM payload changed")
	}
	if err := writeDeviceAudioFrame(&packet, 0, pcm[:len(pcm)-1]); err == nil {
		t.Fatal("short PCM frame accepted")
	}
}
