# Current installation — v0.1.11

The owner's requested v0.1.11 changes are installed on the TomTom; matching Linux production hub is active. There is no pending device application. Do not redeploy either old preview.

- Device slot `/mnt/sdcard/fracture-v3/releases/v0.1.11`; every payload and root icon verified by SHA-256. Backups and report: ignored `artifacts/deploy-v0111-final`.
- Production host launcher points to `~/.local/share/fracture-v3/releases/v0.1.11`. One hub listener at `.105:47831`; transient preview hub stopped. `.114` remains the TomTom LAN alias and `.115` the USB address.
- Latest observed owner settings: Desk, Amber, Nunito, brightness 100, UTC offset 120, alarm off, telemetry **off**, unusual-only weather, notifications/media on, notification duration 10 seconds, menu timeout 15 seconds. Read fresh state before future changes; do not restore these values automatically.
- Checks/evidence: `artifacts/manual-v0111` and docs/RELEASE-VALIDATION.md. Includes timed lyric boundary/pause/seek and live Windows-position webhook checks, telemetry grace, Quick settings/Clock navigation, startup animation and exact C home-preview pixels. Temporary lyric overrides were cleared and telemetry preference restored.
- Accepted Windows build/run: 40499752919 at source `27eb8a3fa2e5c0fcd41b122373cbde69f8e9263c`; local artifacts `artifacts/windows-v0111-accepted`. Both architectures built; only x64 executed. New Windows app is needed for millisecond media samples and the local preview.
- Remaining: owner-PC visual preview/PawnIO acceptance, ARM64 runtime, full cold boot/power/suspend tests, earlier bootloader/kernel logo replacement and long-duration operation. The application startup animation is implemented; firmware graphics were not replaced.
