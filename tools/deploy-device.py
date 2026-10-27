#!/usr/bin/env python3
"""Verified deployment of the 0.1.10 application environment, not firmware flashing."""
import argparse
import datetime
import hashlib
import json
import re
import secrets
import shlex
import socket
import time
from pathlib import Path
from tomtom import TomTom

ROOT = '/mnt/sdcard/fracture-v3'
START = '/mnt/sdcard/opentom/start.sh'
VERSION = 'v0.1.10'


def control(device, cmd):
    return device.checked('/bin/sh ' + ROOT + '/control.sh ' + cmd)


def restore(device):
    device.checked('test -s ' + ROOT + '/stock-start.sh')
    device.checked('cp ' + ROOT + '/stock-start.sh ' + START + '.restore && sync && mv ' + START + '.restore ' + START)
    device.checked('if test -f '+ROOT+'/tomtom.ico.original; then cp '+ROOT+'/tomtom.ico.original /mnt/sdcard/tomtom.ico; fi')
    device.checked('touch ' + ROOT + '/disabled')
    device.checked('if test -f /tmp/fracture-v3.pid; then kill $(cat /tmp/fracture-v3.pid); fi')
    # During a live takeover, stopped original shells retain zombie children.
    # BusyBox pidof counts those zombies and can prevent OpenTom restarting.
    # Resume the preserved original chain so it can reap and restart its jobs;
    # pause the background fallback wrapper to avoid duplicate supervisors.
    ps = device.checked('ps')
    preserved = [line.split()[0] for line in ps.splitlines()
                 if 'sh '+START in line and len(line.split())>=4 and 'T' in line.split()[3]]
    if preserved:
        device.checked('if test -f /tmp/fracture-supervisor.pid; then kill -STOP $(cat /tmp/fracture-supervisor.pid); fi')
        for pid in preserved:
            if pid.isdigit() and int(pid)>1: device.checked('kill -CONT '+pid)
    # boot.sh handles cold-boot fallback itself. Never kill PID 1/startup chain.
    deadline = time.monotonic()+20
    while time.monotonic()<deadline:
        out = device.checked('ps')
        if any('nano-X' in line and len(line.split())>=4 and 'Z' not in line.split()[3] for line in out.splitlines()): break
        time.sleep(1)
    if not any('nano-X' in line and len(line.split()) >= 4 and 'Z' not in line.split()[3] for line in out.splitlines()):
        raise RuntimeError('Original startup restored on disk, but Nano-X recovery was not observed')
    print('Restored original OpenTom startup and observed Nano-X running. V3 files retained for recovery.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['install', 'restore'])
    parser.add_argument('--host', default='192.168.101.115')
    parser.add_argument('--package', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--evidence', type=Path)
    args = parser.parse_args()
    socket.inet_aton(args.host)
    with TomTom(args.host) as device:
        if args.action == 'restore':
            restore(device)
            return
        identity = device.checked('uname -m; uname -r; cat /proc/barcelona/usbname')
        if 'armv5' not in identity or '2.6.13' not in identity or 'ONE (v6)' not in identity:
            raise RuntimeError('This device release supports the validated OpenTom ONE (v6), Linux 2.6.13 only: ' + identity)
        previous_current = device.command('cat '+ROOT+'/current 2>/dev/null')[1].strip()
        # A new private backup directory on the modern host, outside public files.
        backup = args.evidence or (Path.home() / '.local/share/fracture-v3/backups' / (datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + secrets.token_hex(3)))
        backup.mkdir(parents=True, mode=0o700, exist_ok=False)
        payloads = {'fracture-device': args.package/'build/fracture-device', 'platform-probe': args.package/'build/platform-probe',
                    'tomtom.ico': args.package/'assets/branding/fracture.ico', 'boot.sh': args.package/'device/scripts/boot.sh', 'control.sh': args.package/'device/scripts/control.sh', 'entry.sh': args.package/'device/scripts/entry.sh'}
        for path in sorted((args.package/'assets/faces').glob('*')):
            if path.suffix in ('.face', '.pgm'): payloads['faces/'+path.name] = path
        for path in sorted((args.package/'assets/themes').glob('*.theme')):
            payloads['themes/'+path.name] = path
        for path in sorted((args.package/'assets/ui').glob('*.bin')):
            payloads['ui/'+path.name] = path
        for path in payloads.values():
            if not path.is_file(): raise RuntimeError('Missing release payload: ' + str(path))
        stage = '/tmp/fracture-install-' + secrets.token_hex(6)
        route = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        route.connect((args.host, 23)); bind = route.getsockname()[0]; route.close()
        device.checked('mkdir ' + stage + ' ' + stage+'/faces '+stage+'/themes '+stage+'/ui')
        uploaded = []
        changed = False
        report = {'version': VERSION, 'device': args.host, 'payloads': {}, 'backup': {}}
        try:
            for name,path in payloads.items():
                destination = stage+'/'+name
                uploaded.append(destination)
                device.upload(path, destination, bind)
            probe = stage+'/platform-probe'
            device.checked('chmod 700 ' + probe + ' ' + stage+'/fracture-device')
            print(device.checked(probe+' --selftest'), end='')
            for name,path in payloads.items():
                digest = device.download(stage+'/'+name, backup/(name.replace('/','_')+'.readback'), probe)
                if digest != hashlib.sha256(path.read_bytes()).hexdigest(): raise RuntimeError('Transfer verification failed: '+name)
                report['payloads'][name] = digest
            for remote,local in [(START,'start.sh.before'),('/mnt/sdcard/ttsystem','ttsystem.before'),('/etc/rc','init.before'),('/mnt/sdcard/tomtom.ico','tomtom.ico.before')]:
                report['backup'][remote] = device.download(remote, backup/local, probe)
            if 'FRACTURE_V3_BOOT_ENTRY' not in (backup/'start.sh.before').read_text():
                device.checked('mkdir -p '+ROOT+'/releases')
                code,_ = device.command('test -e '+ROOT+'/stock-start.sh')
                if code == 0:
                    original = backup/'saved-original'
                    device.download(ROOT+'/stock-start.sh', original, probe)
                    if original.read_bytes() != (backup/'start.sh.before').read_bytes():
                        raise RuntimeError('Existing saved original differs from current OpenTom startup; preserving both and stopping')
                else:
                    device.checked('cp '+START+' '+ROOT+'/stock-start.sh && sync')
            else:
                device.checked('test -s '+ROOT+'/stock-start.sh')
            destination = ROOT+'/releases/'+VERSION
            # Never overwrite a running executable or silently republish same version.
            exists,_ = device.command('test -d '+destination)
            if exists == 0:
                for name in [n for n in payloads if n not in ('boot.sh','control.sh','entry.sh')]:
                    digest = device.download(destination+'/'+name, backup/(name.replace('/','_')+'.installed'),probe)
                    if digest != report['payloads'][name]: raise RuntimeError('Installed '+VERSION+' differs; use a new release version')
            else:
                device.checked('mkdir '+destination+' '+destination+'/faces '+destination+'/themes '+destination+'/ui')
                for name in [n for n in payloads if n not in ('boot.sh','control.sh','entry.sh')]:
                    device.checked('cp '+stage+'/'+name+' '+destination+'/'+name)
            device.checked('mkdir -p '+ROOT+'/faces '+ROOT+'/themes')
            for name in ['boot.sh','control.sh']:
                device.checked('cp '+stage+'/'+name+' '+ROOT+'/'+name+'.new && sync && mv '+ROOT+'/'+name+'.new '+ROOT+'/'+name)
            for name,value in [('device-ip',args.host),('current',VERSION)]:
                device.checked('echo '+shlex.quote(value)+' > '+ROOT+'/'+name+'.new && sync && mv '+ROOT+'/'+name+'.new '+ROOT+'/'+name)
            offset = int(datetime.datetime.now().astimezone().utcoffset().total_seconds()/60)
            device.checked('if test ! -f '+ROOT+'/settings.cfg; then echo '+shlex.quote('version=1\noffset='+str(offset)+'\nbrightness=30\nprofile=0\ntheme=0\ntheme_name=midnight\nalarm_hour=-1\nalarm_minute=0\nlast_alarm_day=-1')+' > '+ROOT+'/settings.cfg; fi')
            device.checked('cp '+stage+'/entry.sh '+START+'.new && sync && mv '+START+'.new '+START)
            changed = True
            # Preserve foreground parent shells so /etc/rc cannot reach force_reboot.
            ps = device.checked('ps')
            for line in ps.splitlines():
                if 'sh '+START in line or 'sh '+ROOT+'/stock-start.sh' in line:
                    pid = line.split()[0]
                    if pid.isdigit() and int(pid)>1: device.checked('kill -STOP '+pid)
            for name in ['power_button','watchface.new','nano-X','nanowm','nxmenu','weather-sync','tt-control','gltt']:
                device.checked('killall '+name+' 2>/dev/null || true')
            device.checked('rm -f '+ROOT+'/disabled')
            # If there is a stopped/restored old v3 supervisor, its PID may now run
            # OpenTom. Start a fresh v3 wrapper only when there is no active clock.
            code,out = device.command('/bin/sh '+ROOT+'/control.sh status')
            if code == 0 and (json.loads(out).get('version') != VERSION[1:] or previous_current != VERSION):
                device.checked('kill $(cat /tmp/fracture-v3.pid)')
            elif code:
                device.checked('sh '+ROOT+'/boot.sh </dev/null >/tmp/fracture-v3.log 2>&1 &')
            deadline = time.monotonic()+12
            while time.monotonic()<deadline:
                code,out = device.command('/bin/sh '+ROOT+'/control.sh status')
                if code == 0 and json.loads(out).get('version') == VERSION[1:]:
                    report['status'] = json.loads(out)
                    break
                time.sleep(0.5)
            else: raise RuntimeError('Clock did not become healthy')
            if report['status']['version'] != VERSION[1:]: raise RuntimeError('Unexpected running version')
            device.checked('if test ! -f '+ROOT+'/tomtom.ico.original; then cp /mnt/sdcard/tomtom.ico '+ROOT+'/tomtom.ico.original; fi')
            device.checked('cp '+destination+'/tomtom.ico /mnt/sdcard/tomtom.ico.new && sync && mv /mnt/sdcard/tomtom.ico.new /mnt/sdcard/tomtom.ico')
            digest=device.download('/mnt/sdcard/tomtom.ico',backup/'tomtom.ico.installed',probe)
            if digest != report['payloads']['tomtom.ico']: raise RuntimeError('TomTom icon verification failed')
            device.checked('sync')
            report['installed'] = True
            print('FractureOS '+VERSION+' is running. Startup backed up at '+ROOT+'/stock-start.sh')
            print('Host backup:', backup)
            print(json.dumps(report['status']))
        except BaseException:
            if changed:
                print('Activation failed; restoring OpenTom.')
                restore(device)
            raise
        finally:
            # A USB disconnect must not hide the original error or lose recovery evidence.
            report['activation_changed'] = changed
            try:
                for path in uploaded: device.checked('rm -f '+path)
                device.checked('rmdir '+stage+'/faces '+stage+'/themes '+stage+'/ui '+stage)
            except Exception as cleanup_error:
                report['cleanup_deferred'] = {'stage': stage, 'files': uploaded, 'error': str(cleanup_error)}
                print('Temporary staging cleanup deferred until the device is reachable.')
            (backup/'deployment.json').write_text(json.dumps(report,indent=2)+'\n')


if __name__ == '__main__': main()
