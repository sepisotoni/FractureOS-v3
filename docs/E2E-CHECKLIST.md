# End-to-end checklist: host, web dashboard and device push

Branch `agent/device-web` on `release/v0.1.18.1`. Written by AGENT-B.
Environment of every run below: Linux x86-64 sandbox, Go 1.22.2, Node 18+ with jsdom, **no physical TomTom, no Windows machine, no ARM toolchain, no real browser, no access to api.open-meteo.com or Spotify/Windows APIs.**

Legend
- **PASS**: ran automatically, the command is given, and a deliberate break of the code made it fail (mutation checked where marked `M`).
- **PASS (fake)**: passes against the fake TomTom in `host/fakedevice_test.go`. That fake was written from reading the host's own code, so it proves the host's framing and flow, **not firmware behaviour**.
- **FAIL**: known problem, not fixed.
- **NOT TESTABLE HERE**: cannot be run in this environment; the reason is stated.
- **NOT COVERED**: could be tested here but is not yet.

## How to run everything

```sh
export GOPROXY=direct GOSUMDB=off                    # sandbox has no module proxy
go vet ./host/ && go test -race -count=1 ./host/     # 71 top-level tests + 20 subtests
CGO_ENABLED=0 go build -o /dev/null ./host/          # Linux hub release build mode
GOOS=windows go build -o /dev/null ./host/           # Windows cross-build
sh tests/preview_drift.sh                            # device renderer vs PC preview renderer, native x86
cd tests/ui && npm i jsdom && node wallpaper-ui.test.js
NODE_PATH=... sh tests/preview_drift_js.sh           # web preview vs device (needs jsdom + @napi-rs/canvas)
```

Last run (this branch): vet OK, `go test -race` 71/71 + 20/20 subtests PASS, `CGO_ENABLED=0` build OK, Windows build OK, drift script 6/6 PASS, UI test 25/25 PASS.

## 1. Build and static checks

| Item | Result | How / reason |
|---|---|---|
| `go vet ./host/` (linux) | PASS | clean |
| `go build` with `CGO_ENABLED=0` | PASS | the Linux hub release mode |
| Windows cross-build incl. new tests | PASS | `GOOS=windows go build`; tests compile for windows via `go vet` |
| `GOOS=windows go vet ./host/` | FAIL (pre-existing) | 9 warnings `possible misuse of unsafe.Pointer`: `audio_windows.go` 187, 248, 258; `dashboard_windows.go` 441, 823 (AGENT-A); `media_windows.go` 45, 46, 93, 352 (AGENT-C). None are in files changed by AGENT-B |
| `gofmt -l host` | FAIL (not mine) | `dashboard_layout.go` (AGENT-A), `media.go`, `notifications_windows.go` (AGENT-C). `telnet.go` fixed |
| Race detector | PASS | `go test -race` |
| ARM device build (`tools/build-device.sh`) | NOT TESTABLE HERE | needs the OpenTom ARM gcc 3.3.4 SDK |
| Windows binary run | NOT TESTABLE HERE | no Windows host |

## 2. Settings UI routes (`newSettingsMux`, session header auth)

Run: `go test -race ./host/ -run 'TestUI'`. These go UI mux -> real pinned-TLS `hubRequest` -> real hub mux -> real telnet client -> fake device.

| Route | Result | What is asserted |
|---|---|---|
| `/` page + `/favicon.ico`, `/fracture.svg` | PASS | served without a session, `X-Frame-Options: DENY`, `no-store`, contains the Wallpaper tab |
| session check (all routes) | PASS M | missing/wrong `X-Fracture-Session` -> 401 on every route; unknown route -> 404 |
| `/changelog` | PASS | GET returns a non-empty JSON array; POST -> 405 |
| `/config` GET/POST | PASS | token hidden on GET; bad connection mode -> 400; stale form cannot overwrite hub-only weather/agenda/notification/audio settings |
| `/config` direct mode | PASS | fills `127.0.0.1:47831`, hub URL, `192.168.101.115`; generates token, cert, key, 64-hex fingerprint |
| `/command` | PASS | valid command reaches the device as `control.sh` args; invalid command refused before the device; device error -> 502 with message; unreachable device -> 502 |
| `/integrations` | PASS | GET/POST round trip persists weather; out-of-range latitude refused and config unchanged; disabling weather sends `weather-clear` |
| `/wallpaper` | PASS (fake) | dry run touches nothing; apply uploads exactly 153,600 bytes to `wallpaper.rgb` and sends `wallpaper load`; unchanged sync does not re-upload; restore returns to the previous selection |
| old hub missing an endpoint | PASS | UI returns "the connected hub is older than this app ...", never the raw "404 page not found" (the original "hub: 404" bug) |
| `/update` | PASS | see section 8 |
| `/notification-test` | NOT TESTABLE HERE | Windows-only; on Linux the route returns a clear 502 (asserted) |

## 3. Hub API routes (`newHubMux`, bearer auth)

Run: `go test -race ./host/ -run 'TestHub'`.

| Route | Result | What is asserted / what is not |
|---|---|---|
| all 10 `/api/*` routes | PASS M | missing/wrong bearer token -> 401; wrong method -> 405 where applicable |
| `/api/command` | PASS (fake) | valid -> 200; below-range time, unknown JSON field, non-JSON, 2 KB command -> 400 |
| `/api/integrations` | PASS | see section 2 |
| `/api/notifications` | PASS (fake) | disabled by default; excluded app -> "excluded"; empty app / unknown field -> 400; delivery sends `notice -`, `notice-text app/title/body`, `notice-show N`; burst -> 429; device-side disable honoured |
| `/api/forecast` | PASS (fake) | valid forecast lands on the device as `weather.dat`; out-of-range temperature and empty forecast -> 400 |
| `/api/agenda` | PASS (fake) | valid events send `agenda-clear` + events; empty title, missing time, unknown field -> 400 |
| `/api/media` | PASS (fake) | valid snapshot -> `media ...` commands; position > duration, huge duration, negative volume, garbage -> 400 |
| `/api/audio` | PASS (guards only) | wrong content type -> 415; bad source -> 400; disabled -> 409. Streaming frames: see section 6 |
| `/api/preview` | PASS (fake) M | model op returns `FPR1`; old device -> actionable "0.1.11" message; asset op downloads in 8 KiB hex chunks and returns identical bytes; stale hash -> 409; unknown kind, zero hash, unknown op -> 400; truncated chunk from the device -> 502 |
| `/api/lyrics` | PASS (fake) M | session; provide (wrong track -> 409, disabled in settings -> 403); clear-provided; schedule (sends `lyrics-begin`/`lyrics-line`/`lyrics-commit`; decreasing times, no lines, 13 lines, time past a day -> 400); update (sends `media-lyrics ...`; oversized -> 400; view closed -> 409); unknown op -> 400; oversized title / unknown field -> 400. Note `Broker.call` caches `status` for 1 s by design, so a lyrics-view change can lag up to 1 s |
| `/api/wallpaper` | PASS (fake) | see section 4 |

## 4. Wallpaper (item 1 and 3)

Run: `go test -race ./host/ -run 'Wallpaper|Offline|Stale|Reflash|Solid|Legacy|Unconfirmed|DryRun|Restore|Invalid|Pending|Colour|Truncate|HTTP|TestUI'`.

| Behaviour | Result |
|---|---|
| Save -> apply -> persist across restart (config on disk) | PASS (fake) |
| Apply while TomTom unplugged saves and warns; later plug-in pushes exactly once; second sync is a no-op | PASS (fake) M |
| Selection changed while unplugged replaces the *stale* wallpaper on the device (old "wallpaper_active only" hole) | PASS (fake) |
| Device re-flashed / wallpaper lost -> restored | PASS (fake) |
| Firmware older than 0.1.11 (no `home-state`) falls back to `status` | PASS (fake) |
| Unconfirmed load reported and retried | PASS (fake) |
| Dry run saves, stores and sends nothing | PASS (fake) |
| Restore previous selection | PASS (fake) |
| Delete of the applied custom wallpaper -> 409; of another one -> OK | PASS |
| Invalid input (path traversal, missing preset/id, bad colour, bad mode) leaves config untouched | PASS |
| Legacy black (#000000) changes no theme/face background | PASS (fake) |
| Solid colour records originals and restores exactly those values; never calls `theme-reset`/`face-reset`; pending restore completes on plug-in; refuses to override when originals unreadable | PASS (fake) |
| `mode:"pc"` on a Remote Linux hub reads the **hub's** desktop, not the Windows PC | FAIL (design): Windows must upload its own wallpaper as a custom image (AGENT-A) |
| Real `wallpaper_hash`/dim/blur on the ARM firmware | NOT TESTABLE HERE (no TomTom) |
| Real telnet over USB | NOT TESTABLE HERE |
| Selection-level backup/restore (`previous.json`, "Restore previous") | PASS (fake) |
| Device-file backup/restore (copy of `wallpaper.rgb` on the TomTom) | NOT IMPLEMENTED. Hardware-gated, TASKS F-03. There is no device-side backup in 0.1.18.1 |
| Caveat: restoring a colour override writes the original value back (an override file with the original value remains); a user's own theme-background edit made during an override is overwritten on restore | KNOWN GAP |

## 5. Preview (item 2)

| Item | Result | How / reason |
|---|---|---|
| PC preview renderer == device renderer, home screen, 3 faces x {no wallpaper, wallpaper} | PASS M | `tests/preview_drift.sh`: 0 differing pixels. Control: feeding an undimmed wallpaper gave 69,238 differing pixels |
| Menu screens in the preview | NOT SUPPORTED | `navigation_draw()` is compiled out of the preview build; TASKS F-01 (declined for 0.1.18.1) |
| Notification overlay / dock / other model states in the drift test | NOT COVERED | only the default home state with a fixed clock is compared |
| Home-preview packet (`fetchHomePreview`): downloads atlas/font/notice/wallpaper from the device, reuses unchanged assets, serves a fresh client from the disk cache, repairs a corrupted cache entry by hash, degrades to no wallpaper if the wallpaper download fails, rejects a missing face or malformed model | PASS (fake) M | `TestFetchHomePreview*`. The fake's asset bytes are synthetic; the real firmware hash/size contract is not testable here |
| Windows DLL path (`preview_windows.go` -> `fracture-preview.dll`) | NOT TESTABLE HERE | no Windows; the native x86 build of the same C source is what is tested. Note the renderer emits **BGRA** |
| Web dashboard preview vs device | **FAIL** | `tests/preview_drift_js.sh`: aqua 17.66%, neon 22.28%, sunset 17.61% of pixels differ; 14.8% / 10.8% / 14.8% differ by more than 32/255. Images: `docs/preview-drift/`. Cause is structural (browser fonts and hand-tuned scale factors vs bitmap atlases): thinner glyphs, higher position with clipping at the top edge, bold bright date and minutes label. Font fallback in this sandbox affects the absolute number but not these differences |

## 6. Weather, notifications, audio, media

| Item | Result | Reason |
|---|---|---|
| Weather settings validation and persistence | PASS | section 2 |
| Weather forecast push to device | PASS (fake) | section 3 |
| Weather description text | PASS | existing `TestWeatherDescription` |
| Lyrics search (`fetchLyrics`/`cachedLyrics`, LRCLIB): found, 404, 5xx, malformed, oversized, over-long lyrics, redirect refused, duration filter omitted when implausible, 7-day expiry, 8-entry bound, failures not cached | PASS M | `TestFetchLyricsResponses`, `TestCachedLyricsCachesExpiresAndPrunes` against a fake TLS server. The real lrclib.net is not reachable from the sandbox |
| Weather fetch + parse from Open-Meteo (`weatherSupervisor`) | NOT COVERED | the sandbox cannot reach api.open-meteo.com and the URL has no test seam; supervisor code is AGENT-C's |
| Notification filtering, quiet hours, queue | PASS | existing `notifications_test.go` |
| Notification route and device delivery | PASS (fake) | section 3 |
| Windows notification capture (WinRT listener) | NOT TESTABLE HERE | Windows only; also TASKS B12-06 |
| Audio config validation, frame scaling, queue, device frame write | PASS | existing `audio_test.go` |
| Audio route guards | PASS | section 3 |
| Audio capture and streaming to the TomTom | NOT TESTABLE HERE | needs Windows/Pulse capture and the device's audio path |
| Media text transliteration | PASS | existing `media_test.go` |
| Media route | PASS (fake) | section 3 |
| Spotify/Windows media session, artwork upload | NOT TESTABLE HERE | needs those services; artwork upload uses the same wget-back path as the wallpaper |

## 7. Time sync

| Item | Result | How |
|---|---|---|
| `syncDeviceTime` sends current Unix time as `time <epoch>` | PASS (fake) M | `TestTimeSyncSendsCurrentUnixTime` |
| Out-of-range / malformed `time` commands refused | PASS M | validateCommand cases in the same test |
| Unreachable device -> error | PASS | same test |
| 5-minute cadence inside `hubBackground` | NOT COVERED | timer loop (AGENT-C's supervisor code) |
| Device applying the time and timezone offset | NOT TESTABLE HERE | firmware |

## 8. Update

| Item | Result | How |
|---|---|---|
| Release check: newer, four-part hotfix, draft ignored, junk tags ignored, current, older-only | PASS | `TestCheckUpdateOffers` against a fake TLS release server |
| Release server error / malformed JSON -> error | PASS | same |
| Download verifies `SHA256SUMS`: valid accepted; tampered archive, missing entry, ambiguous entry, short digest rejected | PASS M | `TestDownloadUpdateVerifiesChecksum` |
| Malformed version / no-update offer will not download | PASS | same |
| `/update` route: check, unknown action, bad JSON, up-to-date install, release server down | PASS | `TestUpdateRouteActionsAndUpToDateInstall` |
| Actual install (Linux `install.sh` via curl; Windows installer helper) | NOT TESTABLE HERE | deliberately destructive: replaces the running install, needs network to GitHub and (Windows) an elevated installer |
| Real GitHub API/redirect host allow-list | NOT TESTABLE HERE | sandbox cannot reach GitHub release assets; the allow-list code is exercised only for direct requests |

## 9. Direct USB (host side)

| Item | Result | How / reason |
|---|---|---|
| Direct-mode config defaults and identity generation | PASS | `TestUIDirectModeConfigGeneratesIdentity` |
| Hub served with the generated cert (TLS 1.3, loopback) and driven by the pinned client end to end | PASS (fake) | `TestDirectModeLocalHubEndToEnd`; wrong fingerprint and wrong token are refused |
| `effectiveConnectionMode` selection | PASS | same |
| USB RNDIS link to `192.168.101.115`, driver, PawnIO, tray app, Windows dashboard | NOT TESTABLE HERE | Windows + hardware (AGENT-A) |

## 10. Web dashboard (`host/ui.html`)

Run: `cd tests/ui && node wallpaper-ui.test.js` (jsdom with a stubbed API).

| Item | Result |
|---|---|
| No top-level Wallpaper tab; wallpaper card inside Appearance | PASS M (fails on the pre-change page) |
| Launch deep links `?page=appearance/wallpaper#token` and legacy `?page=wallpaper#token`; session token preserved; clean `#appearance/wallpaper` afterwards | PASS M |
| `?page=appearance` opens Clock & faces; tab click, `hashchange`, in-page link all navigate | PASS |
| Staging sends only `dry_run`; Apply sends exactly one real request; Restore; Discard | PASS |
| Apply disabled until a change; Restore disabled while changes pending | PASS |
| Real browser rendering, layout, canvas crop tool, mobile layout | NOT TESTABLE HERE (jsdom is not a browser; canvas is stubbed) |
| Other views (Device, Integrations, System, What's new) | NOT COVERED beyond loading |

## 11. Hygiene

| Item | Result |
|---|---|
| No tokens/secrets committed | PASS (`git diff` scan for GitHub token patterns on every commit: 0 matches) |
| No version strings, workflows or `install/*` touched | PASS |

## 12. Release-notes wording (draft, for LEAD)

Suggested text for the 0.1.18.1 notes; it makes no claim of a device-side backup:

- Wallpaper now lives under Appearance (Clock & faces | Wallpaper). Choosing a wallpaper only previews it; nothing is sent to the TomTom until you press **Apply to TomTom**.
- **Restore previous** returns to your previous wallpaper *selection*. FractureOS does not back up the wallpaper file stored on the TomTom itself.
- A wallpaper chosen while the TomTom is unplugged, or lost if the TomTom is re-flashed, is applied automatically when it connects.
- A solid colour records the theme and face background it replaces and puts exactly those back when you pick something else; Legacy black changes no theme or face colours.
- Fixed: "hub: 404 page not found" when the app is newer than the Linux hub; the message now says to update the hub or use Direct USB.
- Fixed: opening the dashboard on a specific page (`?page=`) showed an error.
