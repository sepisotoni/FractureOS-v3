#!/bin/sh
# FractureOS per-user installer. No root privileges or package-manager changes.
set -eu
version=${FRACTURE_VERSION:-v0.1.11}
case "$version" in v[0-9]*.[0-9]*.[0-9]*) ;; *) echo 'Invalid version' >&2; exit 1;; esac
case "$version" in *[!a-zA-Z0-9._-]*) exit 1;; esac
os=$(uname -s)
case "$os" in
 MINGW*|MSYS*|CYGWIN*) exec powershell.exe -NoProfile -Command "\$env:FRACTURE_VERSION='$version'; Invoke-RestMethod 'https://raw.githubusercontent.com/sepisotoni/FractureOS-v3/$version/install/install.ps1' | Invoke-Expression";;
 Linux) ;;
 *) echo 'Supported: Linux x86-64/ARM64 and Windows x64/ARM64. Use PowerShell on Windows.' >&2; exit 1;;
esac
case "$(uname -m)" in x86_64) arch=amd64;; aarch64|arm64) arch=arm64;; *) echo 'Run this installer on the Linux USB host, not inside the TomTom. Its legacy ARM payload is supplied separately.' >&2; exit 1;; esac
for utility in curl tar sha256sum mktemp; do command -v "$utility" >/dev/null || { echo "Required host tool: $utility" >&2; exit 1; }; done
prefix=${FRACTURE_PREFIX:-"$HOME/.local"}
case "$prefix" in /*) ;; *) echo 'FRACTURE_PREFIX must be absolute' >&2; exit 1;; esac
case "$prefix" in *'"'*|*'%'*|*'\'*) echo 'Unsupported characters in installation prefix' >&2; exit 1;; esac
root="$prefix/share/fracture-v3"
if test -e "$root" && test ! -f "$root/.fracture-owned"; then echo "Refusing to replace unmanaged directory: $root" >&2; exit 1; fi
if test -e "$prefix/bin/fracture" && test ! -f "$root/.fracture-owned"; then echo 'Existing unmanaged fracture command; choose another prefix.' >&2; exit 1; fi
base=${FRACTURE_RELEASE_BASE:-"https://github.com/sepisotoni/FractureOS-v3/releases/download/$version"}
case "$base" in https://*) ;; file://*) test "${FRACTURE_TESTING:-0}" = 1 || exit 1;; *) echo 'HTTPS release URL required' >&2; exit 1;; esac
stage=$(mktemp -d)
trap 'rm -rf -- "$stage"' EXIT HUP INT TERM
archive="fracture-$version-linux-$arch.tar.gz"
curl -fLsS --retry 2 "$base/$archive" -o "$stage/$archive"
curl -fLsS --retry 2 "$base/SHA256SUMS" -o "$stage/SHA256SUMS"
expected=$(awk -v name="$archive" '$2==name {print $1}' "$stage/SHA256SUMS")
test "${#expected}" -eq 64 || { echo 'Missing or ambiguous release checksum' >&2; exit 1; }
actual=$(sha256sum "$stage/$archive"); actual=${actual%% *}
test "$actual" = "$expected" || { echo 'Release checksum mismatch' >&2; exit 1; }
mkdir "$stage/unpack"
# The release archive has a single, flat package root. Reject traversal/symlinks.
tar -tzf "$stage/$archive" | awk '/^\// || /(^|\/)\.\.($|\/)/ {bad=1} END {exit bad}'
tar -tvzf "$stage/$archive" | awk 'substr($0,1,1)!="-" && substr($0,1,1)!="d" {bad=1} END {exit bad}'
tar -xzf "$stage/$archive" -C "$stage/unpack" --no-same-owner
"$stage/unpack/bin/fracture" version
mkdir -p "$prefix/bin" "$root/releases"
# Marker owns only this install subtree, never the entire prefix.
printf '%s\n' 'FractureOS v3 managed installation' > "$root/.fracture-owned"
if test -d "$root/releases/$version"; then
 test -f "$root/releases/$version/.archive-sha256" && test "$(cat "$root/releases/$version/.archive-sha256")" = "$actual" || { echo 'Installed version differs. Use a new release version.' >&2; exit 1; }
else
 printf '%s\n' "$actual" > "$stage/unpack/.archive-sha256"
 mv "$stage/unpack" "$root/releases/$version"
fi
ln -sfn "releases/$version" "$root/current.new"
mv -Tf "$root/current.new" "$root/current"
# Install a small launcher so upgrades and device asset lookup use the same slot.
cat > "$prefix/bin/fracture.new" <<'LAUNCH'
#!/bin/sh
fracture_prefix=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export FRACTURE_ASSETS="$fracture_prefix/share/fracture-v3/current"
export FRACTURE_PREFIX="$fracture_prefix"
exec "$FRACTURE_ASSETS/bin/fracture" "$@"
LAUNCH
chmod 755 "$prefix/bin/fracture.new"
mv "$prefix/bin/fracture.new" "$prefix/bin/fracture"
if test "${FRACTURE_NO_SERVICE:-0}" != 1 && command -v systemctl >/dev/null && systemctl --user show-environment >/dev/null 2>&1; then
 unitdir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
 mkdir -p "$unitdir"
 unit="$unitdir/fracture-v3.service"
 if test -e "$unit" && ! grep -q '^# FractureOS managed unit' "$unit"; then echo 'Unmanaged service exists; start the hub manually.' >&2
 else
 cat > "$unit" <<UNIT
# FractureOS managed unit
[Unit]
Description=FractureOS USB clock hub
After=network.target
[Service]
ExecStart="$prefix/bin/fracture" serve
Restart=on-failure
RestartSec=5
[Install]
WantedBy=default.target
UNIT
 systemctl --user daemon-reload
 systemctl --user enable fracture-v3.service
 systemctl --user restart fracture-v3.service
 fi
else
 echo "Start the hub with: $prefix/bin/fracture serve"
fi
printf '\nInstalled %s in %s\n' "$version" "$prefix"
echo "Add $prefix/bin to PATH if needed."
echo 'Deploy to the connected TomTom: fracture device install'
echo 'Configure: fracture settings  |  Help: fracture help  |  Remove: fracture uninstall'
echo 'The hub listens on localhost by default. Change its listen address in Settings for Windows access.'
