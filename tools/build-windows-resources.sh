#!/bin/sh
# Build-only dependency, pinned; the shipped host app still uses standard Go.
set -eu
project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$project_dir/build/tools"
if test ! -x "$project_dir/build/tools/go-winres"; then
 GOBIN="$project_dir/build/tools" go install github.com/tc-hib/go-winres@v0.3.3
fi
cd "$project_dir/host"
"$project_dir/build/tools/go-winres" make --arch amd64,arm64 --file-version 0.1.10.0 --product-version 0.1.10.0
