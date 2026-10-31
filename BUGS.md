# BUGS.md — FractureOS v3 audit backlog

> Static inspection on 2026-10-31. **Not an exhaustive audit and not proof of runtime failures.** No hardware, Windows, or full test-suite execution was performed. Prioritize reproduction before fixes. This file tracks risks and defects separately.

| ID | Priority | Type | File(s) | Finding / next action |
|---|---|---|---|---|
| FOS-001 | High | Security hardening | `host/update.go`, `host/update_linux.go` | Release SHA-256 checksum comes from the same release distribution channel as the artifact and is not a signed trust root. Linux separately fetches a versioned installer shell script and executes it. Introduce verified release signing / trusted manifest and explicitly authenticate installer contents before execution. This is a **supply-chain risk**, not a demonstrated compromise. |
| FOS-002 | Medium | Recovery reliability | `tools/deploy-device.py` | Installation changes the device startup chain and relies on restoration succeeding after activation failure. A USB disconnect or interrupted rollback could leave the application unavailable. Exercise power-loss/USB-loss tests; consider boot-time health verification and device-side fallback. **Scenario not reproduced.** |
| FOS-003 | Low | Confirmed code cleanup | `device/src/fracture.c` (`framebuffer` near line 232) | After opening `/dev/fb`, some failed ioctl/format-validation paths return without closing the descriptor. Add close/reset on each error path. Usually reclaimed by process exit. |
| FOS-004 | Medium | Test gap | `host/media_windows.go`, `host/dashboard_windows.go`, `host/preview_windows.go` | Audit COM/WinRT reference lifetimes, asynchronous worker cancellation, preview buffers, and repeated open/close/minimize cycles. **No leak demonstrated.** |
| FOS-005 | Medium | Test gap | `host/sensor_elevation_windows.go`, `host/temperature_windows.go`, `host/pawnio.go` | Exercise privilege elevation, driver IOCTL failure handling, access-denied cases and sensor cleanup on supported real Windows hardware. **No confirmed vulnerability.** |
| FOS-006 | Medium | Test gap | `host/lyrics.go`, `host/weather.go`, `host/preview.go` | Simulate stale/corrupt cache, timeout, disconnect, pause/seek, rapid tab changes and unexpected network responses. Bounded responses and timeouts exist; this is a validation item. |
| FOS-007 | Medium | Transport limitation | `tools/tomtom.py`, `host/telnet.go` | Root Telnet is unencrypted. Keep it isolated to the trusted USB subnet; never expose it publicly. Document this operational constraint and test access isolation. |

## Positive controls observed
- Hub TLS certificate fingerprint pinning and bearer token; HTTPS-only requests.
- Explicit device identity checks and deployment backups/readback hashes.
- Bounded network reads and timeouts in examined host paths.

## Suggested verification
1. Reproduce FOS-003 with resource/error-path tests and fix it.
2. Define signed-update threat model and implement FOS-001.
3. Run device disconnect and reboot recovery drills for FOS-002.
4. Run Windows lifecycle, driver, and media stress tests for FOS-004–006.

No source files were changed as part of documenting these findings.
