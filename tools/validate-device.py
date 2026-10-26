#!/usr/bin/env python3
"""Run the v3 platform scripts on the actual USB TomTom and retain evidence."""
import argparse
import datetime
import hashlib
import json
import secrets
import shlex
import subprocess
from pathlib import Path

from tomtom import TomTom


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="192.168.101.115")
    parser.add_argument("--bind", default="192.168.101.114")
    args = parser.parse_args()
    root = Path(__file__).resolve().parent.parent
    subprocess.run(["sh", str(root / "tools/build-probe.sh")], check=True)
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    suffix = secrets.token_hex(4)
    evidence = root / "artifacts" / (stamp + "-" + suffix)
    evidence.mkdir(parents=True, mode=0o700)
    stage = "/tmp/fracture-v3-check-" + suffix
    report = {"host": args.host, "utc": stamp, "stage": stage, "complete": False, "checks": [], "backups": {}}
    uploaded = []
    print("Evidence:", evidence, flush=True)
    with TomTom(args.host) as device:
        def run(name, command, expected=0, timeout=15):
            code, output = device.command(command, timeout)
            (evidence / (name + ".txt")).write_text(output)
            report["checks"].append({"name": name, "exit_code": code, "expected": expected, "passed": code == expected})
            print(name + ": exit=" + str(code), flush=True)
            if code != expected:
                raise RuntimeError(name + " failed: " + output)
            return output

        run("inventory", "uname -a; cat /proc/version; cat /proc/cpuinfo; cat /proc/meminfo; mount; df -k; cat /proc/partitions; cat /proc/bus/input/devices; cat /proc/self/status; ps; busybox")
        run("applet-matrix", "for applet in id printf timeout sha256sum md5sum base64 tar dd od hexdump tail head grep sed cut wget stat find readlink wc sh; do found=no; for dir in /bin /sbin /usr/bin /usr/sbin /mnt/sdcard/opentom/bin; do if test -x $dir/$applet; then echo FOUND:$dir/$applet; found=yes; break; fi; done; if test $found = no; then echo NO_EXECUTABLE:$applet; fi; done")
        run("remote-failure-propagation", "false", 1)
        run("remote-multiline", "echo first\necho second\ntest 3 -eq 3")
        device.checked("mkdir " + shlex.quote(stage))
        try:
            for source, name in [(root / "build/platform-probe", "platform-probe"),
                                 (root / "device/scripts/platform-smoke.sh", "platform-smoke.sh")]:
                destination = stage + "/" + name
                uploaded.append(destination)
                device.upload(source, destination, args.bind)
            probe = stage + "/platform-probe"
            device.checked("chmod 700 " + shlex.quote(probe))
            run("probe-usage", shlex.quote(probe), 2)
            run("probe-selftest", shlex.quote(probe) + " --selftest")
            run("capabilities", shlex.quote(probe) + " --probe")
            run("shell-smoke", "/bin/sh " + stage + "/platform-smoke.sh " + probe)
            run("hex-reject-directory", probe + " --hex /dev", 1)
            run("hex-reject-missing", probe + " --hex " + stage + "/missing", 1)
            run("hex-reject-large-range", probe + " --hex " + probe + " 0 65537", 2)
            run("hex-reject-negative-range", probe + " --hex " + probe + " -1 20", 2)
            run("shell-reject-missing-probe", "/bin/sh " + stage + "/platform-smoke.sh " + stage + "/missing", 1)
            try:
                device.upload(root / "build/platform-probe", probe, args.bind)
            except RuntimeError as error:
                if not str(error).startswith("Remote exit 1:"):
                    raise
                report["checks"].append({"name": "upload-rejects-existing-target", "passed": True})
                print("PASS upload rejects an existing target", flush=True)
            else:
                raise RuntimeError("Upload overwrote existing target")
            for name, source in [("platform-probe", root / "build/platform-probe"),
                                 ("platform-smoke.sh", root / "device/scripts/platform-smoke.sh")]:
                digest = device.download(stage + "/" + name, evidence / (name + ".readback"), probe)
                expected = hashlib.sha256(source.read_bytes()).hexdigest()
                if digest != expected:
                    raise RuntimeError(name + " transfer digest mismatch")
                report["checks"].append({"name": name + "-transfer-sha256", "passed": True, "sha256": digest})
                print(name + ": readback SHA-256 matches", flush=True)
            # Boot/application launch files only; no keys, account files or logs.
            for remote, local in [
                ("/mnt/sdcard/ttsystem", "ttsystem"),
                ("/etc/rc", "init-rc"),
                ("/mnt/sdcard/opentom/start.sh", "opentom-start.sh"),
                ("/mnt/sdcard/opentom/etc/power-button.cfg", "power-button.cfg"),
                ("/bin/busybox", "device-busybox"),
                ("/lib/ld-linux.so.2", "device-loader"),
            ]:
                digest = device.download(remote, evidence / local, probe)
                report["backups"][remote] = {"file": local, "sha256": digest, "bytes": (evidence / local).stat().st_size}
                print("Backed up:", remote, flush=True)
            run("final-health", "ps; cat /proc/meminfo; fbset")
            report["complete"] = True
        finally:
            for remote in uploaded:
                device.checked("rm -f " + shlex.quote(remote))
            device.checked("rmdir " + shlex.quote(stage))
            report["temporary_device_files_removed"] = True
            (evidence / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print("PASS all requested platform checks; capability limitations are in capabilities.txt", flush=True)
    print("No boot/startup configuration changed. Temporary probe/scripts removed.", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
