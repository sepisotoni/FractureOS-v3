#!/bin/sh
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
sdk_dir=${FRACTURE_SDK:-/home/sepisotoni/projects/LeddaZ-OpenTom}
compiler="$sdk_dir/gcc-3.3.4_glibc-2.3.2/bin/arm-linux-gcc"
test -x "$compiler" || { echo "Missing legacy compiler: $compiler" >&2; exit 1; }
mkdir -p "$project_dir/build"
"$compiler" -std=gnu99 -Os -Wall -W -Werror -march=armv5te -mtune=arm9tdmi \
    -idirafter "$sdk_dir/kernel/include" \
    "$project_dir/device/diagnostics/platform_probe.c" \
    -o "$project_dir/build/platform-probe"
"$sdk_dir/gcc-3.3.4_glibc-2.3.2/bin/arm-linux-strip" "$project_dir/build/platform-probe"
file "$project_dir/build/platform-probe"
sha256sum "$project_dir/build/platform-probe"
