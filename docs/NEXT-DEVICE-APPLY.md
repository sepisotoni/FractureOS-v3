# Device application completed — v0.1.10

The owner reconnected the TomTom and authorized continuation on 2026-10-27. The pending final application is complete; do not redeploy the old preview.

- Current device slot: `/mnt/sdcard/fracture-v3/releases/v0.1.10`. All payloads and the volume `tomtom.ico` passed SHA-256 readbacks. Original startup and icon backups remain available.
- Matching production Linux host v0.1.10 is installed and `fracture-v3.service` is active at `.105:47831`. The transient preview hub is inactive.
- Fresh owner settings were preserved byte-for-byte: Desk, Amber, Nunito, brightness 100, UTC offset 120, alarm off, unusual-only weather, notifications/media/telemetry enabled, notification duration 10 seconds and menu timeout 15 seconds. Read fresh state before future work; these are observations, not values to restore automatically.
- Stored Weather overview/hourly/weekly views were inspected with the hub stopped. Final player/lyrics timeout exemptions, ordinary menu return, rounded notification overlays/expiry/dismissal, and normal live clock were checked. App memory measured 940 KiB RSS.
- Interrupted `/tmp/fracture-install-27dc244b5581` staging was cleaned using explicit payload paths. Successful retry evidence is in ignored `artifacts/deploy-v0110-reconnected`; screenshots/state are in `artifacts/reconnect-v0110`. Captures with real media metadata remain private.
- USB reconnection changed the gadget interface name. The host mapping now selects the known USB port and starts a matching service instance; `.114` reaches the TomTom again. See NETWORK.md. A further physical replug remains an acceptance check.

Remaining hardware acceptance: W06 actual Windows UAC/PawnIO temperature reading, ARM64 Windows runtime, suspend/wake/reboot/shutdown and long-duration operation. Those are not prerequisites for repeating the completed device deployment.
