# Reconnect handoff — v0.1.10

Owner unplugged the TomTom on 2026-10-27 and explicitly deferred applying changes until returning. Do not automatically deploy on USB reconnection alone.

## Ready locally

- ARM binary, Go host builds and Linux/Windows release packages in `build/release`.
- Accepted Windows x64/ARM64 Inno artifacts in `artifacts/windows-v0110-accepted`. Windows run 40456618712 passed native settings, staged update/reopen, retained configuration, tray close and uninstall. ARM64 was built only.
- Real-device preview captures under `artifacts/manual-v0110`: mixed dock, activity strip, notifications with artwork, supplied lyrics, current/hourly/week weather, media symbols and clock.
- Tests passed; final app overlays/media timeout exemptions have only host checks since unplugging. Actual Windows UAC/PawnIO sensor data still requires the owner's PC.

## Last observed state

The device was running `/mnt/sdcard/fracture-v3/releases/v0.1.10-preview`. The final deploy attempt `artifacts/deploy-v0110-final` timed out while uploading staging payloads, before activation. The root volume icon was not replaced. Temporary files may remain in a `/tmp/fracture-install-<hex>` directory; inspect and remove only confirmed FractureOS staging files after reconnecting. The old deployer's cleanup timeout prevented its final report from being written; the updated deployer preserves deferred-cleanup evidence.

The locally started `fracture-preview-v0110.service` was explicitly stopped after the unplug message. The installed `fracture-v3.service` was already stopped during preview work. Resume the appropriate hub service when the owner asks to apply.

## When the owner returns and requests application

1. Read current settings/status first; the owner actively tests controls. Preserve their latest profile, theme, brightness, offset and alarm. Do not restore the earlier `before.json` over new choices.
2. Identify reachability over USB `.115`, with Linux/hub `.105` and the existing TomTom LAN alias `.114`. Inspect current release pointer and any interrupted staging directory. Do not change networking based solely on old assumptions.
3. Use a fresh private evidence directory because `artifacts/deploy-v0110-final` already exists. Deploy the verified v0.1.10 payload. It backs up `tomtom.ico`, installs the shared FractureOS icon, verifies readbacks and activates the immutable release slot. Do not rewrite a differing immutable v0.1.10 slot; inspect first.
4. Inspect the final overlay refinements, active player/lyrics staying open, menu returning home, forecast cache offline behavior and current RAM. Preserve timer/watch state if the owner has started one since the previous session.
5. Install/activate the matching Linux host build, start `fracture-v3.service`, and check companion reconnect. There should be one hub listener on `.105:47831` and no leftover preview service.
6. Mark W07/W08 complete only after the device checks. Keep W06 open until the Windows owner verifies Authorize sensor with their installed PawnIO driver.
