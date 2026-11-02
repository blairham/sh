#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 Blair Hamilton
# SPDX-License-Identifier: Apache-2.0
#
# TEMPORARY (#7290 measurement): runs `go test "$@"` with an action trace and a
# CPU sampler beside it, then prints when each test binary ran and how busy the
# runner's CPUs were. Removed before the pull request merges.
set -u
out=${RUNNER_TEMP:-.}/racetimeline
mkdir -p "$out"
echo "nproc=$(nproc) GOMAXPROCS=${GOMAXPROCS:-unset} GOFLAGS=${GOFLAGS:-}"
vmstat -n 5 >"$out/vmstat.txt" &
vm=$!
start=$(date +%s.%N)
rc=0
: >"$out/cpu.tsv"
go test -debug-trace="$out/trace.json" -exec "$PWD/.github/racecpu.sh" "$@" || rc=$?
end=$(date +%s.%N)
kill "$vm" 2>/dev/null
echo "::group::racetimeline"
awk -v s="$start" -v e="$end" 'BEGIN{printf "go test wall %.1fs\n", e-s}'
jq -r -f "$(dirname "$0")/racetimeline.jq" "$out/trace.json"
echo "-- CPU (vmstat, 5 s samples; 30 s buckets): t busy%(us+sy) idle% wa% runq"
awk 'NR>3 { i=int((NR-4)/6); us[i]+=$13; sy[i]+=$14; id[i]+=$15; wa[i]+=$16; r[i]+=$1; n[i]++; tb+=$13+$14; tn++ }
     END { for (k=0; k in n; k++) printf "%4ds %5.1f %5.1f %4.1f %5.1f\n", k*30, (us[k]+sy[k])/n[k], id[k]/n[k], wa[k]/n[k], r[k]/n[k];
           if (tn) printf "mean busy %.1f%% over %d samples\n", tb/tn, tn }' "$out/vmstat.txt"
echo "-- CPU per test binary (user+sys, waited-for children included): cpu user sys wall cores package"
awk -F'\t' '{c=$2+$3; printf "%7.1f %7.1f %6.1f %6.1f %4.2f %s\n", c, $2, $3, $4, ($4>0?c/$4:0), $1}' "$out/cpu.tsv" | sort -k1,1 -gr | head -40
awk -F'\t' '{t+=$2+$3; w+=$4} END {printf "total test-binary cpu %.1fs over %d binaries, summed wall %.1fs\n", t, NR, w}' "$out/cpu.tsv"
echo "::endgroup::"
exit "$rc"
