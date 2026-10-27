#!/bin/sh
# Host-specific reversible mapping: Linux .105, TomTom LAN .114 / USB .115.
# Installed only by explicit host administration, never by the generic installer.
set -eu
lan=enp1s0
usb=${2:?Usage: tomtom-lan.sh start|stop USB_INTERFACE}
case "$usb" in ''|*[!a-zA-Z0-9_.-]*) echo 'Invalid USB interface' >&2; exit 1;; esac
case "${1:-}" in
 start)
  # The gadget MAC (and hence its enx name) changes after reconnecting.
  # Restrict this host-specific mapping to the known TomTom USB port/device.
  properties=$(udevadm info -q property "/sys/class/net/$usb")
  echo "$properties" | grep -qx 'ID_PATH=pci-0000:00:14.0-usb-0:10:1.0'
  echo "$properties" | grep -qx 'ID_VENDOR_ID=0525'
  echo "$properties" | grep -qx 'ID_MODEL_ID=a4a1'
  ip link show "$usb" >/dev/null
  ip addr replace 192.168.101.105/32 dev "$usb"
  ip route replace 192.168.101.115/32 dev "$usb" src 192.168.101.105
  ip route replace 192.168.101.114/32 dev "$usb" src 192.168.101.105
  sysctl -qw net.ipv4.ip_forward=1
  sysctl -qw "net.ipv4.conf.$usb.proxy_arp=1"
  ip neigh replace proxy 192.168.101.114 dev "$lan"
  iptables -t nat -N FRACTURE_TOMTOM 2>/dev/null || true
  iptables -t nat -F FRACTURE_TOMTOM
  iptables -t nat -A FRACTURE_TOMTOM -d 192.168.101.114/32 -j DNAT --to-destination 192.168.101.115
  iptables -t nat -C PREROUTING -i "$lan" -d 192.168.101.114/32 -j FRACTURE_TOMTOM 2>/dev/null || iptables -t nat -A PREROUTING -i "$lan" -d 192.168.101.114/32 -j FRACTURE_TOMTOM
  iptables -t nat -C OUTPUT -d 192.168.101.114/32 -j FRACTURE_TOMTOM 2>/dev/null || iptables -t nat -A OUTPUT -d 192.168.101.114/32 -j FRACTURE_TOMTOM
  iptables -t nat -C POSTROUTING -o "$usb" -d 192.168.101.115/32 -j SNAT --to-source 192.168.101.105 2>/dev/null || iptables -t nat -A POSTROUTING -o "$usb" -d 192.168.101.115/32 -j SNAT --to-source 192.168.101.105
  iptables -t nat -C POSTROUTING -o "$lan" -s 192.168.101.115/32 -j SNAT --to-source 192.168.101.114 2>/dev/null || iptables -t nat -A POSTROUTING -o "$lan" -s 192.168.101.115/32 -j SNAT --to-source 192.168.101.114
  ip addr del 192.168.101.114/24 dev "$lan" 2>/dev/null || true
  ip addr del 192.168.101.114/32 dev "$usb" 2>/dev/null || true
  ;;
 stop)
  iptables -t nat -D PREROUTING -i "$lan" -d 192.168.101.114/32 -j FRACTURE_TOMTOM 2>/dev/null || true
  iptables -t nat -D OUTPUT -d 192.168.101.114/32 -j FRACTURE_TOMTOM 2>/dev/null || true
  iptables -t nat -D POSTROUTING -o "$usb" -d 192.168.101.115/32 -j SNAT --to-source 192.168.101.105 2>/dev/null || true
  iptables -t nat -D POSTROUTING -o "$lan" -s 192.168.101.115/32 -j SNAT --to-source 192.168.101.114 2>/dev/null || true
  iptables -t nat -F FRACTURE_TOMTOM 2>/dev/null || true
  iptables -t nat -X FRACTURE_TOMTOM 2>/dev/null || true
  ip neigh del proxy 192.168.101.114 dev "$lan" 2>/dev/null || true
  ip route del 192.168.101.114/32 dev "$usb" 2>/dev/null || true
  ;;
 *) echo 'Usage: tomtom-lan.sh start|stop USB_INTERFACE' >&2; exit 2;;
esac
