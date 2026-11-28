package main

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestBrokerCancelInterruptsDeviceLogin(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	oldPort := devicePort
	devicePort = port
	defer func() { devicePort = oldPort }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := &Broker{device: "127.0.0.1", ctx: ctx}
	done := make(chan error, 1)
	go func() {
		_, err := broker.call("status")
		done <- err
	}()
	_ = listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second))
	peer, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled login returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("device login kept blocking after cancellation")
	}
}

func TestWeatherDescription(t *testing.T) {
	tests := map[int]string{
		0: "Clear", 1: "Mostly clear", 2: "Partly cloudy", 3: "Overcast",
		45: "Fog", 48: "Rime fog",
		51: "Light drizzle", 53: "Drizzle", 55: "Heavy drizzle",
		56: "Light freezing drizzle", 57: "Heavy freezing drizzle",
		61: "Light rain", 63: "Rain", 65: "Heavy rain",
		66: "Light freezing rain", 67: "Heavy freezing rain",
		71: "Light snow", 73: "Snow", 75: "Heavy snow", 77: "Snow grains",
		80: "Light rain showers", 81: "Rain showers", 82: "Heavy rain showers",
		85: "Snow showers", 86: "Heavy snow showers",
		95: "Thunderstorm", 96: "Thunderstorm with hail", 99: "Thunderstorm with heavy hail",
		4: "Unknown", 98: "Unknown",
	}
	for code, want := range tests {
		if got := weatherDescription(code); got != want {
			t.Errorf("weatherDescription(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestCommandBoundaries(t *testing.T) {
	for _, cmd := range []string{"status", "face nunito", "face-get", "face-set digit0 10,10,55,88", "face-reset", "profile bedside", "brightness 1", "brightness 100", "offset -720", "offset 840", "timer 0", "timer 86400", "alarm off", "alarm 23 59", "stats 0 100", "event double", "audio-start 0123456789abcdef0123456789abcdef", "audio-stop", "time 1800000000"} {
		if e := validateCommand(cmd); e != nil {
			t.Fatalf("%q: %v", cmd, e)
		}
	}
	for _, cmd := range []string{"", "face ../bad", "face-set atlas ../../bad", "face-set color $(id)", "status ; reboot", "profile minimal;reboot", "brightness 0", "brightness 101", "offset -721", "timer -1", "alarm 24 0", "alarm 1 60", "stats 1", "event reboot", "audio-start invalid", "audio-start 0123456789abcdef0123456789abcdef extra", "time 0", "unknown", "profile $(id)"} {
		if e := validateCommand(cmd); e == nil {
			t.Fatalf("unsafe/invalid command accepted: %q", cmd)
		}
	}
}

func TestConnectionModeValidation(t *testing.T) {
	if got := effectiveConnectionMode(Config{}); got != "hub" {
		t.Fatalf("default connection mode = %q, want hub", got)
	}
	if got := effectiveConnectionMode(Config{ConnectionMode: "direct"}); got != "direct" {
		t.Fatalf("direct connection mode = %q, want direct", got)
	}
}
