#!/usr/bin/env python3
"""Verify pinned legacy SDK inputs and repeatable ARMv5 device builds."""
import argparse
import hashlib
import json
import os
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_SDK = Path("/home/sepisotoni/projects/LeddaZ-OpenTom")
TOOLCHAIN = "gcc-3.3.4_glibc-2.3.2"


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--sdk",
        type=Path,
        default=Path(os.environ.get("FRACTURE_SDK", DEFAULT_SDK)),
        help="SDK root containing the pinned archive, compiler, sysroot, and kernel headers",
    )
    args = parser.parse_args()
    sdk = args.sdk.expanduser().resolve()

    provenance_path = ROOT / "docs/TOOLCHAIN-PROVENANCE.json"
    provenance = json.loads(provenance_path.read_text(encoding="utf-8"))
    failures = []
    for relative_path, expected_hash in provenance["files"].items():
        path = sdk / relative_path
        if not path.is_file():
            failures.append(f"missing: {relative_path}")
            continue
        actual_hash = sha256(path)
        if actual_hash != expected_hash:
            failures.append(f"hash mismatch: {relative_path} ({actual_hash})")
    if failures:
        raise SystemExit("SDK provenance check failed:\n  " + "\n  ".join(failures))

    compiler = sdk / TOOLCHAIN / "bin/arm-linux-gcc"
    strip = sdk / TOOLCHAIN / "bin/arm-linux-strip"
    include = sdk / "kernel/include"
    sources = (
        ("fracture-device", ROOT / "device/src/fracture.c"),
        ("platform-probe", ROOT / "device/diagnostics/platform_probe.c"),
    )
    with tempfile.TemporaryDirectory(prefix="fracture-legacy-sdk-") as temporary:
        output = Path(temporary)
        build_hashes = []
        for build_number in (1, 2):
            build_dir = output / str(build_number)
            build_dir.mkdir()
            hashes = {}
            for name, source in sources:
                binary = build_dir / name
                subprocess.run(
                    [
                        str(compiler),
                        "-std=gnu99",
                        "-Os",
                        "-Wall",
                        "-W",
                        "-Werror",
                        "-march=armv5te",
                        "-mtune=arm9tdmi",
                        "-idirafter",
                        str(include),
                        str(source),
                        "-o",
                        str(binary),
                    ],
                    cwd=ROOT,
                    check=True,
                )
                subprocess.run([str(strip), str(binary)], check=True)
                hashes[name] = sha256(binary)
            build_hashes.append(hashes)

    if build_hashes[0] != build_hashes[1]:
        raise SystemExit(f"Repeated builds differ:\n{build_hashes[0]}\n{build_hashes[1]}")

    print(f"Verified pinned legacy SDK: {sdk}")
    for name, digest in build_hashes[0].items():
        print(f"Repeatable ARMv5 build {name}: {digest}")
    print("Both stripped outputs matched byte-for-byte across isolated builds.")


if __name__ == "__main__":
    main()
