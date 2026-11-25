#!/bin/sh
set -eu
prefix=${FRACTURE_PREFIX:-"$HOME/.local"}
root="$prefix/share/fracture-v3"
test -f "$root/.fracture-owned" || { echo 'Managed FractureOS installation not found' >&2; exit 1; }
unit="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/fracture-v3.service"
if test -f "$unit" && grep -q '^# FractureOS managed unit' "$unit" && grep -Fq "ExecStart=\"$prefix/bin/fracture\" serve" "$unit"; then
 if command -v systemctl >/dev/null; then systemctl --user disable --now fracture-v3.service || true; fi
 rm -- "$unit"
 if command -v systemctl >/dev/null; then systemctl --user daemon-reload || true; fi
fi
if test -f "$prefix/bin/fracture" && grep -q 'FRACTURE_ASSETS=.*share/fracture-v3/current' "$prefix/bin/fracture"; then rm -- "$prefix/bin/fracture"; fi
# Keep settings, certificates, device backups and the running TomTom installation.
rm -rf -- "$root"
echo 'Removed the host app. Settings and backups retained. TomTom startup was not changed.'
echo 'To restore the TomTom, run fracture device restore before uninstalling the host app.'
