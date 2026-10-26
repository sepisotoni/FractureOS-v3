# FractureOS v3 module specifications

Design status: specified, not implemented. See `ARCHITECTURE.md` for common lifecycle, protocol, budgets and error handling; see `../TASKS.md` for progress. Modules are capability boundaries: small shared clock/render/input primitives stay in the core; optional integration and application code uses separately launched workers. Every optional module is absent until enabled and leased. An unsupported capability displays a reason and creates no worker.

## M01 — Display and scene renderer

**Owner:** C shell. **Inputs:** active scene, validated assets, topic values. **Output:** RGB565 framebuffer regions. Core enabled always while awake. Support text, digits, analog clock hands, shapes, icons, bounded sparkline/ring/bar widgets, image backgrounds and badges. No HTML, arbitrary scripts or video codec is required. UI motion defaults off; the displayed face determines redraw deadlines. Fonts/assets for inactive faces stay on storage.

**Acceptance:** render recognizable 320×240 templates in simulator and on device; obey actual pitch/offset; freeing a face releases its asset arena; oversized scenes fail before activation; no redraw loop for a static minute clock.

## M02 — Faces and widgets

**Owner:** C scene runtime plus Studio compiler. Include Minimal (large time/date), Desk (clock with optional system widgets), Bedside (dim time/alarm indicator), Workshop (large timer), Travel (time/offset), Status (hub tiles), Media and Guest templates. A widget binds to a typed topic; two widgets sharing a source acquire one collector through leases. Dynamic widgets support label/unit/precision, thresholds and named actions. Layouts allow coordinates, sizes, layering, palette, font, border and show/hide conditions from an allowlisted expression set (comparisons and boolean operators, no scripting).

**Defaults:** Minimal loaded, seconds off; all imported layouts are bounded and include a visible time or accessible home action. **Acceptance:** edit → preview → apply → reconnect preserves face; hidden/inactive widgets release subscriptions; unsupported widget shows a placeholder without loading its provider; export/import round-trips all editable properties.

## M03 — Switch input, launcher and accessibility

**Owner:** C core with a hardware adapter for touchscreen and power events. Touch x/y are discarded. Launcher shows a short scrollable list with icons, highlight and gesture hint. Apps can be reordered, hidden or pinned in Settings. Support single/double/hold gestures and optional timed scan selection; use architecture defaults. Local menus cover profile, brightness, time/offset, alarm, timer, connection and power. Confirm disruptive actions with a separate explicit selection; a held button cannot confirm itself.

**Acceptance:** navigate every primary screen with press events only; simulate bounce, missing release, accidental long hold and double press; perform no duplicate action; recovery/home remains usable after an app crash. Coordinate mode never becomes a requirement.

## M04 — Time and scheduled events

**Owner:** core deadline scheduler; editor/configuration worker on demand. Store UTC time with timezone transition table. Local manual time/offset is supported without PC. Display validity and sync age in diagnostics. The scheduler retains only next deadlines and small alarm records, not an always-running alarm UI. Alarm volume/sound is separate from routine feedback and default visual-only, clearly labeled in the alarm editor.

**Acceptance:** offline restart with valid/invalid RTC; host clock changes; DST missing/repeated hour; expired timezone table; missed alarm policy; no duplicate alarm after rollback. Do not advertise wake from suspend or power-off unless P006 demonstrates it.

## M05 — Profiles, presence and quiet behavior

**Owner:** C core policy engine, optional Go presence worker. Profiles: desk, bedside, kitchen/workshop, travel, guest/shared room, focus/presentation, remote status and custom. Each selects scene, allowed widgets, brightness schedule, notification visibility and input presets. Bedside suppresses PC metrics and ordinary notices; guest redacts private text and thumbnails immediately; focus fixes the face until expiry/manual exit. Optional gradual bedside brightening uses scheduled brightness steps only after brightness support is proven; no ambient sensor is assumed.

**Defaults:** manual profile switching; automatic schedules and PC-driven face changes are toggles. Presence opt-in uses active/idle/locked/disconnected states without transmitting key contents or window titles. Manual override and guest filtering follow architecture precedence.

**Acceptance:** correct precedence with overlapping rules; no profile flapping; Windows disappears and clock falls back; guest mode clears visible/private caches; profile switch releases unused modules. Bedside works with Windows off and USB network absent.

## M06 — Timers, focus and reminders

**Owner:** optional C app worker; compact scheduled state survives in core. Countdown presets (1/5/10/25 minutes), stopwatch with bounded lap list, configurable work/rest intervals, Pomodoro (25/5 default), one-shot/weekday alarms, snooze (9 minutes default), and text reminders. Configure new presets in Studio; use/edit essentials through switch menus. No sound unless enabled per alarm/timer or profile.

**Acceptance:** timers remain due when their app closes; monotonic countdown survives host clock correction; restart handles unknown time honestly; simultaneous deadlines become a bounded queue; snooze/dismiss are reachable without coordinates. A scheduled timer is a valid ongoing consumer, not a hidden background process.

## M07 — Power, brightness and feedback

**Owner:** core power adapter and on-demand C sound worker. Capabilities report brightness range, external power/battery readability, suspend support and audio availability. Offer night dimming, scheduled display off/tap to wake if supported, battery badge and explicit suspend. Use actual known driver interfaces, not guessed ioctls. Battery percentage is absent if only voltage is available. No assumed vibrator or light sensor.

**Defaults:** tap/click/notification sound off; visual action acknowledgement; no automatic suspend with armed alarms. **Acceptance:** unavailable controls explain why; no suspend triggered by normal menu press; audio stops/releases device at end; brightness restored after display module exit; show degraded alarm capability if output unavailable.

## M08 — Connections, remote access and pairing

**Owner:** C link worker + Go hub/tray. Default path Windows → Linux `.114` → USB TomTom `.115`. Offline clock has no connection lease. Connected mode acquires a link lease with bounded retry; all data carries freshness. Pair/revoke through Settings. Private-network remote administration and hub relay use authenticated TLS; USB authentication and its physical trust assumptions are documented in architecture.

A physically remote online TomTom needs another Linux USB host/bridge; a USB charger alone is offline. A browser on a phone can configure the original connected device through the optional hub UI. Relayed peers get explicit scopes (view state, edit faces, administer updates, invoke selected actions). No command replay after reconnect. Direct Windows USB is an alternative only if drivers work.

**Acceptance:** unplug/replug, hub restart, wrong key, wrong protocol, expired lease, revoked peer and remote VPN disconnect. Clock/timers continue; stale data is labeled; no actions cross scope; changes from concurrent/offline editors produce revision conflicts.

## M09 — Windows tray and Settings / optional hub UI

**Owner:** Go. Tray menu: connection summary, open Studio, select profile, pause sharing, modules, diagnostics, exit. A native Go/Win32 message loop owns the icon and restores it after Explorer restart. Settings/editor is a separate process; user chooses launch-at-login in Settings. No setup interview. Local clock features require no login/account. Hub UI is optional and offers equivalent validated configuration actions for Linux/phone access.

**Acceptance:** closed Studio has no editor process; disabled collectors do not start when opening unrelated settings; Windows lock/logoff/sleep transitions are handled; pause sharing terminates relevant leases immediately; tray exit closes user-session workers and reports disconnection.

## M10 — Face Studio and asset tooling

**Owner:** Go Studio; portable C rendering preview helper. Toolbox, layered canvas, properties and live device preview are separate panes. Support drag/resize and numeric geometry, snapping, alignment, fonts/colors, conditional visibility, source/topic selection, action bindings, undo/redo, duplicate face, templates and pack export/import. Convert images/fonts on the host, downsample to device limits, and show estimated asset memory before apply. Preview makes temporary subscriptions only while open; changing a preview does not persist until Apply.

**Acceptance:** all M02 properties editable; render parity with device; cancel leaves current revision intact; reject corrupt assets; bounded undo history; closing preview removes its leases; no executable or secret accepted in a face bundle.

## M11 — Windows presence and selected app activity

**Owner:** Go Windows worker. Subscribe to lock/session events and read session idle state with Windows APIs; sample idle at two seconds only if an active profile rule consumes it. Send only state by default. Optional foreground app identity uses a separate permission; titles and keystrokes are excluded. Track each paired machine independently and choose a primary device in Settings.

**Acceptance:** active/idle/lock semantics match the logged-in session; wrap/time anomalies do not flip presence; no presence polling when rule disabled; resource monitoring remains off while presence is on. API behavior: [GetLastInputInfo](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-getlastinputinfo).

## M12 — Resource metrics and optional reporting

**Owner:** separate Go Windows and Linux workers. Common topics: aggregate/per-core CPU, memory used/total, network rates, disk space/I/O where supported, uptime. Linux reads procfs/sysfs; Windows uses OS counters/APIs rather than launching PowerShell per sample. GPU/temperature providers are extra capability gates and remain absent if unavailable. Windows metrics default disabled; hub metrics enabled as available but collect only with a visible widget/reporting lease. History and threshold reporting are explicit background consumers, default off.

**Defaults:** visible metrics two-second cadence, inactive graphs stop, optional history 60-second samples retained seven days with 16 MiB quota. **Acceptance:** CPU/rate deltas use actual elapsed intervals and reset correctly after reconnect/counter wrap; no disk history without opt-in; multiple widgets share collector; missing sensor never appears as zero; Windows off does not stop hub metrics.

## M13 — Weather

**Owner:** Go hub integration worker. First adapter: Open-Meteo HTTPS forecast with cached normalized temperature, condition and next forecast; verify current service terms before shipping. Manual location and timezone set through Settings; optional GPS location feed requires separate permission and valid fix. No IP geolocation by default. Cache on hub, send compact selected values, default refresh 30 minutes only with a weather lease; invalidate as stale after two hours. Credentials, if later needed, remain on host.

**Acceptance:** manual location works without GPS; HTTP failures/rate limits back off; stale weather labeled; enabling unrelated clock feature makes no weather request; service/schema errors never crash renderer. No continuous retry when no consumer.

## M14 — Calendar, notifications and notes

**Owner:** Go integration workers plus C notice view. Local notes/reminders and imported iCalendar files first; optional configured HTTPS calendar subscription next, refreshed every 15 minutes with a consumer. Restrict initial recurring calendar support to explicitly supported recurrence forms; flag unsupported events rather than invent occurrences. Normalize upcoming event times and redact private fields in guest mode. Generic paired `notify` endpoint permits integrations to publish scoped notifications with source, priority, expiry and optional registered action.

System-wide Windows notification capture is a separate opt-in adapter and capability/permission gate. Start with FractureOS/integration-generated notifications; do not pretend all OS notifications are already accessible. Default ordinary notifications silent, TTL five minutes, maximum 20 items. Calendar all-day/date-only events preserve local dates.

**Acceptance:** timezone and recurrence fixtures, sensitive notice redaction, expired notices discarded, source allowlist, no unwanted replay, bounded queue. Shared-room profile shows count/public items only; calendar worker unloads without a consumer or explicit reminder rule.

## M15 — Service status, automation and quick actions

**Owner:** Go hub/action workers. Configured HTTP/TCP health checks every 30 seconds with short timeout and concurrency cap; inspect only listed targets. First home automation adapter: user-configured Home Assistant endpoint/token with chosen entities and service actions, gated on current official API review at implementation. No scanning or automatic control discovery. Windows actions: media controls, lock screen, configured app launch, selected volume controls; shutdown/close-app require explicit confirmation.

Actions use ID + validated arguments + target + expiry; programs are configured absolute paths with fixed argument templates, never remote shell strings. Home automation writes are separately enabled per action. A prompt-approved action cannot be triggered again by a held switch or repeated packet.

**Acceptance:** timeout/offline status shown; read-only pairing cannot invoke actions; guest restrictions apply; disruptive confirmations work without coordinates; lost response produces unknown status and no blind retry. Widgets may view health with control disabled.

## M16 — Now playing / Spotify

**Owner:** Go Windows media worker, optional independent Go hub Spotify provider. Prefer [Windows Global System Media Transport Controls](https://devblogs.microsoft.com/oldnewthing/20231108-00/?p=108980) for compatible local Spotify and other players: title, artist, state, timeline, artwork and supported play/pause/next/previous controls. It is designed for companion-device scenarios. Downsample artwork on host; interpolate progress locally between timeline events; no constant artwork fetch.

Remote/account Spotify support uses official Web API authorization only if current access/account rules permit; implement PKCE/token refresh on host and scope only needed features. Verify current rules before implementation; no assumption that development API access or Premium capabilities are universally available. The Web Playback SDK from the original README is not the default path and is not a TomTom decoder.

**Acceptance:** real Windows Spotify session reports metadata, controls reflect supported capabilities, player exit clears state, switching sources cancels prior subscription, disabling removes worker/token refresh. API refusal is reported as unavailable; do not bypass content protection. Media status requires no device audio streaming.

## M17 — GPS / travel information

**Owner:** C worker with inspected device GPS interface; host computes optional location-to-timezone mapping. Reuse proven kernel/proprietary driver interface where necessary, but write fresh v3 parsing/application logic. Parse bounded valid NMEA or the discovered output format. Expose fix age, satellite/fix status and optional position/time. Do not infer a valid fix from a running daemon. Default off for desk/bedside; no background GPS battery drain.

**Acceptance:** no-fix indoors, corrupt data, stale fix and known date/week rollover anomalies; position remains private unless enabled; invalid GPS never silently resets system clock; worker and any owned driver process stop after lease ends. Travel can use manual offset offline; full maps/navigation are outside the current feature list.

## M18 — Settings, storage and extensibility

**Owner:** core snapshot parser, Go config service and on-demand editors. Versioned schemas for global settings, module manifests, profiles, faces and schedules. Validate locally and at authority before commit. Export nonsecret settings/face packs, preserve previous revision and support reset per category. Defaults: minimal face, silent feedback, coordinate-free navigation, no Windows metrics/sharing, manual profile selection. Hub connectivity can be configured without enabling any cloud integration.

**Acceptance:** corrupt/truncated settings recover previous valid generation; unsupported newer schema fails safely; concurrent edits conflict visibly; imported secrets/executable paths are rejected where not allowed; disabled modules retain only saved settings on disk. New module manifests can add apps/data providers without changing launcher code; new widget primitives require a versioned renderer release.

## M19 — Diagnostics and history

**Owner:** core bounded counters plus on-demand Go diagnostics UI. Show module state/lease reasons, memory, version, connection/freshness, capabilities, failed update and clock validity. Default device log ring 64 KiB RAM, errors redacted; no continuous packet/body logging. Optional host history quotas per M12. Manual support export includes config with secrets removed and capability/version summaries.

**Acceptance:** diagnostics distinguishes disabled/idle/unavailable/failed; viewing it does not load every feature; export contains no tokens, full calendar contents or location by default; log flooding drops old entries; no SD write per UI frame/metric.

## M20 — Update and recovery

**Owner:** on-demand Go host updater and C device helper/supervisor. Follow architecture's signed two-slot application release and startup-health protocol. Updates cover device core/workers, host/tray workers, Studio and schemas with explicit compatible version ranges. Automatic checks off initially; manual Check starts updater, reports available version, then exits. Keep current and last-known-good release. Stage independently from activation; avoid activating while an alarm is ringing or a critical action is pending.

**Acceptance:** signature/hash/architecture mismatch, path traversal, disk full, unplug at each stage, failed health check, config migration failure and recovery from previous slot. A kernel/bootloader update requires a distinct project task. Document FAT corruption limits and host backup restoration; do not call A/B slots a guarantee against all storage damage.

## M21 — Audio forwarding experiment

**Owner:** Go Windows capture worker → hub relay → C audio sink worker. Use [WASAPI loopback](https://learn.microsoft.com/en-gb/windows/win32/coreaudio/loopback-recording) for an explicitly selected output device; capture only while session active. Start with 16 kHz mono signed 16-bit PCM, host resampling, 20 ms blocks (640 bytes), bounded 200 ms jitter buffer, sequence/time stamps and drop-old overflow behavior. Probe actual OSS/hardware output compatibility before choosing final parameters. No microphone recording or raw content storage by default.

**Acceptance gate:** audible demonstration, measured CPU/latency/underruns, audio format support and stable reconnect. Stop on no heartbeat, profile quiet request or session close; alarm sound preempts stream. Protected content is not bypassed; fidelity and latency are measured, not promised. No always-loaded codec engine. Higher fidelity/compression is a later measured option.

## M22 — Window / desktop mirroring experiment

**Owner:** Go Windows capture worker + Go relay + C scene-stream worker. User selects window, display or crop. Begin with low-rate capture via Windows APIs, host scaling/letterboxing to 320×240, RGB565 16×16 changed tiles and 2 fps default, 10 fps experimental ceiling. Full frame is 153,600 bytes: 10 uncompressed frames/s is 1,536,000 bytes/s before overhead; link and CPU measurements decide feasibility. Use separate binary stream, bounded one-frame pending queue and periodic keyframe. Frozen last frame has a visible disconnected label; timeout returns clock.

**Acceptance gate:** actual Windows capture including minimize/resize/lock behavior, measured bandwidth/frame latency and device memory. Closing stream releases capture/render buffers. Tap does not become a guessed desktop mouse coordinate. Mirror mode supports overview/zoom presets; it does not create an extended desktop.

## M23 — True monitor extension experiment

**Owner:** separate optional native Windows UMDF/IddCx driver plus Go host transport using M22. A real additional display requires a driver; this is an explicit native-code exception to the Go host application preference. Follow [Microsoft's Indirect Display Driver model](https://learn.microsoft.com/windows-hardware/drivers/display/indirect-display-driver-model-overview). Plan a logical 640×480 output scaled to the physical panel first; add 320×240 only if accepted by Windows/driver validation. Driver exchanges frames with host transport, and advertises a monitor only when the user enables extension.

**Acceptance gate:** suitable WDK build environment, Windows runtime access, installation permission and a valid signed distribution route. No automatic test-signing/Secure Boot changes. Validate display connect/disconnect, mode changes, scale, capture, sleep/wake and removal without stranding user windows. Driver unload/deactivation has OS-controlled lifetime: report it separately from unloadable user workers. A mocked or mirrored window is never reported as true extension. If dependencies block it, task remains explicitly blocked while ordinary clock/companion releases can proceed.

## Scenario acceptance matrix

| Scenario | Required behavior | Must remain inactive |
|---|---|---|
| Offline bedside with charger | Time, dim schedule, local alarm and snooze; invalid RTC clearly shown | PC/hub collectors, GPS, streams |
| Active Windows desk | Selected detail face after opt-in presence; chosen metrics only | Unselected monitoring/integrations |
| Windows off, hub on | Local clock and enabled hub information continue | Windows-dependent widgets marked offline |
| Kitchen/workshop | Large timer and optional completion sound, tap-only control | Background PC metrics/media |
| Travel on usable battery | Manual time/zone and optional temporary GPS fix | Network retries in offline profile |
| Guest/shared room | Public clock/widgets, private content cleared | Private data subscriptions unless explicitly needed elsewhere |
| Remote administrator | Scoped settings/actions through hub private-network path | Raw TomTom ports on LAN/public internet |
| Physically relocated online unit | USB Linux bridge supplies actual network path | Assumed Wi-Fi/Bluetooth connectivity |
| Media/stream exit | Clock restored and worker memory reclaimed | Capture/audio processes and queues |

Integration adapters must consult current official provider documentation before implementation. These choices describe intended interfaces; they do not imply the accounts, permissions, drivers or hardware capabilities already exist.
