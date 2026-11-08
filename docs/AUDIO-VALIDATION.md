# Audio forwarding validation — v0.1.14

## Verified

- Linux monitor capture produced non-silent PCM while system audio was playing.
- The hub streamed Linux output to the TomTom at 5% system output and 5% independent PCM gain; the owner confirmed the brief sample was comfortable.
- `go test -race ./host`, Windows amd64/arm64 cross-builds, ARM device build, face/Quick settings test, package generation and archive checksum checks passed for the v0.1.14 candidate.
- The Windows packages reuse the unchanged renderer DLL from the previously validated build; its source is unaffected by this release. The Windows installer workflow still needs a Windows runner.
- The time-sync path sends Unix time shortly after hub startup and every five minutes. The connected device reports a valid clock; exact drift was not measured.

The successful hardware audio sample used the earlier audio-test image, not the combined v0.1.14 device image. The combined image has been compiled but has not yet been deployed for hardware playback testing.

## Not yet verified

- Windows WASAPI loopback capture has only been cross-compiled; it has not been exercised on a Windows machine.
- End-to-end latency, CPU use and underrun behavior have not been measured. The 20 ms PCM frame size is not an end-to-end latency guarantee.
- Windows audio output through a connected TomTom remains untested.

Audio forwarding is disabled by default. The output-level control scales PCM independently of the source system volume and defaults to 5%.
