#!/usr/bin/env python3
"""Release helper: one place to bump the version and to verify a release is consistent.

  python3 tools/release.py bump 0.1.19      # rewrite every version string (X.Y.Z or X.Y.Z.W)
  python3 tools/release.py check            # fail if anything disagrees with host/main.go
  python3 tools/release.py check --tag v0.1.19   # also verify a tag/release matches

Never edit version strings by hand: run bump, then check.
"""
import re, subprocess, sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
VER = r'[0-9]+\.[0-9]+\.[0-9]+(?:\.[0-9]+)?'
# (file, regex with one capture group = the version, how it is written)
SITES = [
    ('host/main.go',                   r'const version = "(%s)"' % VER, 'plain'),
    ('host/main.go',                   r'FractureOS (%s)\n' % VER, 'plain'),
    ('host/ui.html',                   r'TomTom companion · (%s)' % VER, 'plain'),
    ('host/ui.html',                   r'FractureOS (%s) · ' % VER, 'plain'),
    ('device/src/fracture.c',          r'#define FRACTURE_VERSION "(%s)"' % VER, 'plain'),
    ('install/fracture.iss',           r'#define AppVersion "(%s)"' % VER, 'plain'),
    ('install/install.ps1',            r"'v(%s)' }" % VER, 'plain'),
    ('install/install.sh',             r'FRACTURE_VERSION:-v(%s)' % VER, 'plain'),
    ('tools/package-release.py',       r"VERSION='v(%s)'" % VER, 'plain'),
    ('tools/deploy-device.py',         r"VERSION = 'v(%s)'" % VER, 'plain'),
    ('tools/deploy-device.py',         r'Verified deployment of the (%s) application' % VER, 'plain'),
    ('tools/build-windows-resources.sh', r'--file-version (%s)' % VER, 'win'),
    ('tools/build-windows-resources.sh', r'--product-version (%s)' % VER, 'win'),
    ('.github/workflows/windows-installer.yml', r'--file-version (%s)' % VER, 'win'),
    ('.github/workflows/windows-installer.yml', r'--product-version (%s)' % VER, 'win'),
]

def win4(v):
    """Windows file versions always have four parts."""
    return v if v.count('.') == 3 else v + '.0'

def current():
    m = re.search(r'const version = "(%s)"' % VER, (ROOT / 'host/main.go').read_text())
    if not m:
        sys.exit('cannot find version in host/main.go')
    return m.group(1)

def bump(new):
    if not re.fullmatch(VER, new):
        sys.exit('version must be X.Y.Z or X.Y.Z.W')
    for path, pat, kind in SITES:
        p = ROOT / path
        text = p.read_text(encoding='utf-8')
        value = win4(new) if kind == 'win' else new
        text, n = re.subn(pat, lambda m: m.group(0).replace(m.group(1), value, 1), text)
        if n == 0:
            sys.exit(f'{path}: pattern not found: {pat}')
        p.write_text(text, encoding='utf-8')
    print('bumped to', new, '- now add a changelog entry in host/changelog.go and run: check')

def check(tag=None):
    want, bad = current(), []
    for path, pat, kind in SITES:
        text = (ROOT / path).read_text(encoding='utf-8')
        found = re.findall(pat, text)
        expect = win4(want) if kind == 'win' else want
        if not found:
            bad.append(f'{path}: version pattern missing')
        for f in found:
            if f != expect:
                bad.append(f'{path}: {f} != {expect}')
    notes = (ROOT / 'host/changelog.go').read_text(encoding='utf-8')
    if not re.search(r'Version:\s*"v%s\b' % re.escape(want), notes):
        bad.append(f'host/changelog.go: no entry for v{want}')
    if tag and tag != 'v' + want:
        bad.append(f'tag {tag} != v{want}')
    if bad:
        print('\n'.join(bad)); sys.exit(1)
    print('version', want, 'consistent in', len(SITES), 'places + changelog')

if __name__ == '__main__':
    a = sys.argv[1:]
    if a[:1] == ['bump'] and len(a) == 2: bump(a[1])
    elif a[:1] == ['check']: check(a[a.index('--tag') + 1] if '--tag' in a else None)
    else: sys.exit(__doc__)
