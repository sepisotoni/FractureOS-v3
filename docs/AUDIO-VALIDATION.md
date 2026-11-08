# Audio forwarding validation — v0.1.13

## Verified

- Linux monitor capture produced non-silent PCM while system audio was playing.
- The hub streamed Linux output to the TomTom using the isolated audio-test device slot, with the Linux sink at 5% and the separate PCM gain at 5%. The owner confirmed the brief sample was comfortable.
- During the successful sample, the `parec` capture worker remained active and the audio hub reported no stream-start failure. Forwarding was then disabled.
- After the controlled sample, forwarding was disabled and the production host and prior TomTom slot were restored. Linux output remained at 5%.
- `go test -race ./host`, Windows amd64/arm64 cross-builds, the ARM device build and release archive generation passed.
- Windows package archives reuse the renderer DLL from the checksum-verified v0.1.11 packages; its renderer source is unchanged in this release. The Windows installer workflow still needs a Windows runner.

## Not yet verified

- Windows WASAPI loopback capture has only been cross-compiled; it has not been exercised on a Windows machine.
- End-to-end latency, CPU use and underrun behavior have not been measured. The 20 ms PCM frame size is not an end-to-end latency guarantee.
- Windows audio output through a connected TomTom remains untested.

Audio forwarding is disabled by default. The output-level control scales PCM independently of the source system volume and defaults to 5%.
