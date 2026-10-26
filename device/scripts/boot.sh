#!/bin/sh
# Keep this foreground supervisor alive: /etc/rc reboots if start.sh exits.
root=/mnt/sdcard/fracture-v3
echo $$ >/tmp/fracture-supervisor.pid
ip=$(cat "$root/device-ip")
case "$ip" in ''|*[!0-9.]*) exec /bin/sh "$root/stock-start.sh";; esac
ifconfig usb0 "$ip" netmask 255.255.255.0 up
ifconfig lo 127.0.0.1 up
failures=0
while test ! -f "$root/disabled"; do
    version=$(cat "$root/current")
    case "$version" in ''|*[!a-zA-Z0-9._-]*) break;; esac
    executable="$root/releases/$version/fracture-device"
    test -x "$executable" || break
    started=$(date +%s)
    "$executable" --assets "$root/releases/$version/faces" --config "$root/settings.cfg"
    ended=$(date +%s)
    if test $((ended - started)) -ge 30; then failures=0; else failures=$((failures + 1)); fi
    test "$failures" -lt 3 || break
    sleep 1
done
echo 'FractureOS stopped: returning to preserved OpenTom environment' >&2
exec /bin/sh "$root/stock-start.sh"
