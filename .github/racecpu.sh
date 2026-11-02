#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Blair Hamilton
# SPDX-License-Identifier: Apache-2.0
#
# TEMPORARY (#7290 measurement): a `go test -exec` wrapper. Runs the test
# binary, then records the CPU it and every child it waited for used. go test
# runs a test binary in its package's directory, so $PWD names the package.
start=$(date +%s.%N)
"$@"
rc=$?
end=$(date +%s.%N)
tf=$(mktemp)
times >"$tf" # in this shell: a subshell would report its own children, none
read -r cu cs < <(sed -n 2p "$tf")
rm -f "$tf"
secs() { local m=${1%%m*} s=${1#*m}; s=${s%s}; awk -v m="$m" -v s="$s" 'BEGIN{print m*60+s}'; }
printf '%s\t%s\t%s\t%s\n' "${PWD#"${GITHUB_WORKSPACE:-}"/}" "$(secs "$cu")" "$(secs "$cs")" \
  "$(awk -v a="$start" -v b="$end" 'BEGIN{printf "%.1f", b-a}')" >>"${RUNNER_TEMP:-/tmp}/racetimeline/cpu.tsv"
exit "$rc"
