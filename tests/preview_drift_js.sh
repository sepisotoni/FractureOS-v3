#!/bin/sh
# Measures web-dashboard (JS canvas) vs device (C renderer) pixel drift for the home screen.
# Needs: gcc, python3, node with jsdom and @napi-rs/canvas on NODE_PATH. Output PNGs: $1 (default docs/preview-drift).
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd); root=$(dirname "$here")
out=${1:-$root/docs/preview-drift}; work=$(mktemp -d); trap 'rm -rf "$work"' EXIT; mkdir -p "$out" /mnt/sdcard/fracture-v3
cc="gcc -std=gnu99 -O1 -w -idirafter /usr/include"
$cc "$here/preview_drift_device.c" -o "$work/dev"; $cc -fPIC "$here/preview_drift_preview.c" -o "$work/prev"
for face in aqua neon sunset; do
  "$work/dev" "$root/assets/faces" "$work/p-$face" "" "$face"; "$work/prev" "$work/p-$face" "$work/p-$face.rgba"
done
node "$here/ui/preview-drift-js.js" "$work/p" "$out"
