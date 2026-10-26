# TomTom platform validation — 2026-10-26

This records the earlier read-only platform phase. See [release validation](RELEASE-VALIDATION.md) for subsequent deployment and application work.

## Result

Connected to `192.168.101.115:23` as root over the Linux host's USB Ethernet route. Built fresh v3 C diagnostics with the available legacy toolchain, transferred them to the TomTom, and executed both the binary and shell smoke script there. The final run passed all **17 harness checks**, including deliberately failing operations returning the expected status. The C self-test passed seven checks. Six host-side Telnet regression tests also passed.

This initial phase validated the platform tools before application implementation. No package installation was necessary. No boot/startup configuration was changed and no services were stopped. The diagnostic and smoke script were staged in a unique `/tmp/fracture-v3-check-*` directory; the finalizer removed them. The C self-test removed its own temporary directory. The temporary host HTTP server was shut down after each transfer. `/tmp` was empty at the final follow-up check.

## Tested files and commands

| File | Where executed | Result |
|---|---|---|
| `tools/build-probe.sh` | Ubuntu Linux host | GCC 3.3.4 build and strip passed; repeat build produced identical SHA-256 |
| `tools/tomtom.py` | Ubuntu host, connected to actual TomTom | Login, multiline commands, exit codes, transfers/readback and no-overwrite handling passed |
| `tools/validate-device.py` | Ubuntu host orchestrating device commands | Final full run complete; evidence and backups retained locally |
| `device/diagnostics/platform_probe.c` | Compiled ARM executable on TomTom | Self-test and read-only driver capability inventory passed |
| `device/scripts/platform-smoke.sh` | TomTom `/bin/sh` | Shell builtins/text applets, failure status, C self-test and driver probe passed |
| `tests/test_telnet.py` | Ubuntu host | Six protocol regression tests passed |

Run from the project directory:

```sh
python3 -m unittest discover -s tests -v
sh tools/build-probe.sh
python3 tools/tomtom.py exec 'uname -a'
python3 tools/validate-device.py
```

The build and orchestration scripts are host tools; do not run Python or the cross-compiler on the TomTom. Only the ARM binary and `device/scripts/platform-smoke.sh` run on the device. `FRACTURE_SDK` can point the build script at an equivalent legacy SDK; the default is `/home/sepisotoni/projects/LeddaZ-OpenTom`. No old prototype application code was copied into v3.

## Device evidence

| Item | Observed value / outcome |
|---|---|
| Kernel | `2.6.13-LeddaZ`, ARMv5TEJ |
| libc / loader | glibc 2.3.2, `/lib/ld-linux.so.2` |
| Toolchain | legacy ARM GCC 3.3.4 + glibc 2.3.2 SDK, executable successfully run |
| Probe size | 10,728 bytes |
| Probe dependencies | `libc.so.6` only; existing loader and libc suffice |
| Probe RSS / virtual size | 424 KiB / 1,384 KiB in self-test; not an OS resource budget measurement |
| Primitive checks | 32-bit ABI, root identity, random-device read, monotonic timer, anonymous mapping, fork/socketpair/poll/wait, temporary write/fsync/rename/read |
| Framebuffer | 320×240 visible, 320×480 virtual, RGB565, stride 640, memory 307,200 bytes, offsets 0/0 |
| Framebuffer access | Read-only mmap succeeded; no pixel writes or page flipping attempted |
| Touch | Input device opened; pressure range 0–1023; no gesture validation or coordinate use |
| Button | Non-consuming duration snapshot ioctl works; sequence/duration 0 at this inspection |
| Brightness | Current level 30; read-only query only |
| Battery | Approximately 4.13 V, driver reports charge state 2 (charging); no runtime/capacity claim |
| RTC | Read ioctl succeeded and reported 2026-10-26 UTC time; retention/wake not tested |
| Device flags | Bluetooth, audio recording and light sensor each reported 0 |

The ELF has legacy ARM flags `0x602`, OS/ABI ARM and the old loader; its syscall path matches the legacy kernel. Modern `readelf` labels some historical flag bits confusingly. Do not interpret its “GNU EABI” text as proof that modern arm-linux-gnueabi/glibc binaries run here. The tested legacy binary is the compatibility evidence. `docs/TOOLCHAIN-PROVENANCE.json` records SDK archive/header/source hashes; this is provenance, not a complete hermetic toolchain lock.

`/dev/urandom` returned bytes; that verifies API availability only, not sufficient entropy immediately after cold boot. Temporary file checks ran on the RAM root filesystem, not FAT, so they prove neither power-loss persistence nor updater recovery.

## Missing utilities and workarounds

`command -v` itself is unavailable in this BusyBox shell. Executable lookup therefore checks known directories with the `test -x` builtin. No executables were found for `id`, `printf`, `timeout`, `sha256sum`, `md5sum`, `base64`, `tar`, `dd`, `od`, `hexdump`, `tail`, `stat`, `find` or `readlink` in the inspected device paths.

Available and exercised: `/bin/sh`, `test`, `echo`, `basename`, `grep`, `sed`, `cut`, `wget`, `wc`, plus `head` in the inventory. Use `head -n N`, not `head -N`; do not assume `grep -E` or `df -h` support. Plain `df -k` works.

- Transport: standard-library Python Telnet parser on host, with negotiation, bounded output, EOF handling and command completion markers.
- Upload: device `wget` fetches one file from an ephemeral host HTTP server restricted to the device source address and a random route. Existing target files are rejected.
- Readback/backup: C exporter reads regular files in small hex chunks; host decodes and hashes them. No device `base64`, FTP daemon or hash applet needed.
- Timeout: C diagnostic has a 30-second process alarm; host bounds each command. A host timeout is reported as an unknown remote state, not claimed as successful cancellation.
- Shell identity: process credentials from `/proc/self/status` and C `getuid`, without an `id` applet.

The smoke script uses only verified shell constructs and applets. Adding more utilities can be done later when a concrete module needs them; the current tooling does not replace BusyBox.

## Issues found and corrected during testing

1. GCC 3.3.4 rejects `-Wextra`; build now uses its supported `-W` with `-Wall -Werror`.
2. The old sysroot exposes absolute-input info as five integers rather than `struct input_absinfo`; the probe uses the equivalent layout confirmed against the kernel header.
3. The sysroot lacks `clock_gettime` declarations. The diagnostic uses the native legacy syscall number from the matching kernel, and monotonic advancement was measured on-device.
4. The shell's secondary prompt polluted command output; the Telnet session now clears both `PS1` and `PS2` after disabling echo.
5. The first applet inventory incorrectly relied on missing `command -v`; the directory-based check replaces it. The failed initial inventory is not used as final evidence.
6. Byte-at-a-time hex formatting exceeded the diagnostic alarm during the first boot-image backup. Buffered hex encoding and bounded range reads fixed it; the final complete image matched the local build's SHA-256. Error reporting now truncates large output rather than dumping binary hex into a traceback.
7. Kernel source inspection showed `IOR_HWSTATUS` clears a shared change flag. The probe deliberately uses `IOR_BUTTON_EVENT`, which reads a snapshot without clearing it, so existing power handling stays intact.
8. Brightness is returned as the ioctl return value, not written into an output pointer. The probe uses the actual driver behavior. The driver's charge-current field is a compatibility constant and is not exposed as a measured current.

## Evidence and backups

Final run: `artifacts/20261026T133558Z-783e01c1/`. Its `report.json` contains command results, transfer digests, backup digests and cleanup status. `probe-selftest.txt`, `shell-smoke.txt`, `capabilities.txt` and `final-health.txt` contain raw evidence. Earlier attempt directories are retained for troubleshooting and are not the final result. Artifacts are ignored by Git; run directories are private to the local user.

Backed up read-only to the host: current `ttsystem`, `/etc/rc`, OpenTom `start.sh`, power-button configuration, BusyBox executable and loader. These are targeted platform backups, not a full storage image.

Boot image SHA-256: `3f7a560b6b1990e659104e8a53ba5ffabf2b427169b23df7c29b7ef35f5317b4`, matching `/home/sepisotoni/projects/LeddaZ-OpenTom/build/ttsystem`.

Diagnostic SHA-256: `ad376398a48f9c6eb366c3508d684d873d63f2710dbdfc4703b8d66d83fafd0f`. Shell smoke SHA-256: `e65a238e2368f48239f88297deb9ac3649f7eee11266e157d0472cf0eeecff89`. Both matched their device readbacks. The final process inventory still showed OpenTom Nano-X, watchface, weather, GPS and power daemons running.

## Remaining gates

No actual touch/button interaction session, framebuffer drawing, brightness writes, audible speaker test, GPS fix, RTC retention/suspend wake, FAT interruption test, boot restoration rehearsal or Windows runtime verification has been performed. Those task boxes remain open. The recovery backup exists, but no claim is made that restoration has been rehearsed. No v3 autostart has been installed.
