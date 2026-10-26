#!/bin/sh
# Deliberately limited to builtins and applets present in the inspected BusyBox.
set -eu
probe=${1:?Usage: sh platform-smoke.sh /absolute/path/to/platform-probe}
test -x "$probe"
echo 'FRACTURE_SCRIPT_SMOKE_V1'
test "$(basename /tmp/fracture-probe)" = fracture-probe
test "$(echo abc | cut -c 2)" = b
test "$(echo abc | sed 's/b/X/')" = aXc
echo abc | grep -q abc
echo 'PASS shell.builtins_and_available_text_applets'
if (exit 7); then
    echo 'FAIL shell.exit_status' >&2
    exit 1
else
    test "$?" = 7
fi
echo 'PASS shell.exit_status'
"$probe" --selftest
"$probe" --probe
echo 'PASS platform-smoke.complete'
