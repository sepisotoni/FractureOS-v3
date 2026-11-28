# FractureOS v3 task board

Status: v0.1.11 remains the latest stable release and v0.1.14 the latest published prerelease. This branch is a v0.1.15 Beta candidate based on v0.1.14; four platform archives have been built and checksummed locally, but no v0.1.15 GitHub release is published. Windows visual and physical-device gates remain open. See [release validation](docs/RELEASE-VALIDATION.md). Updated for the v0.1.15 integration.

Checkboxes mean the stated deliverable is complete, not merely discussed. Design tasks and implementation tasks are tracked separately. Update this file as each deliverable lands; add evidence or a limitation alongside completed implementation tasks. Experimental features remain open until demonstrated on the target hardware or explicitly declined by the owner.

## R — Initial deployable release (bounded subset of the roadmap)

- [x] R01 Build C framebuffer clock, switch-operated launcher, profiles, countdown/stopwatch, visual alarm and persistent settings.
- [x] R02 Reuse nine original face designs as data assets; exclude old sidebar/Nano-X/weather application code; retain font notices and artwork provenance.
- [x] R03 Load only the selected atlas; add editable colors/rectangles, face reset and host JSON import/export.
- [x] R04 Add an iOS-inspired pure-black UI, compact text atlas, geometric symbols and brief selection animations; add customizable UI themes and optional face recoloring; restore the original blue face with date/seconds, retain optional large stacked/horizontal layouts, blinking colon and hour-only behavior during minute 00.
- [x] R05 Run C core/face checks on the real ARM device; verify upload readbacks and capture framebuffer output.
- [x] R06 Implement TLS/pinned-certificate Go hub, CLI and on-demand settings UI.
- [x] R07 Implement separate optional Linux metrics worker and native Windows tray; cross-compile both PC architectures.
- [x] R08 Build platform-selecting Linux/Windows installers and uninstallers; Linux install/uninstall fixture passed.
- [x] R09 Manually check all nine faces with extra information, rehearse device restore/reinstall and test the actual TLS hub, collector shutdown and settings endpoints.
- [x] R10 Publish repository and release downloads, then test the public installation command. v0.1.7 public installation completed on the Linux hub.
- [x] R11 Validate Windows installer/tray on a real Windows session. Windows x64 runner passed native UI, tray close, in-place update/reopen and uninstall. Owner-specific playback acceptance remains separate; the owner reports PawnIO works.

## U — Requested UI, input and media revision

- [x] U01 Add separate Timer/custom duration, Stopwatch/pause/lap/reset, Alarm/custom time, Media and Settings pages.
- [x] U02 Add release-based 1.5s Back, 3s Home, 4.5–7s cancel and 7s+ suspend gestures with optional colored indicator. Owner physically confirmed indicator and Back/Home on preview. Suspend/reboot/shutdown have not been executed during development.
- [x] U03 Render date/seconds at final readable size and thicken digits by configurable 0–2 pixels; dynamically size status information. Manual device captures recorded.
- [x] U04 Restore CPU/RAM icons, add disk and download/upload measurements; verify live hub values and lazy worker shutdown on device.
- [x] U05 Implement a Windows Spotify client bridge without developer credentials; validate device/hub payloads and Windows cross-builds. Real Windows playback/artwork/control acceptance remains separate.
- [x] U06 Package, deploy and publish v0.1.8, update controls/install documentation and verify public installation. Version-pinned GitHub installer completed on Linux and the hub service reported active.

The P-series checkboxes below describe complete target modules. Initial implementations above do not imply those broader acceptance criteria are finished.

## V — Dashboard, weather and compact UI revision

- [x] V01 Hide missing metrics, show maximum traffic direction, center the date and adapt crowded clocks to horizontal time.
- [x] V02 Add on-device face/theme previews, day/night appearance and a compact neon dashboard face.
- [x] V03 Add persistent on-device module switches and PC controls.
- [x] V04 Add an opt-in hub weather worker, unusual-only settings and compact weather symbols.
- [x] V05 Add separate native Windows and web dashboards, with no dashboard process while closed.
- [x] V06 Build an Inno Setup installer with uninstallation and publish downloadable builds.
- [x] V07 Correct Linux/TomTom LAN address ownership and verify routing.
- [x] V08 Add authenticated notification forwarding and application exclusions. Silent webhook and an opt-in Windows listener candidate are implemented; Store/install/runtime gates and notification sound remain pending.
- [x] V10 Replace disk widget with live CPU temperature; add optional signed PawnIO sensor support for Windows.
- [x] V09 Manually validate on the TomTom, preserve settings and document measured limits.

## W — Adaptive dock, media controls and branding

- [x] W01 Pack timers/stopwatch/alarm into one measured strip; dock telemetry beside media/weather.
- [x] W02 Add rounded notification overlay, optional artwork and configurable 10-second lifetime.
- [x] W03 Add configurable 30-second menu inactivity timeout with preview cancellation.
- [x] W04 Draw Spotify and transport/volume/more icons; add player clock.
- [x] W05 Add bounded on-demand synchronized/plain lyrics lookup and authenticated lyrics webhook.
- [x] W06 Correct PawnIO access handling and add an explicitly approved elevated sensor helper. Flow and pinned-module checks implemented and Windows builds passed; the owner reports PawnIO works on their PC. The exact hardware and authorization path were not independently inspected.
- [x] W07 Create one FractureOS vector logo, app/tray/installer resources and web favicon; replace backed-up TomTom icon. Final root icon SHA-256 verified after reconnection.
- [x] W09 Add on-demand cached hourly/7-day Weather app.
- [x] W10 Add verified one-click app updates in native/web dashboards.
- [x] W08 Manually inspect real-device layouts/lifecycles, build Windows installers, deploy and publish. Final v0.1.10 activated after reconnection with settings preserved; matching Linux hub installed. Windows updater acceptance passed; W06 tracks actual sensor hardware separately.

## D — Design

- [x] D01 Reconcile requirements, inspected hardware, broken touch input, and physical connectivity. See `docs/ARCHITECTURE.md`.
- [x] D02 Define component ownership, actual unloading, scheduling, resource targets, and failure handling.
- [x] D03 Define protocol, configuration, face format, extension contract, and update/recovery design.
- [x] D04 Specify all README modules, defaults, dependencies, and acceptance scenarios. See `docs/MODULES.md`.
- [x] D05 Order implementation work and record hardware/platform gates in this task board.

## P0 — Prove the platform

- [x] P000 Build and verify Telnet/transfer harness, diagnostic C binary and device shell smoke script. Final run: 17 harness checks and seven C primitive checks passed; six host protocol regression tests passed. See `docs/DEVICE-VALIDATION.md`.
- [x] P001 Record current boot scripts, mounts, device drivers, relevant kernel headers, compiler ABI, libc/loader and executable format. Current boot image matched the local OpenTom build; targeted backups and SDK provenance saved. Previous prototype directory is absent.
- [ ] P002 Produce a reproducible ARMv5 build toolchain/sysroot compatible with Linux 2.6.13 and the discovered ABI. Installing an ARM compiler alone does not establish compatibility.
  - Progress: `tools/verify-legacy-sdk.py` checks every recorded SDK archive/compiler/sysroot/header/source hash and builds the device app and probe twice to verify byte-identical output. The tested archive hash matches provenance. A portable toolchain package/bootstrap and fresh-machine validation remain outstanding.
- [x] P003 Build and run a minimal v3 C binary on-device; 10,728 bytes, exit 0, libc-only dependency, 424 KiB measured RSS. Transfer readback SHA-256 matches.
- [x] P004 Validate framebuffer ioctls, pitch, offsets and pixel format using a reversible display session.
  - Verified RGB565/pitch/offsets and live rendering on the device; framebuffer screenshots captured during release validation.
- [ ] P005 Validate touch press/release, noise and power-button durations; inspect existing power policy before replacing its event owner. No coordinate-based navigation requirement.
- [ ] P006 Validate brightness, RTC retention, battery status, audio output and GPS separately. Record unsupported capabilities explicitly; do not infer hardware from device-node names.
  - Progress: brightness 30, battery voltage/charge status and RTC read verified. GPS hardware reports Global Locate (`gpstype=128`); the original `gltt` helper produced a valid fix when run with a process-scoped `LD_LIBRARY_PATH` to the existing `/mnt/sdcard/opentom/lib`. No library was copied or persistent configuration changed. RTC retention, brightness writes and audio remain unverified.
- [x] P007 Save a host-side backup and boot-file hashes; define and rehearse restoration before enabling v3 autostart.
  - Boot image/start scripts backed up and hashed; live restore/reinstall rehearsed. This is a targeted recovery backup, not a full SD-card image.
- [ ] P008 Establish Linux/device and real Windows measurement baselines. Windows runtime tests require access to a Windows session; cross-compiling is not runtime verification.

## P1 — Local clock and module foundation

- [ ] P101 Add build layout, dependency/license manifest and native simulator sharing C renderer and input logic.
- [ ] P102 Implement supervisor, exclusive display/input ownership, module registry, leases and worker shutdown.
- [ ] P103 Implement framebuffer renderer, bounded assets, dirty rectangles, fonts and minimal face (M01–M02).
- [ ] P104 Implement coordinate-free launcher and gesture state machine, configurable actions and input noise handling (M03).
- [ ] P105 Implement persisted settings, configuration revisions and local rollback (M18).
- [ ] P106 Implement time validity, timezone transitions, scheduler and profile arbitration (M04–M05).
- [ ] P107 Implement alarms, countdowns, stopwatch, intervals, Pomodoro and reminders (M06).
- [ ] P108 Implement quiet feedback, dimming and capability-aware power behavior (M07).
- [ ] P109 Demonstrate standalone bedside, kitchen, travel, guest and focus modes with Windows and hub disconnected.

## P2 — Connected core and Windows configuration

- [ ] P201 Implement bounded protocol, subscriptions, time sync, pairing, revocation and reconnect (M08).
- [ ] P202 Implement Go Linux broker/service and USB route binding without replacing unrelated host networking.
- [ ] P203 Implement Go Windows native tray and separate on-demand settings/editor process (M09).
- [ ] P204 Implement face editor, templates, widget/action binding, preview, import/export and bounded device asset compiler (M10).
- [ ] P205 Implement status/activity worker and consent controls; demonstrate idle/active face switching (M11).
- [ ] P206 Implement optional Windows and Linux metrics workers and subscription aggregation (M12).
- [ ] P207 Implement full Settings coverage for device, profiles, inputs, privacy, modules, connections and updates.
- [ ] P208 Demonstrate Windows → Linux hub → USB TomTom flow, PC-off operation, and actual worker removal after last consumer exits.

## P3 — Integrations and remote operation

- [ ] P301 Implement weather worker and manual location/timezone defaults (M13).
- [ ] P302 Implement calendar summaries, local/imported calendars and scoped notification forwarding (M14).
- [ ] P303 Implement configured service checks, home automation adapter and action allowlist (M15).
- [ ] P304 Implement native Windows media sessions for Spotify/other compatible players (M16).
- [x] P305 Resolve provider choice: owner declined the account/developer API; use the Windows client bridge. No OAuth provider or token-refresh code is included.
- [ ] P306 Implement on-demand GPS location/time widget with invalid-fix handling (M17).
- [ ] P307 Implement hub settings access, authenticated remote relay over private network, revocation and stale-command rejection (M08/M09).
- [ ] P308 Demonstrate remote administration and offline recovery; document extra USB-host requirement for a physically relocated networked TomTom.

## P4 — Updates and observability

- [ ] P401 Implement bounded local diagnostics, module status and opt-in history/export (M19).
- [ ] P402 Implement signed manifests, staged release slots, validation and activation journal (M20).
- [ ] P403 Implement startup health check, automatic application rollback and host-assisted recovery; keep bootloader/kernel outside routine updates.
- [ ] P404 Implement Go host/Windows updater helper and configuration migration rollback.
- [ ] P405 Validate interrupted transfer, corruption, disk full, failed startup and interrupted activation on test storage before device trials.

## P5 — Media/display experiments

- [x] P501 Implement opt-in PCM audio forwarding with Linux and Windows capture sources and adjustable device gain. Linux playback is verified; Windows runtime, CPU, underruns and end-to-end latency still need measurement (M21).
- [ ] P502 Prototype cropped/window/screen mirroring with RGB565 dirty tiles and measured frame rate (M22).
- [ ] P503 Establish Windows WDK/tooling and signed-driver distribution route for true extension; this is a native driver exception to the Go host preference (M23).
- [ ] P504 Implement and validate virtual monitor connect/disconnect, mode handling, frame forwarding and recovery on Windows; keep driver opt-in.
- [ ] P505 Document measured support matrix and unsupported modes. No simulated success for unavailable audio/driver functionality.

## P6 — Release readiness

- [ ] P601 Verify parser bounds, module lifecycle, protocol C/Go compatibility, clock transitions, gesture ambiguity and action authorization with meaningful tests.
- [ ] P602 Measure idle and active RAM/CPU/wakeups; compare against architecture targets; fix regressions or document evidence-based revised budgets.
- [ ] P603 Run device soak with 100 profile/module cycles, USB reconnects and a 24-hour clock/alarm session.
- [ ] P604 Validate Windows tray restart, login/logout, sleep/wake, editor closing and disabled-collector behavior on a real Windows machine.
- [ ] P605 Complete installation/restore instructions, capability matrix, package provenance and release notes.

## Current handoff

The latest stable release is v0.1.11; v0.1.14 is the latest published prerelease and was installed on the Linux host and TomTom per its 2026-11-11 session record. This `release/v0.1.15-beta` candidate integrates the requested host/device work on the v0.1.14 base. Go/C source builds and focused tests pass; four platform archives and their SHA-256 manifest were generated locally. No v0.1.15 release, Store acceptance, Windows runtime or new TomTom deployment is claimed. The listener implementation does not establish Windows runtime acceptance. Broader P-series items remain roadmap, not completed implementation.

## X — v0.1.11 playback, navigation and local preview

- [x] X01 Buffer timed lyrics on-device, track subsecond playback and seeks/pauses; use song title in header and Spotify in Media.
- [x] X02 Batch telemetry and keep the last valid readings through a five-second delivery grace period.
- [x] X03 Power opens Apps; touch opens Quick settings with real battery/link information. Group timer, stopwatch and alarms inside Clock; add Home.
- [x] X04 Refine the curved purple/blue F; add a lightweight Fracture → FractureOS startup animation and inspect the earlier boot-logo mechanism. Application animation checked; earlier firmware graphics remain unchanged.
- [x] X05 Render the home preview locally on Windows using the shared C renderer and synchronized face/theme/font/model data; unload it when hidden.
- [x] X06 Check on TomTom and Windows, preserve settings, package/install and publish v0.1.11 with explicit hardware limits.

## v0.1.12 — requested feature scope

- [x] V12-01 Add a date above and seconds at lower right on every supplied face except the owner's Nunito face; redesign TomTom Quick settings as compact icon-led tiles. Installed v0.1.12 on 2026-11-01, then deployed the refined centered-tile/top-right-status layout to a separate same-version slot with settings preserved; owner visual acceptance is pending.
- [x] V12-02 Add a read-only Today agenda from a user-configured ICS feed URL; send bounded upcoming summaries to the device. Disabled by default; local recurrence/payload tests pass, but no personal feed URL or new device build has been configured/validated.
- [x] V12-03 Add notification quiet hours using the Windows host's local time. Quiet-window notifications are dropped rather than replayed.
- [x] V12-04 Add opt-in Windows tray startup at user login. The existing Inno Setup installer exposes an unchecked startup task.
- [x] V12-05 Add notification listener/permission/hub/device health diagnostics, last-success time and a synthetic test-forward action. Code is implemented; Windows runtime acceptance remains gated below.
- [x] V12-06 Improve exact per-app allow/block controls while retaining an explicit privacy opt-in. Matching supports app display names and Windows app IDs; block rules take precedence.
- [x] V12-07 Add quick-select face presets for common face/layout/theme combinations in the web Appearance view and on the TomTom.
- [x] V12-08 Check the battery signal before adding a warning: the current driver/app path exposes charge state but no trustworthy capacity or low-charge threshold. No percentage is inferred from voltage and no low-battery warning is added.
- [ ] V12-09 Verify Windows notification forwarding end to end (needs a packaged build for listener access).

## v0.1.12 Beta — host and weather polish

- [x] B12-01 Reorganize the desktop web dashboard into Overview, Device, Appearance, Integrations and System views with local SVG icons. Mobile-specific layout is intentionally out of scope.
- [x] B12-02 Expand Open-Meteo condition names and device labels to distinguish cloud cover, precipitation type/intensity, showers, fog, snow and thunderstorms; unknown codes stay explicit.
- [x] B12-03 Replace question-mark corruption in song titles with bounded ASCII transliteration and an explicit fallback for unsupported scripts.
- [x] B12-04 Consolidate the Windows Start-menu shortcuts into one FractureOS dashboard entry; retain optional tray startup and the standard Installed Apps uninstaller.
- [x] B12-05 Validate the web dashboard in Chrome, capture a README screenshot, measure host process memory and check collector/preview cleanup. Desktop navigation passed; mobile-specific layout was subsequently dropped per owner request. Visible-view polling and the previous v0.1.11 native-preview unload evidence are recorded in `docs/RELEASE-VALIDATION.md`. The 13,060 KiB RSS figure is a point sample, not a soak result.
- [ ] B12-06 Test real Windows notification forwarding end to end; no public claim is made until verified.

## v0.1.13 Beta — native and device refinement

- [ ] B13-01 Redesign the native Go/Win32 dashboard and make controls reflow at supported window sizes; retain lazy preview and worker lifecycles.
  - Progress: resize-aware navigation, controls and overview cards are implemented with layout tests and Windows cross-builds. Windows visual/interaction acceptance is still required; see `docs/RELEASE-VALIDATION.md`.
- [ ] B13-02 Refine TomTom app views with practical scalable navigation and weather condition artwork; four device-side face/layout/theme presets are implemented, but device deployment and artwork acceptance still require separate validation.
- [x] B13-03 Implement the opt-in read-only agenda with host-local Today filtering, recurrence expansion, bounded transient device summaries and optional locations. A live personal feed and physical TomTom acceptance remain unverified; notification diagnostics and quiet hours are implemented.
- [x] B13-04 Add a shared Go changelog and expose it in the web and native dashboards; clearly distinguish stable releases from unpublished Beta work.
- [x] B13-05 Refresh README and implementation/release guides to distinguish the published v0.1.14 base from the unpublished v0.1.15 Beta candidate and record remaining acceptance gates.

## v0.1.18.1 integration status

The v0.1.18.1 candidate preserves per-face hour-only overrides, defaults Neon to hour-only, and exposes the setting on Windows and the TomTom. Its configurable idle timeout covers ordinary menus. The candidate also adds city search and an on-demand exact screen capture alongside the web clock preview. Windows x64 installer, resize, staged updater and uninstaller checks passed. Physical Windows USB, ARM64 execution and published-update checks remain open. See [release validation](docs/RELEASE-VALIDATION.md).

## Future — not scheduled for 0.1.18.1

- [ ] F-01 Menu previews. The PC preview renders only the home screen: `render()` compiles `navigation_draw()` out of the `FRACTURE_RENDER_ONLY` build and the preview forces `menu=0`. Previewing menus needs `navigation.h` (and its `agenda.h`, `forecast.h`, `power.h`, `startup.h` dependencies) in the render-only build, menu/selection state in `home-state`, and the wallpaper blur levels (`wallpaper_blur1/2`, which the device derives from the undimmed image) exported as preview assets. Needs native pixel tests like `tests/preview_drift.sh`, an ARM build, and Windows DLL validation. Declined for 0.1.18.1.
- [ ] F-02 Web dashboard preview fidelity. The dashboard draws faces with browser fonts and hand-tuned scale factors; measured 17.6–22.3% of pixels differ from the device renderer (`tests/preview_drift_js.sh`, images in `docs/preview-drift/`). Any fix must not add a C toolchain dependency to the Linux hub (`CGO_ENABLED=0`).
- [ ] F-03 On-device wallpaper file backup/restore. Only the selection is backed up today (`previous.json`); a device-side copy of `wallpaper.rgb` would need new telnet shell commands and hardware validation.
