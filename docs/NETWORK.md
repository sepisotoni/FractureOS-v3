# Owner's TomTom LAN mapping

Configured on `stoni-room-serve`, 2026-10-26. These interface names and addresses are specific to this host; the generic installer does not apply them.

| Role | Address |
|---|---|
| Linux LAN / HTTPS hub | 192.168.101.105 / port 47831 |
| Linux end of USB link | 192.168.101.105/32 |
| TomTom internal USB address | 192.168.101.115 |
| TomTom LAN-facing alias | 192.168.101.114 |

The extra `.114/24` Linux LAN address was removed from `/etc/netplan/50-cloud-init.yaml`. The USB networkd file `/etc/systemd/network/20-tomtom-usb.network` now uses `.105/32`. Host routes direct `.114` and `.115` to the current TomTom USB interface; proxy ARP on `enp1s0` answers for `.114`. Scoped NAT translates `.114` to `.115`, translates incoming source addresses to the USB host `.105`, and translates device egress to `.114`. Existing Tailscale/firewall chains were preserved. No router Internet port forwarding was added.

Implementation: `install/tomtom-lan.sh`, installed as `/usr/local/sbin/fracture-tomtom-lan`, accepts `start|stop USB_INTERFACE`. `install/fracture-tomtom-lan@.service` is installed under `/etc/systemd/system`; each instance binds to its USB interface lifetime. `/etc/udev/rules.d/90-fracture-tomtom.rules` requests that instance, matching `ID_PATH=pci-0000:00:14.0-usb-0:10:1.0` and USB vendor/product `0525:a4a1`. The rule runs after network path discovery. The device's optional `/mnt/sdcard/fracture-v3/gateway` contains `.105`; `boot.sh` adds that default route when configured. The live route was applied without rebooting.

The hub config now binds/connects to `https://192.168.101.105:47831`. Pairing token and certificate were preserved. Existing Windows configurations that still use `.114:47831` must change their hub URL to `.105:47831` in Connection settings, retaining the fingerprint/token.

Validation: `.114` and `.115` Telnet connections from this host both reached the TomTom's ARM Linux 2.6.13 kernel; the HTTPS hub continued working at `.105`; netplan generation succeeded. On 2026-10-27 physical reconnection changed the gadget MAC/interface from `enx9abda0e24f81` to `enx227e1f145615`, exposing the old fixed-name service/rule. The replacement selects the known USB port instead of its changing MAC. Udev evaluation requested the matching service instance, unit verification passed, the instance started, and `.114` reached the ARM device again. The pinned hub reconnected and live Windows media arrived. Another physical replug with the replacement rule remains untested. Moving the cable to another host port requires updating the path matches in networkd and the rule/script. Root Telnet is a trusted LAN/USB development interface, not an Internet-facing service.

## Recovery

Backups were saved beside the network files with `.before-fracture-v019` suffixes. A private hub-config/firewall snapshot is in the ignored `artifacts/network-v019` directory. The old non-template unit remains disabled; the old script and rule have `.before-fracture-v0110` backups. To revert this mapping, stop the active `fracture-tomtom-lan@INTERFACE.service`, remove only its named `90-fracture-tomtom.rules` rule, restore the two backed-up network files, and restore the saved hub configuration before restarting the hub. The service's `stop` action removes only its own proxy entry, alias route and NAT rules; it deliberately does not reset global forwarding used by Tailscale. Reapplying netplan can affect active network sessions, so use a local console or an independent connection for a full rollback.
