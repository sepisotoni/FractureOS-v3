# FractureOS v3

FractureOS v3 is a from-scratch smart clock and companion platform for a TomTom ONE (v6). It can work as a standalone bedside or travel clock, a desk clock paired with Windows, or a remote glance display connected through the optional Linux hub. The TomTom provides a glanceable 320×240 display and physical interaction; the Windows app provides configuration and PC integrations; the hub provides optional always-on services and remote access.

This README describes the product scope. The implementation design is now in [Architecture](ARCHITECTURE.md) and [Module specifications](MODULES.md). Follow [TASKS.md](../TASKS.md) for checked design deliverables and implementation progress. This document is the long-term product vision. See the repository README for the implemented release and its limitations. V3 uses fresh application code on OpenTom; the original face artwork is reused at the owner’s request.

The detailed architecture supersedes tentative choices below. It specifies 23 modules, explicit worker lifetimes, configuration and wire interfaces, offline behavior, and hardware validation gates.

## Goals

- Turn the TomTom into a customizable desk clock and glanceable companion for Windows.
- Make the Windows app the main place to configure the system, with no configuration interview during installation.
- Keep the device, PC app, and optional hub lightweight through feature modules that initialize only when enabled and needed.
- Make the cracked touchscreen usable through large, simple controls and configurable power-button actions, while verifying actual touch behavior before choosing the final interaction model.
- Prefer local communication and local data handling; make data sharing, monitoring, and integrations explicit settings.
- Keep core clock and connection functions useful when optional services or the network are unavailable.
- Support multiple contexts—bedside, desk, kitchen, workshop, travel, and remote status display—without requiring a Windows PC to be running.

## Product shape

FractureOS has three parts. In the intended deployment, the Linux hub bridges the permanently USB-attached TomTom to Windows and remote clients. The TomTom can also run standalone; the hub is optional for offline use. Direct Windows USB operation is a secondary capability requiring driver validation.

### TomTom device (C)

A small C application runs on the TomTom's OpenTom/Linux environment. Its core responsibilities are drawing the active face, handling touch and power-button events, maintaining the device connection, and loading the currently needed feature modules. It should avoid a heavyweight runtime and keep transient data bounded because the device has roughly 30 MB of RAM.

The display uses a 320×240 RGB565 framebuffer. The operating system exposes a touchscreen input device and the power button is already handled through a user-space service. The user reports that touch coordinates are unusable, so all primary navigation ignores position and treats touch as a single switch. Taps advance menu focus, double taps select, and touch holds go back; short, double and long power presses open/select, return home, and open power controls. Raw press/release reliability and kernel power behavior still need validation. Coordinates are never required.

### Windows companion (Go)

A small Go app runs in the Windows notification area (system tray). Its settings window is the control center for faces, widgets, appearance, button mappings, connection, privacy, and optional modules. It should start with only the tray, connection manager, and enabled services active. Settings and features that are disabled should not start collectors or background workers.

The app may optionally collect Windows resource and activity information and share selected data with the device or hub. Collection is disabled until enabled in Settings. When enabled, collectors should run only while a subscribed widget or hub function needs them, use a configurable sample interval, and stop when no consumer remains.

### Linux hub (Go)

An optional Go service on the always-on Linux machine can aggregate system resource readings, provide local integrations, relay updates and configuration, and share selected status with the TomTom and Windows app. The hub's modules are independently enabled. Hub resource monitoring is a first-class optional module, but it should not scan or report data outside the configured scope.

The current lab setup has the Linux host at `192.168.101.114` and the TomTom USB Ethernet endpoint at `192.168.101.115`. Treat these as current development addresses, not hard-coded product defaults. Discovery or configurable connection settings should support other PCs and networks.

## Feature catalog

### Places and operating modes

The device should remain useful without the PC or hub. Profiles select which face, widgets, schedules, brightness behavior, and integrations apply in each context.

- **Bedside clock:** large dimmable time, quiet alarms, sleep/wake schedule, optional sunrise-style gradual brightening if the display supports it, night mode, and a one-action timer. Keep notifications and PC metrics hidden by default.
- **Desk clock:** minimal idle face, optional richer face while the paired PC is active, focus timer, reminders, selected PC status, and quick actions.
- **Kitchen or workshop timer:** large countdowns, recurring interval timers, stopwatch, and clear completion indication. Sound remains optional and off by default.
- **Travel clock:** standalone clock/date, manually selected timezone, optional GPS-derived local time when GPS is available, and a compact battery/power indicator if hardware status is readable.
- **Remote glance display:** receive selected status through a networked Linux USB host/bridge and, when needed, a private network or VPN. The TomTom has no verified wireless interface; a charger alone provides offline operation only. The hub can relay allowed notifications, service health, calendar summaries, or home-lab status without exposing the entire PC or network.
- **Guest or shared-room mode:** temporarily hide private notifications, location, and PC activity while retaining the clock and public widgets.
- **Presentation or focus mode:** freeze the layout to a quiet clock/timer face and suppress noncritical alerts for a configured period.

Profiles can be switched manually from the device menu or automatically by schedule, connection state, PC activity, or hub availability. Automatic switching must be individually configurable and should always have a simple manual override.

### Clock and watch faces

- Minimal desk-clock face with readable time and date.
- Multiple faces and profiles, with a quick way to switch between them.
- Fully editable widget position, size, alignment, color, font, visibility, and refresh behavior within the screen's resolution and memory limits.
- Optional time, date, CPU, memory, network activity, weather, battery/power state when available, GPS-derived location/timezone, PC activity, and notification widgets.
- Smart face behavior: show a clean clock while the PC is idle; optionally show selected details while the PC is active; return to the minimal face after a configurable idle period.
- Offline behavior: retain the clock and last known cached values when the PC or hub disconnects, with clear stale/offline indicators where useful.

### Input and feedback

- Configurable actions for touch, short power press, double press, long press, and, if the hardware event stream supports it, repeated presses or press-duration bands.
- Power button opens an app-style menu from clock mode. Menu entries and ordering are configurable in the Windows app.
- Large, high-contrast touch targets; no keyboard or precise gesture requirement.
- Coordinate-free navigation is the default throughout; optional scan selection supports limited press/release reliability.
- Disable routine click, tap, and notification sounds by default. Keep only necessary feedback, with separate controls for visual, audio, and vibration feedback if the hardware supports them.

### PC and hub companion

- Windows system-tray icon for connection state, quick actions, and opening Settings.
- Optional PC resource monitoring for CPU, memory, network throughput, and selected process/app activity.
- Hub resource monitoring and reporting for configured machines/services.
- Configurable PC status widgets and compact alerts/reminders on the TomTom.
- Focus/Pomodoro timer, countdowns, recurring reminders, and simple desk routines.
- User-configured quick actions for supported PC operations, with an explicit confirmation step for disruptive actions such as shutdown or closing applications.
- Optional local integrations and plug-in-like providers for weather, calendars, media status, and other services.
- Optional home-lab or home-automation status tiles through explicitly configured hub integrations; control actions should be separately enabled and limited to a user-defined allowlist.
- Remote commands from the clock to the hub should use paired identities, encrypted transport when crossing an untrusted network, replay protection, and explicit authorization for any action that changes device or service state.

### Media and audio experiments

- Spotify and other compatible Windows players expose status through an optional native Windows media-session worker. Remote account-based Spotify support is a separate official API adapter gated on current access and authorization requirements.
- Playback controls may be considered through a supported integration, with only the currently selected media module active.
- Audio forwarding from Windows to the TomTom is an experiment, not a baseline promise. It needs measured bandwidth, codec, latency, audio output, and reconnect tests before inclusion; the device's age and USB/network path may make it impractical.
- Keep all media integration disabled until configured and requested by a face or action.

### Display extension experiments

- A status/companion display that renders FractureOS widgets is the core display feature.
- Mirroring or extending the Windows desktop is an advanced research track. A real Windows virtual monitor requires an Indirect Display Driver and a frame transport/decoder path. It should be treated as a separate driver project with signing, installation, performance, compatibility, and recovery requirements.
- Start any display-streaming experiment with a low-resolution, low-refresh prototype. Do not make the clock or hub depend on it.

### Updates and recovery

- A Windows- or hub-managed updater can deliver versioned device and app releases, verify integrity, retain a known-good version, and report update status.
- Device updates should be staged on SD storage, checked before activation, and include a recovery path that restores the last working FractureOS release. The exact install/rollback mechanism must be designed after inspecting the boot and storage layout in detail.
- The updater must not overwrite stock OpenTom components as an implicit side effect. The reviewed v3 install procedure should state every file it changes and how to restore the previous setup.

## Lazy module model

“Lazy” means more than hiding a setting. Disabled features do not start collectors, timers, network polling, or worker processes. Enabled features load when a face, user action, schedule, or configured integration needs them; they stop and release resources when no consumer remains, subject to a short cache or shutdown grace period where that avoids churn.

- Keep a small always-on core for clock rendering, input, configuration, and connection health.
- Represent optional capabilities as modules with declared dependencies, settings, and resource lifetimes.
- Start collectors on demand and share one collector among multiple widgets that use the same data.
- Use low-frequency updates for static information and configurable refresh rates for live metrics.
- Use separate executable workers for optional features on C and Go components, with private bounded IPC. Stop workers when their final consumer leaves so executable mappings and allocations are reclaimed. A stopped collector compiled into a monolith does not satisfy full unloading. Shared rendering/input/clock primitives remain core. Go runtime plugins are unsupported on Windows and cannot unload.
- Measure idle RAM, CPU, network traffic, and wakeups on each component. Define numeric budgets during implementation after a working baseline is measured.

## Communication and data flow

The Windows app and Linux hub coordinate enabled services. The TomTom reaches the network through USB Ethernet to a host; it is not independently attached to LAN or Wi-Fi. V3 uses its own versioned protocol and does not depend on prior prototype services.

Proposed communication behavior:

- Use a versioned request/event protocol over a persistent local connection where practical, with a simple reconnect path and bounded messages.
- Exchange only the data needed by enabled widgets and actions. Keep PC monitoring, activity sharing, location, and media status independently configurable.
- Treat the Windows PC as the authoritative source for PC metrics and settings; the hub may relay or aggregate only when configured.
- Cache the last safe display values on the device and clearly indicate disconnected or stale data.
- Authenticate paired peers and restrict control commands to an explicit allowlist. Provide a local pairing/reset flow in Settings.
- Support a direct PC-to-TomTom mode if the hub is disabled, and a hub-assisted mode if enabled.
- Support standalone mode with no network dependency, local LAN mode, USB tether mode, and optional remote mode through the Linux hub. The remote mode should be off until explicitly configured.
- For remote access, prefer a user-managed VPN or another authenticated private network path. Do not expose a raw device control port directly to the public internet.
- Queue only small, non-sensitive status updates while disconnected; expire old alerts and commands instead of replaying stale actions after reconnect.

The architecture selects TLS WebSocket control between modern hosts and authenticated, bounded framed TCP over the dedicated USB link. Remote TLS/VPN terminates at the modern Linux host. See the architecture for pairing, message limits, subscriptions, freshness and compatibility rules.

## Settings and user experience

All questions about behavior belong in the Windows app's Settings. Installation should use sensible defaults and avoid a setup wizard that blocks launch. The app should provide:

- Device connection and pairing status, with manual address entry and reconnect controls.
- Feature/module enable switches and a short explanation of the data and resources each uses.
- Face editor for widget selection, positions, sizes, styles, refresh cadence, and active/idle rules.
- Button/touch mapping editor with a test view and fallback navigation selection.
- Separate privacy controls for Windows monitoring, hub reporting, location, notifications, and media integrations.
- Separate profiles for bedside/quiet, desk, travel, shared/guest, and custom use, with a manual override and configurable automatic profile rules.
- Remote-access settings that show the active path, paired peers, shared data categories, and a one-step disconnect/revoke control.
- Quiet-by-default feedback settings and explicit controls for notifications and sounds.
- Update channel, update status, and recovery instructions.

## Feasibility and known limits

Inspected TomTom facts: ONE (v6); Linux 2.6.13; ARM926EJ-S / ARMv5TEJ; approximately 30 MB reported RAM; 320×240 RGB565 framebuffer; SD storage; USB Ethernet; touchscreen event device; power-button handling; GPS-related hardware reported present. The device currently boots an OpenTom environment from SD. The actual behavior of touch coordinates on the damaged display still needs an event-level test.

Practical baseline candidates: standalone clock and widget renderer, button-driven menus, USB communication through a host, PC/hub status, low-rate resource metrics, weather via the host, reminders/timers, profiles, and simple supported media metadata. Bedside alarm scheduling and offline clock operation should not require the hub.

Higher-risk experiments: audio forwarding, continuous high-rate monitoring, rich animation, arbitrary plugin loading on-device, and true Windows extended/mirrored display. These need separate prototypes and measured resource/latency results. Screen resolution, input reliability, legacy kernel/toolchain support, and the tiny memory budget constrain UI complexity and feature concurrency.

## Proposed delivery stages

1. **Hardware and environment validation:** confirm touch event coordinates and button timing; inspect available C libraries, framebuffer access, audio/GPS interfaces, and update/recovery constraints.
2. **Core clock:** clean C device app, basic Go Windows tray/settings app, pairing/connection, minimal face, and input fallback.
3. **Modular companion:** widget/face configuration, lazy module lifecycle, PC and hub status providers, timers, and privacy controls.
4. **Polish and updates:** accessibility/readability, offline behavior, quiet defaults, logs/diagnostics, versioned updates, and tested rollback.
5. **Optional integrations and remote use:** profiles and schedules, hub relays over a private network, home-lab/service status, and remote notification summaries, each with explicit pairing, privacy, and reconnect behavior.
6. **Optional research tracks:** Spotify integration, audio forwarding, and virtual-display streaming/driver work, each evaluated independently before becoming a supported feature.

Each stage should be tested on the actual TomTom. Device-side performance budgets and the final input model should be based on measured results rather than assumed specifications.

## Research references

- [OpenTom project and build guidance](https://github.com/ElectronicsClubAltona/OpenTom)
- [TomTom ONE product specifications](https://download.tomtom.com/open/manuals/one/refman/1_EN_UK/TomTomONE_manual.htm)
- [Microsoft Indirect Display Driver overview](https://learn.microsoft.com/windows-hardware/drivers/display/indirect-display-driver-model-overview)
- [Microsoft notification-area guidance](https://learn.microsoft.com/en-us/windows/win32/shell/notification-area)
- [Spotify Web Playback SDK](https://developer.spotify.com/documentation/web-playback-sdk)
- [Go plugin package platform limitations](https://go.dev/pkg/plugin/?m=old)
