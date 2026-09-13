// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// The gate's own workload, timed in process.
//
// The gate above spawns binaries, which is the honest way to answer "is our
// dash slower than dash" and the useless way to answer "why". A spawn is
// ~2.2ms of runtime and loader before any of this repository's code runs, so a
// profile of it is a profile of exec. These two benchmarks run the same two
// programs through the same `-c` entry point with no process in the way, which
// is what makes the interpreter's share of #1403 profilable.
package startupcost_test

import (
	"io"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/driver"
)

// inProcessWorkload is workloadProgram, kept separate only because this file
// must not depend on the order the package's constants are declared in.
const inProcessWorkload = `i=0
n=0
while [ $i -lt 2000 ]; do
	i=$((i + 1))
	n=$((n + 1))
done
for w in a b c d e f g h i j; do
	case $w in
	a|e|i|o|u) n=$((n + 1)) ;;
	*) n=$((n + 2)) ;;
	esac
done
s=abcdefghij
printf 'ANS:%s:%s:%s\n' "$n" "${s#abc}" "${s%hij}"
`

func dashShell() driver.Shell {
	return driver.Shell{
		Name:                   "dash",
		SystemStartupDirectory: "/etc",
		Dialect:                dash.Dialect(),
		Semantics:              dash.Semantics(),
		Diagnostics:            dash.Diagnostics(),
		Prelude:                dash.Prelude(),
		Register:               dash.Apply,
		PromptStyle:            dash.PromptStyle(),
		EditorStyle:            dash.EditorStyle(),
		HistoryStyle:           dash.HistoryStyle(),
		Stdout:                 io.Discard,
		Stderr:                 io.Discard,
	}
}

func BenchmarkGateWorkload(b *testing.B) {
	sh := dashShell()
	b.ReportAllocs()
	for b.Loop() {
		if status := driver.RunCommand(sh, inProcessWorkload, nil); status != 0 {
			b.Fatalf("the workload exited %d", status)
		}
	}
}

// BenchmarkGateBare is what every subshell pays with no work in it at all.
func BenchmarkGateBare(b *testing.B) {
	sh := dashShell()
	b.ReportAllocs()
	for b.Loop() {
		if status := driver.RunCommand(sh, ":", nil); status != 0 {
			b.Fatalf("the bare program exited %d", status)
		}
	}
}
