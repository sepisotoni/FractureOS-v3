# Owner's TomTom LAN mapping

Configured on `stoni-room-serve`, 2026-10-26. These interface names and addresses are specific to this host; the generic installer does not apply them.

| Role | Address |
|---|---|
| Linux LAN / HTTPS hub | 192.168.101.105 / port 47831 |
| Linux end of USB link | 192.168.101.105/32 |
| TomTom internal USB address | 192.168.101.115 |
| TomTom LAN-facing alias | 192.168.101.114 |

The extra `.114/24` Linux LAN address was removed from `/etc/netplan/50-cloud-init.yaml`. The USB networkd file `/etc/systemd/network/20-tomtom-usb.network` now uses `.105/32`. Host routes direct `.114` and `.115` to `enx9abda0e24f81`; proxy ARP on `enp1s0` answers for `.114`. Scoped NAT translates `.114` to `.115`, translates incoming source addresses to the USB host `.105`, and translates device egress to `.114`. Existing Tailscale/firewall chains were preserved. No router Internet port forwarding was added.

Implementation: `install/tomtom-lan.sh`, installed as `/usr/local/sbin/fracture-tomtom-lan`, and `fracture-tomtom-lan.service` under `/etc/systemd/system`. `/etc/udev/rules.d/80-fracture-tomtom.rules` requests the service on USB reattachment. The device's optional `/mnt/sdcard/fracture-v3/gateway` contains `.105`; `boot.sh` adds that default route when configured. The live route was applied without rebooting.

The hub config now binds/connects to `https://192.168.101.105:47831`. Pairing token and certificate were preserved. Existing Windows configurations that still use `.114:47831` must change their hub URL to `.105:47831` in Connection settings, retaining the fingerprint/token.

Validation: `.114` and `.115` Telnet connections from this host both reached the TomTom's ARM Linux 2.6.13 kernel; the HTTPS hub continued working at `.105`; netplan generation succeeded. Actual access from the owner's Windows PC and a physical USB disconnect/reconnect were not exercised during this change. Root Telnet is a trusted LAN/USB development interface, not an Internet-facing service.

## Recovery

Backups were saved beside the network files with `.before-fracture-v019` suffixes. A private hub-config/firewall snapshot is in the ignored `artifacts/network-v019` directory. To revert this mapping, stop/disable `fracture-tomtom-lan.service`, remove only its named udev rule, restore the two backed-up network files, and restore the saved hub configuration before restarting the hub. The service's `stop` action removes only its own proxy entry, alias route and NAT rules; it deliberately does not reset global forwarding used by Tailscale. Reapplying netplan can affect active network sessions, so use a local console or an independent connection for a full rollback.
