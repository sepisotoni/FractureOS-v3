# Release protocol

Every release, by anyone (human or AI agent), follows this list in order. Do not skip steps, do not claim a step passed unless you ran it, and say plainly what you could not test.

## 0. Rules
- Never put tokens or secrets in the repo, in commits, in URLs saved in `.git/config`, or in chat logs. Pass credentials out of band; the owner revokes them afterwards.
- Never move or re-tag a published tag. A fix is a new version (`vX.Y.Z` or the hotfix form `vX.Y.Z.W`).
- Windows is installed and updated with the Inno Setup `.exe` and the in-app **Update FractureOS** button. There is no Microsoft Store path.
- Release notes are compiled into the app (`host/changelog.go`). They must be updated for every release; nothing is fetched from the network.

## 1. Prepare
1. Branch `release/vX.Y.Z[.W]` from `main`. Merge feature branches into it; resolve conflicts here, not on `main`.
2. `python3 tools/release.py bump X.Y.Z[.W]` (never edit version strings by hand).
3. Add the entry at the top of `host/changelog.go` (`Version: "vX.Y.Z[.W] ..."`, accurate `Status`, user-facing changes only).
4. `python3 tools/release.py check` must pass.

## 2. Verify (all must pass; record the output in the PR)
- `gofmt -l host` prints nothing
- `go vet ./host`, `go test -race ./host`
- Cross-builds: `GOOS=windows GOARCH=amd64|arm64` and `GOOS=linux GOARCH=amd64|arm64`
- Device C build: `sh tools/build-device.sh`
- Updater: `go test ./host -run 'Version|Update'` and, for a new version format, a test for `compareVersions`
- Installer: the `windows-installer` workflow must compile `install/fracture.iss` for amd64 and arm64
- If the change touches wallpaper, preview, USB or the dashboards: walk through `docs/E2E-CHECKLIST.md` and mark each item pass / fail / not testable (and why)

## 3. Publish
1. Open a PR into `main`; merge only when section 2 is green.
2. Tag `vX.Y.Z[.W]` on the merge commit. Publish the GitHub release as a **prerelease** first.
3. Assets (names are exact, the updater looks them up):
   - `fracture-<tag>-windows-amd64-setup.exe`, `fracture-<tag>-windows-arm64-setup.exe`
   - `fracture-<tag>-windows-{amd64,arm64}.zip`, `fracture-<tag>-linux-{amd64,arm64}.tar.gz`
   - `SHA256SUMS` containing a line `<sha256>  <name>` for **every** asset above, including both setup exes
   - `build-info.json`
4. Run the `Windows setup for in-app updater` workflow with the tag if the setup exes or their `SHA256SUMS` lines are missing.
5. Download `SHA256SUMS` and the setup exe from the published release and confirm the hash matches.

## 4. After publishing
- Install the previous version, press **Update FractureOS**, confirm it reaches the new version with settings kept and no "missing update checksum" or close-app error.
- Update the Linux hub and confirm the web dashboard loads with no hub 404.
- Only then promote from prerelease, and update `README.md` / `TASKS.md` status.
