# Current codebase architecture

This is the implementation map for the local v0.1.15 Beta candidate based on v0.1.14. It is intentionally separate from [ARCHITECTURE.md](ARCHITECTURE.md), which specifies the broader target architecture. Source defines behavior; [RELEASE-VALIDATION.md](RELEASE-VALIDATION.md) records what was actually demonstrated. Read the newest sections first in the chronological implementation and validation records.

## What this repository builds

FractureOS replaces the **application startup** of a TomTom ONE (v6) running OpenTom. It is not a replacement bootloader, kernel, firmware distribution, navigation/map application, or modern Linux image.

- **Device:** a small C program directly owns framebuffer, switch-style touch input, power-button sampling, local clock/tools, settings and bounded integration state.
- **Linux host:** a mostly standard-library Go executable acts as TLS hub, CLI, settings server and supervisor for optional worker subprocesses. The opt-in agenda uses pinned iCalendar parsing and recurrence-expansion modules. Python tooling stages and verifies application payloads over the legacy device link.
- **Windows:** the same Go source builds a native tray and separate Win32/GDI dashboard, with optional media and temperature workers. A small C DLL shares the home renderer; there is no Electron/WebView runtime. The published v0.1.11 installer is Inno Setup; the in-app updater downloads and checksum-verifies the release setup exe. Windows notification capture needs a packaged build and is not yet runtime validated.
- **Assets:** declarative clock geometry/palettes and pre-rasterized fonts/artwork, not executable face plugins.

## Actual communication chain

```text
Windows tray / native dashboard / CLI
              |
              | HTTPS + pinned certificate + bearer token
              v
Linux Go hub (default loopback:47831; explicitly configurable)
              |
              | serialized, bounded root Telnet commands on trusted USB/LAN
              v
TomTom shell -> fracture control -> root-only local Unix socket
              |
              v
single C framebuffer/input owner
```

The optional web settings process serves a session-protected loopback UI; it is distinct from opening the authenticated hub API to a browser. Host-side weather/lyrics providers use HTTPS from the modern host; the TomTom does not contact those APIs or parse their JSON/images.

The framed HMAC USB service, TLS WebSocket protocol, per-peer scopes, general subscription registry and revisioned configuration transactions in `ARCHITECTURE.md` are **design**, not the current transport. In particular, do not debug a nonexistent device TCP listener on port 47832.

The owner's current network separates Linux/hub `.105`, device USB `.115`, and forwarded device LAN alias `.114`. Older design documents describe `.114` as the Linux address; that is historical. [NETWORK.md](NETWORK.md) owns the host-specific mapping and interface-rename recovery details. These addresses are deployment facts, not authentication or portable installer requirements. Do not expose root Telnet to the Internet; remote access terminates on the modern host over a private network/VPN.

## Device ownership and state

`device/src/fracture.c` is the executable entry point and integration owner. The headers beside it are C implementation slices included into that translation unit, not separately launched modules. They share renderer/app state and must stay compatible with the legacy compiler. The core remains resident; only selected assets, forecast/lyric windows and artwork are allocated on demand.

The app owns one process lock, the framebuffer, input descriptors, control socket and PWM handle. It maintains local clock/profile/settings, timer/stopwatch/visual alarm state and transient integration leases. It polls input at 50 ms because of the legacy button driver, but redraws according to visible content/events. A minute-only clock is not a zero-wakeup process. Device details and command/data contracts are in [DEVICE-RUNTIME.md](DEVICE-RUNTIME.md).

`boot.sh` supervises the selected application and retains the original OpenTom recovery path. It must remain foreground: the original `/etc/rc` application chain can force a reboot if it returns. Live deployment cannot simply kill every old startup shell. [OPERATIONS.md](OPERATIONS.md) explains staging, activation, restoration and the limits of that recovery mechanism.

## Demand-driven host work

The single Go executable has different command modes; optional collection is implemented by launching another copy in a worker mode, not by installing separate Go plugins.

| Owner | Demand | Work kept outside the device core |
|---|---|---|
| Linux hub | Metrics enabled and visible Desk consumer | procfs/sysfs CPU, RAM, CPU temperature and traffic sampling |
| Linux hub | Weather provider enabled and device requesting weather | Open-Meteo current/hourly/daily fetch, validation/cache, compact device updates |
| Linux hub | Agenda feed configured and TomTom Today page open | Bounded HTTPS ICS fetch, recurrence expansion and transient summaries; no feed/event disk cache |
| Linux hub | Lyrics page requesting text for the current track | LRCLIB/provided lyrics, parsing, bounded cache and rolling-window scheduling |
| Windows tray | Media sharing enabled and device requesting media | WinRT Spotify session metadata/artwork, queued playback controls, Core Audio volume |
| Windows tray | PC temperature enabled and telemetry requested | PawnIO sensor reads; optional explicitly authorized elevated helper |
| Windows dashboard | Home preview visible | Selected assets + compact model -> shared C renderer -> local BGRA image |

Feature disablement and disappearance of consumers stop the relevant work; individual paths have documented polling/cache/grace periods rather than one universal lease framework. A short-lived cached value is not proof of a still-running collector. Conversely, hiding a UI element does not establish process unloading: inspect the actual owner and lifecycle. See [HOST-RUNTIME.md](HOST-RUNTIME.md).

## Shared home preview

The preview is not screen streaming. `device/src/preview.c` compiles the common home drawing path without Linux device/input/socket/navigation code. The device exports a bounded `FPR1` snapshot and allowlisted selected drawing assets. The host validates/transfers assets by content hash. The Windows dashboard calls `fracture-preview.dll`, converts output to its native GDI surface, and releases the DLL/frame/asset allocations when preview is hidden, minimized or closed.

There is one rendering implementation, but two platform build paths. Changes to shared home state/layout can require coordinated edits to `fracture.c`, `home_model.h`, `preview.c`, `host/preview.go`, `host/preview_windows.go` and dashboard lifecycle code. Building only the Go EXE does not rebuild the C DLL. One release check matched a device capture pixel-for-pixel; that is evidence for that case, not an exhaustive parity proof.

## Persistence and boundaries

- Device releases live below `/mnt/sdcard/fracture-v3/releases/vVERSION`; selection, user face/theme overrides, private font and caches live outside a release slot so updates preserve them.
- Host config, pairing material and backups live in per-user data/config directories, not source files. The dashboard redacts credentials in its settings responses.
- Device timers/stopwatch are runtime state, not restart-persistent schedules. Alarm settings persist but require the device running; alarms are visual and silent.
- Time uses a configured UTC offset; automatic DST/IANA transition-table scheduling in the design is not implemented.
- Temp-file/rename saves and checksummed transfers are useful integrity measures, not a FAT power-loss guarantee, signed update chain or activation journal.
- UI menus, clock and timer code are compiled together. This is not the design's general dynamic module loader, supervisor registry or arbitrary widget scene engine.

## Where to make coordinated changes

| Change | Read together |
|---|---|
| Home face geometry, information dock or symbols | `fracture.c`, `faces.h`, `dashboard.h`, `symbols.h`, `home_model.h`, preview C/Go paths and `docs/FACES.md` |
| Native Windows dashboard layout | `dashboard_windows.go`, `dashboard_layout.go`, `dashboard_layout_test.go`; verify the resulting window on Windows before claiming visual acceptance |
| What's new / release notes | `changelog.go`, `/changelog` in the session-protected settings server, and the web/native dashboard views |
| Input/menu behavior | `fracture.c`, `navigation.h`, `power.h`, `tests/core_test.c`; physically validate switch behavior |
| New command/status field | Device command/parser/JSON status, `host/main.go` validation, Go tests, web/native consumers and shell control wrapper |
| Media timeline/lyrics | `media.h`, Go media/Windows bridge/lyrics files, Lyrics navigation and rolling-buffer lifetime |
| Weather | `weather.h`, `forecast.h`, Go weather/forecast files, integration settings and FWE1 cache contract |
| Installation/recovery | Device startup/control scripts, deployer, host `device` command, package layout and installers |
| Release version/package | Device build script, host version, installer defaults/metadata, packaging and Windows workflow; inspect all version sources rather than changing one string |
| Branding/font assets | Asset source/exports/embedding copies, Win32 resources, release payload and private-font boundary |

This page is the high-level implementation map; the README links the current user and development references. The Windows notification packaging transition and its validation gates are tracked in [IMPLEMENTATION.md](IMPLEMENTATION.md) and [RELEASE-VALIDATION.md](RELEASE-VALIDATION.md).

## Interpreting older documents

`IMPLEMENTATION.md` and `RELEASE-VALIDATION.md` retain historical release sections. Older statements about browser-only Windows settings, absent media/weather, Linux `.114`, icon activation waiting for reconnect, or untested installer behavior are superseded by newer sections where evidence exists. `TASKS.md` distinguishes bounded release deliverables from the larger P-series module acceptance criteria; its older “Current handoff” paragraph still names v0.1.9. Do not treat an old paragraph as current device state.

`NEXT-DEVICE-APPLY.md` records the separately installed v0.1.12 TomTom build and its deployment evidence. It also records a later restart for the unsuccessful candidate BMP experiment, whereas earlier validation paragraphs say no cold reboot was performed in their own sessions. Neither statement establishes full cold-boot, RTC-retention or suspend/wake acceptance. Always read fresh device state before planning another apply, and preserve the owner's current preferences rather than replaying historical settings.
