#!/usr/bin/env python3
"""Build release payloads from the checked local SDK and Go toolchain."""
import hashlib,json,os,shutil,subprocess,tarfile,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parent.parent
VERSION='v0.1.11'
OUT=ROOT/'build/release'
OUT.mkdir(parents=True,exist_ok=True)
subprocess.run(['sh','tools/build-device.sh'],cwd=ROOT,check=True)
subprocess.run(['sh','tools/build-windows-resources.sh'],cwd=ROOT,check=True)
packages=[]
for system in ('linux','windows'):
 for arch in ('amd64','arm64'):
  name=f'fracture-{VERSION}-{system}-{arch}'
  staging=ROOT/'build'/name
  staging.mkdir(parents=True,exist_ok=True)
  env=dict(os.environ,GOOS=system,GOARCH=arch,CGO_ENABLED='0')
  binary=staging/('bin/fracture' if system=='linux' else 'fracture.exe')
  binary.parent.mkdir(parents=True,exist_ok=True)
  flags='-s -w'+(' -H=windowsgui' if system=='windows' else '')
  subprocess.run(['go','build','-trimpath','-ldflags',flags,'-o',str(binary),'./host'],cwd=ROOT,env=env,check=True)
  if system=='linux':
   for file in ('tools/deploy-device.py','tools/tomtom.py','device/scripts/boot.sh','device/scripts/control.sh','device/scripts/entry.sh','build/fracture-device','build/platform-probe','install/uninstall.sh'):
    target=staging/file;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(ROOT/file,target)
   shutil.copytree(ROOT/'assets/faces',staging/'assets/faces',dirs_exist_ok=True)
   shutil.copytree(ROOT/'assets/themes',staging/'assets/themes',dirs_exist_ok=True)
   shutil.copytree(ROOT/'assets/ui',staging/'assets/ui',dirs_exist_ok=True)
   shutil.copytree(ROOT/'assets/branding',staging/'assets/branding',dirs_exist_ok=True)
   package=OUT/(name+'.tar.gz')
   with tarfile.open(package,'w:gz') as archive:
    for child in sorted(staging.iterdir()):archive.add(child,arcname=child.name)
  else:
   shutil.copy2(ROOT/'install/uninstall.ps1',staging/'uninstall.ps1')
   renderer=ROOT/'build'/f'windows-{arch}'/'fracture-preview.dll'
   if not renderer.is_file(): raise RuntimeError('Missing Windows shared renderer: '+str(renderer))
   shutil.copy2(renderer,staging/'fracture-preview.dll')
   (staging/'README.txt').write_text('FractureOS '+VERSION+'\nOpen fracture.exe for the tray. Right-click for Settings.\nWindows runtime validation is pending. See https://github.com/sepisotoni/FractureOS-v3\n')
   package=OUT/(name+'.zip')
   with zipfile.ZipFile(package,'w',zipfile.ZIP_DEFLATED) as archive:
    for child in sorted(staging.iterdir()):archive.write(child,child.name)
  packages.append(package)
manifest={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in packages}
(OUT/'SHA256SUMS').write_text(''.join(f'{sha}  {name}\n' for name,sha in manifest.items()))
(OUT/'build-info.json').write_text(json.dumps({'version':VERSION,'go':subprocess.check_output(['go','version'],text=True).strip(),'packages':manifest},indent=2)+'\n')
print(OUT)
