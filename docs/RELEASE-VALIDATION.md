# v0.1.5 release validation — 2026-10-26

This is an initial application release on the owner's ONE (v6), not a new kernel or a completed implementation of all planned modules. Evidence backups and raw captures remain in the ignored `artifacts/` directory and the host's private backup directory. Only selected UI screenshots and summaries are published.

## Device evidence

- Compiled with the validated GCC 3.3.4/glibc 2.3.2 ARMv5 legacy SDK. The final stripped C app is approximately 33 KB; dynamically linked against the device's libc.
- Every deployed payload was uploaded to temporary storage, exported back through the diagnostic helper and compared with the local SHA-256. Startup, `/etc/rc` and the original firmware file were backed up; firmware was not changed.
- All nine face assets selected and rendered on the real framebuffer. The selected asset loaded successfully. Earlier C core/face checks ran both natively and as ARM executables on the actual device, including parsing bounds, settings persistence and reset behavior.
- The new larger Nunito face was activated on the device. Rendered transition checks show stacked/horizontal layouts, hour-only display for minute 00, full time returning at minute 01, and blinking versus steady colon. The same C face behavior checks ran on the ARM device before the request to stop adding test scripts.
- Subsequent UI/theme changes were checked manually. [All nine actual device screenshots with a running timer](images/faces-with-timer.png) show the composition above the separate information band. Custom theme fields, save-as, optional face recoloring and returning to the original palette were exercised. [The themed menu](images/custom-theme-menu.png) was captured from the framebuffer.
- Menu actions, profile changes, timer completion and visual-alarm configuration were exercised through synthetic control events. This is not full physical cracked-panel or power-button acceptance.
- The final v0.1.5 device app measured **992 KiB RSS**. This is a point measurement, not a long-duration maximum. The clock has a fixed 50 ms button poll; it is not a zero-wakeup design.
- Restore/reinstall was rehearsed without reboot. The first live restoration exposed zombie processes retained by the stopped original startup shells: BusyBox `pidof` counted them and suppressed Nano-X startup. The restore utility now resumes the preserved chain so it can reap its children, while pausing the extra fallback supervisor. A live, non-zombie Nano-X process was then observed. Reinstallation returned to the healthy v0.1.5 clock with user face/theme settings intact.

The boot entry and supervisor are installed for future boots, but a cold reboot/USB reconnect and 24-hour soak were not performed. Small stopped legacy supervisors may remain until a planned reboot. RTC retention, suspend wakeup, physical long-press shutdown, audio and GPS use are not established by this release. The PWM device admits one opener: a second probe while the app owns it reports unavailable, so independent effective-brightness verification remains open.

## Host evidence

Linux x86-64 host installation, CLI, uninstall/reinstall and user-service startup were exercised. A fixture archive was verified against its release checksum before installation. The hub successfully reads and controls the real TomTom via pinned TLS → bounded broker → USB Telnet → local C socket. An unauthenticated hub request returned 401. Settings returned redacted saved credentials, rejected a missing session token with 401, and served face/layout/theme controls. JavaScript syntax was checked.

The optional Linux metrics worker was observed while Desk mode was visible (9,024 KiB RSS) and was absent five seconds after returning to Minimal mode. The hub measured 12,912–14,104 KiB RSS during these samples. Resource readings are current process samples, not performance guarantees. Configuration credentials were kept out of repository files and command output. The hub is bound to the requested private host address `192.168.101.114:47831`.

Go validation performed during development included race-enabled unit checks and vet; C checks used address/undefined-behavior sanitizers before the later preference for manual UI verification. No recurring tests or monitoring jobs were installed. The newly introduced Python release-test harness was removed at the owner's request; existing platform diagnostic utilities remain available.

Release host binaries are built with the verified official Go toolchain recorded in `HOST-TOOLCHAIN.json`. Linux ARM64, Windows x64 and Windows ARM64 cross-builds completed. **Only Linux x86-64 was run here.** Windows installer/tray behavior, certificate pairing from a real Windows session and Windows resource usage still require Windows acceptance. Windows metrics, media, audio, virtual display drivers and automatic activity switching are not implemented.

## Release integrity and limits

The release contains four host archives, a checksum list and build metadata. The Linux archives include the compatible TomTom payload and deployment utility. No boot firmware, map files, user settings, pairing credentials or private backups are published. SHA-256 detects corrupted downloads; signed release manifests and a complete interrupted-update journal are not implemented.
