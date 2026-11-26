#!/bin/sh
# Pixel-compares the device renderer with the PC preview renderer on the home
# screen, using the exact model/assets the device exports. Native x86-64 build.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
root=$(dirname "$here"); work=$(mktemp -d); trap 'rm -rf "$work"' EXIT
mkdir -p /mnt/sdcard/fracture-v3
cc="gcc -std=gnu99 -O1 -w -idirafter /usr/include"
$cc "$here/preview_drift_device.c" -o "$work/dev"
$cc -fPIC "$here/preview_drift_preview.c" -o "$work/prev"
python3 - "$work" <<'PY'
import sys,struct,random
random.seed(7); out=bytearray()
for y in range(240):
    for x in range(320):
        r=(x*31//319); g=(y*63//239); b=((x+y)*31//559)
        if (x//16+y//16)%7==0: r=min(31,r+6)
        out+=struct.pack('<H',(r<<11)|(g<<5)|b)
open(sys.argv[1]+'/wp.rgb','wb').write(out)
PY
fail=0
for scenario in "none:" "wallpaper:$work/wp.rgb"; do
  name=${scenario%%:*}; wp=${scenario#*:}
  for face in aqua neon sunset; do
    "$work/dev" "$root/assets/faces" "$work/$name-$face" "$wp" "$face"
    "$work/prev" "$work/$name-$face" "$work/$name-$face.rgba"
    python3 - "$work/$name-$face" "$name/$face" <<'PY' || fail=1
import sys,struct
p,label=sys.argv[1],sys.argv[2]
d=open(p+'.dev565','rb').read(); r=open(p+'.rgba','rb').read()
bad=0; box=[999,999,-1,-1]
for i in range(320*240):
    v=struct.unpack_from('<H',d,i*2)[0]
    e=((v&31)*255//31,((v>>5)&63)*255//63,((v>>11)&31)*255//31,255)
    if tuple(r[i*4:i*4+4])!=e:
        bad+=1; x,y=i%320,i//320
        box=[min(box[0],x),min(box[1],y),max(box[2],x),max(box[3],y)]
print(("PASS" if not bad else "FAIL"),label,"differing pixels:",bad,("bbox "+str(box) if bad else ""))
sys.exit(1 if bad else 0)
PY
  done
done
exit $fail
