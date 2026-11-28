package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	audioRate       = 16000
	audioChannels   = 1
	audioFrameMS    = 20
	audioFrameBytes = audioRate * audioChannels * 2 * audioFrameMS / 1000
	audioQueueDepth = 10
	audioPort       = "18745"
)

type AudioConfig struct {
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
	Volume  int    `json:"volume"`
}

func defaultAudioConfig() AudioConfig { return AudioConfig{Source: "windows", Volume: 5} }

func validateAudio(c AudioConfig) error {
	if c.Source != "windows" && c.Source != "linux" {
		return errors.New("audio source must be windows or linux")
	}
	if c.Volume < 1 || c.Volume > 100 {
		return errors.New("audio volume must be between 1 and 100")
	}
	return nil
}

func audioWorkerCommand() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return localAudioWorker(ctx)
}

func audioSupervisor(ctx context.Context, b *Broker, localSource string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var cancel context.CancelFunc
	var done <-chan error
	var active AudioConfig
	retryAt := time.Time{}
	stop := func() {
		if cancel != nil {
			cancel()
			<-done
			cancel = nil
			done = nil
		}
	}
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			cancel()
			cancel = nil
			done = nil
			if err != nil && ctx.Err() == nil {
				log.Printf("audio capture: %v", err)
			}
			retryAt = time.Now().Add(5 * time.Second)
		case <-ticker.C:
			c, err := readConfig()
			if err != nil {
				stop()
				log.Printf("audio settings: %v", err)
				continue
			}
			audio := c.Audio
			if b == nil {
				settings, err := integrationGetContext(ctx, c)
				if err != nil || settings.Audio == nil {
					stop()
					if err != nil {
						log.Printf("audio settings: %v", err)
					}
					continue
				}
				audio = *settings.Audio
			}
			if !audio.Enabled || audio.Source != localSource {
				stop()
				continue
			}
			var raw string
			if b != nil {
				raw, err = b.call("status")
			} else {
				raw, err = hubCallContext(ctx, c, "status")
			}
			if err != nil {
				stop()
				log.Printf("audio device status: %v", err)
				continue
			}
			var state struct {
				AudioBlocked bool `json:"audio_blocked"`
			}
			if err = json.Unmarshal([]byte(raw), &state); err != nil {
				stop()
				log.Printf("audio device status: %v", err)
				continue
			}
			if state.AudioBlocked {
				stop()
				continue
			}
			if cancel != nil && active != audio {
				stop()
			}
			if cancel != nil || time.Now().Before(retryAt) {
				continue
			}
			workerCtx, stopWorker := context.WithCancel(ctx)
			ch, err := launchAudioWorker(workerCtx)
			if err != nil {
				stopWorker()
				retryAt = time.Now().Add(5 * time.Second)
				log.Printf("audio capture: %v", err)
				continue
			}
			cancel, done, active = stopWorker, ch, audio
		}
	}
}

func registerAudio(mux *http.ServeMux, b *Broker, token string) {
	mux.HandleFunc("/api/audio", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/octet-stream" {
			http.Error(w, "POST audio/pcm required", http.StatusUnsupportedMediaType)
			return
		}
		source := r.Header.Get("X-Fracture-Audio-Source")
		if source != "linux" && source != "windows" {
			http.Error(w, "invalid audio source", http.StatusBadRequest)
			return
		}
		c, err := readConfig()
		if err != nil {
			http.Error(w, "audio settings unavailable", http.StatusInternalServerError)
			return
		}
		if !c.Audio.Enabled || c.Audio.Source != source {
			http.Error(w, "audio forwarding disabled or source not selected", http.StatusConflict)
			return
		}
		if !b.audioTryBusy() {
			http.Error(w, "another audio stream is active", http.StatusConflict)
			return
		}
		defer b.audioSetBusy(false)

		controller := http.NewResponseController(w)
		if err = controller.SetReadDeadline(time.Time{}); err != nil {
			http.Error(w, "cannot clear audio read deadline", http.StatusInternalServerError)
			return
		}
		if err = controller.SetWriteDeadline(time.Time{}); err != nil {
			http.Error(w, "cannot clear audio write deadline", http.StatusInternalServerError)
			return
		}
		raw, err := b.call("status")
		var state struct {
			AudioBlocked bool `json:"audio_blocked"`
		}
		if err != nil || json.Unmarshal([]byte(raw), &state) != nil {
			http.Error(w, "device status unavailable", http.StatusBadGateway)
			return
		}
		if state.AudioBlocked {
			http.Error(w, "audio is preempted by an alarm", http.StatusConflict)
			return
		}
		sessionKey := make([]byte, 16)
		if _, err = rand.Read(sessionKey); err != nil {
			http.Error(w, "cannot create audio session", http.StatusInternalServerError)
			return
		}
		streamToken := hex.EncodeToString(sessionKey)
		if _, err = b.call("audio-start " + streamToken); err != nil {
			http.Error(w, "TomTom audio sink could not start", http.StatusBadGateway)
			return
		}
		defer func() {
			if _, e := b.call("audio-stop"); e != nil {
				log.Printf("audio sink stop: %v", e)
			}
		}()

		conn, err := net.DialTimeout("tcp", net.JoinHostPort(b.device, audioPort), 3*time.Second)
		if err != nil {
			http.Error(w, "TomTom audio sink is unreachable", http.StatusBadGateway)
			return
		}
		defer conn.Close()
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetWriteBuffer(audioFrameBytes * audioQueueDepth)
			_ = tcpConn.SetNoDelay(true)
		}
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		token := []byte(streamToken)
		for len(token) > 0 {
			n, writeErr := conn.Write(token)
			if writeErr != nil {
				http.Error(w, "TomTom audio authentication failed", http.StatusBadGateway)
				return
			}
			if n == 0 {
				http.Error(w, "TomTom audio authentication failed", http.StatusBadGateway)
				return
			}
			token = token[n:]
		}
		var ready [4]byte
		if _, err = io.ReadFull(conn, ready[:]); err != nil || string(ready[:]) != "OKAY" {
			http.Error(w, "TomTom audio format is unavailable", http.StatusBadGateway)
			return
		}
		_ = conn.SetReadDeadline(time.Time{})

		frames := make(chan []byte, audioQueueDepth)
		readDone := make(chan error, 1)
		go func() {
			defer close(frames)
			for {
				frame := make([]byte, audioFrameBytes)
				_, e := io.ReadFull(r.Body, frame)
				if e == io.EOF || e == io.ErrUnexpectedEOF {
					readDone <- nil
					return
				}
				if e != nil {
					readDone <- e
					return
				}
				enqueueAudioFrame(frames, frame)
			}
		}()
		var seq uint32
		var writeErr error
		for frame := range frames {
			scaleAudioFrame(frame, c.Audio.Volume)
			if writeErr = writeDeviceAudioFrame(conn, seq, frame); writeErr != nil {
				_ = r.Body.Close()
				break
			}
			seq++
		}
		readErr := <-readDone
		if writeErr != nil {
			http.Error(w, "TomTom audio stream failed", http.StatusBadGateway)
			return
		}
		if readErr != nil && r.Context().Err() == nil {
			http.Error(w, "audio source stream failed", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (b *Broker) audioTryBusy() bool {
	b.audioMu.Lock()
	defer b.audioMu.Unlock()
	if b.audioActive {
		return false
	}
	b.audioActive = true
	return true
}

func (b *Broker) audioSetBusy(active bool) {
	b.audioMu.Lock()
	b.audioActive = active
	b.audioMu.Unlock()
}

func enqueueAudioFrame(queue chan []byte, frame []byte) {
	select {
	case queue <- frame:
		return
	default:
	}
	select {
	case <-queue:
	default:
	}
	select {
	case queue <- frame:
	default:
	}
}

func scaleAudioFrame(pcm []byte, volume int) {
	for i := 0; i+1 < len(pcm); i += 2 {
		sample := int16(binary.LittleEndian.Uint16(pcm[i : i+2]))
		scaled := int16(int32(sample) * int32(volume) / 100)
		binary.LittleEndian.PutUint16(pcm[i:i+2], uint16(scaled))
	}
}

func writeDeviceAudioFrame(w io.Writer, sequence uint32, pcm []byte) error {
	if len(pcm) != audioFrameBytes {
		return errors.New("invalid PCM frame size")
	}
	var header [16]byte
	copy(header[:4], "FRA1")
	binary.BigEndian.PutUint32(header[4:8], sequence)
	binary.BigEndian.PutUint64(header[8:16], uint64(time.Now().UnixMilli()))
	for _, part := range [][]byte{header[:], pcm} {
		for len(part) > 0 {
			n, err := w.Write(part)
			if err != nil {
				return err
			}
			if n == 0 {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}

func hubStreamingClient(c Config) (*http.Client, *http.Transport, error) {
	transport, err := hubTransport(c)
	if err != nil {
		return nil, nil, err
	}
	transport.ExpectContinueTimeout = 100 * time.Millisecond
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("hub redirects refused")
	}}
	return client, transport, nil
}

func hubAudioStream(ctx context.Context, c Config, source string, pcm io.ReadCloser) error {
	client, transport, err := hubStreamingClient(c)
	if err != nil {
		return err
	}
	defer transport.CloseIdleConnections()
	reader, writer := io.Pipe()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.Hub, "/")+"/api/audio", reader)
	if err != nil {
		reader.Close()
		writer.Close()
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Expect", "100-continue")
	req.Header.Set("X-Fracture-Audio-Source", source)
	result := make(chan error, 1)
	go func() {
		resp, e := client.Do(req)
		if e == nil {
			defer resp.Body.Close()
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4097))
			if len(body) > 4096 {
				e = errors.New("audio hub response too large")
			} else if readErr != nil {
				e = readErr
			} else if resp.StatusCode != http.StatusNoContent {
				e = fmt.Errorf("audio hub: %s", strings.TrimSpace(string(body)))
			}
		}
		result <- e
	}()
	copyDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(writer, pcm)
		_ = writer.CloseWithError(copyErr)
		copyDone <- copyErr
	}()
	var copyErr error
	select {
	case err = <-result:
		_ = pcm.Close()
		_ = reader.Close()
		_ = writer.CloseWithError(err)
		copyErr = <-copyDone
	case copyErr = <-copyDone:
		err = <-result
	case <-ctx.Done():
		_ = pcm.Close()
		_ = reader.Close()
		_ = writer.CloseWithError(ctx.Err())
		<-copyDone
		<-result
		return nil
	}
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	if err == nil && copyErr != nil {
		return copyErr
	}
	return errors.New("audio source ended")
}
