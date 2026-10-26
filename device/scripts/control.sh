#!/bin/sh
set -eu
root=/mnt/sdcard/fracture-v3
version=$(cat "$root/current")
case "$version" in ''|*[!a-zA-Z0-9._-]*) echo 'Invalid installed version' >&2; exit 1;; esac
exec "$root/releases/$version/fracture-device" ctl "$@"
