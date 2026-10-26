# v0.1.8 additions

The C app now uses small header modules for page navigation, read-only button sampling/power actions, and bounded media/status state. CPU/RAM symbols, filesystem usage and default-route network rates share the optional Linux collector. The Windows media worker uses native WinRT COM interfaces on a dedicated MTA thread and Windows Core Audio output-volume stepping. It posts updates to `/api/media` through the same pinned TLS/bearer channel; no Spotify developer API remains. Artwork is a fixed 96×96 RGB565 buffer allocated only on the player screen and freed when leaving it. Media becomes stale after 20 seconds and actions expire after 10 seconds. The Windows tray checks enabled demand every three seconds; active media polling is every two seconds. Disabled sharing makes no media API calls and launches no worker.

The live button indicator uses a read-only GPIO page mapping only after matching model 19 (Casablanca) and CPU 4 (S3C2412). GPF0 input is active low, at physical 0x56000054; its configuration must be input/EINT. No GPIO register is written. Other hardware falls back to completed-release ioctl durations without a live indicator. SDK sources: `tomtomgo-iocasablanca-*.h`, `tomtomgo-type.c`, `regs-gpio.h`. Suspend writes `mem` to `/sys/power/state`; restart/shutdown use the kernel reboot API. These power actions were not executed during validation.

Windows media ABI reference: [Microsoft Windows SDK generated bindings](https://github.com/microsoft/windows-rs/tree/master/crates/libs/windows/src/Windows/Media/Control), [media-session usage](https://devblogs.microsoft.com/oldnewthing/20231108-00/?p=108980), [Core Audio endpoint volume](https://learn.microsoft.com/en-us/windows/win32/api/endpointvolume/nn-endpointvolume-iaudioendpointvolume). The four Windows vet unsafe-pointer diagnostics occur at foreign COM vtable/HSTRING/buffer addresses. Their ownership and bounds were reviewed; cross-compilation cannot establish runtime ABI correctness. A real Windows check remains required.

# v0.1.7 implementation

This describes the code that exists. ARCHITECTURE.md and MODULES.md describe the broader target design; their full registry, wire protocol, face composition model and signed updater are not implemented yet.

## Device

`device/src/fracture.c` owns `/dev/fb`, touchscreen events, the Barcelona button-duration snapshot ioctl and PWM brightness. It takes a process lock, exposes a root-only Unix control socket and copies the small framebuffer only when an actual redraw is needed, without retaining a second shadow framebuffer. `faces.h` parses bounded data-only face files and loads one alpha atlas. The old Nano-X application and information sidebar are not dependencies. The selected face, profile, theme, brightness, UTC offset and visual alarm settings are saved through temporary files and rename. FAT durability across sudden power loss is not guaranteed by this mechanism.

The minimal face redraws on minute changes or input/configuration. Visible seconds, a blinking colon, timers, stopwatch and desk view update once per second. Menu selection transitions run for 180 ms at the existing polling cadence, then stop. A 45.7 KB UI text atlas supplies antialiased mixed-case text without a font engine. A 50 ms poll remains necessary for the custom power-button snapshot driver; this is not interrupt-only idle behavior. Clock/menu/timer functionality is currently compiled together, while the large atlas is allocated only when selected. Optional integrations are absent from this release rather than dormant background services.

Power uses the kernel's non-destructive button-event snapshot rather than stealing the shared hardware status flag. The touch handler uses BTN_TOUCH and pressure only, debounces very short contacts, and recognizes single/double/held presses. Physical long-press power policy and a noisy cracked panel still require user hardware acceptance.

`boot.sh` stays in the foreground because the existing `/etc/rc` forces a reboot if its application chain returns. Repeated short-lived failures return to the preserved OpenTom startup. During live replacement, old foreground startup shells are stopped, not killed, to keep that init chain alive. These small stopped processes and unreaped old children remain until a planned reboot; no reboot was performed. The new startup entry is installed for next boot.

## Host

Go standard-library code serves an authenticated, pinned-TLS API, serializes commands to the device and briefly caches status. For this first release, the final hop uses the existing root Telnet connection on the trusted USB link. The C/Go framed HMAC protocol described in the design is future work. Do not expose root Telnet or this early hub directly to the Internet; use the owner's private network for remote access.

Linux resource collection runs as a separate child process only when `hub_metrics` is enabled and the Desk screen is visible. The process is cancelled and reaped when its consumer disappears. Windows collection is not implemented. The tray blocks on the native Windows message loop; it does not run polling collectors. Settings launch as another process serving a session-protected localhost page and opening the user's browser; the settings process expires after five idle minutes. This is not the originally proposed fully native Studio or a drag-and-drop widget editor.

## Installation and updates

Linux installation is per-user, with a versioned host release directory, launcher and optional systemd user service. Windows uses a PowerShell/WinForms installation window, per-user files, a Start menu shortcut and HKCU uninstall registration. Windows installation does not change machine-wide execution policy; its uninstaller uses a process-scoped policy option. No Windows drivers or elevated install are needed for these features.

Downloads use HTTPS and release checksums. Device uploads are staged via a temporary, tokenized HTTP endpoint on the USB host, then read back and SHA-256 verified. The installer preserves startup and firmware files as private host backups; it does not write firmware. New binaries occupy a new release directory. Activation is health-checked; a failure after replacing startup attempts restoration. These checks are not a signed update chain, A/B firmware system, complete activation journal or interrupted-power guarantee. Retrying a partially staged release may require inspecting the abandoned slot; it will never overwrite mismatched same-version executables.
