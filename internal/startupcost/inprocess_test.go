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
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/zsh"
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

// The two dialects #6496 is about, built the way their binaries build them
// (less the protocol servers, which a `-c` run never reaches). The dash
// benchmarks above stay as they are so their history stays comparable.
func gateBashShell() driver.Shell {
	return driver.Shell{
		Name:                   "bash",
		SystemStartupDirectory: "/etc",
		Dialect:                bash.Dialect(),
		Semantics:              bash.Semantics(),
		Diagnostics:            bash.Diagnostics(),
		Prelude:                bash.Prelude(),
		PreludeTree:            bash.PreludeTree(),
		Register:               bash.Apply,
		PromptStyle:            bash.PromptStyle(),
		EditorStyle:            bash.EditorStyle(),
		HistoryStyle:           bash.HistoryStyle(),
		HookStyle:              bash.HookStyle(),
		Stdout:                 io.Discard,
		Stderr:                 io.Discard,
	}
}

func gateZshShell() driver.Shell {
	return driver.Shell{
		Name:                   "zsh",
		SystemStartupDirectory: zsh.SystemStartupDirectory("/etc"),
		Dialect:                zsh.Dialect(),
		Semantics:              zsh.Semantics(),
		Diagnostics:            zsh.Diagnostics(),
		Prelude:                zsh.Prelude(),
		PreludeTree:            zsh.PreludeTree(),
		CommandStringParameter: "ZSH_EXECUTION_STRING",
		Register:               zsh.Apply,
		PromptStyle:            zsh.PromptStyle(),
		EditorStyle:            zsh.EditorStyle(),
		HistoryStyle:           zsh.HistoryStyle(),
		HookStyle:              zsh.HookStyle(),
		MailStyle:              zsh.MailStyle(),
		Stdout:                 io.Discard,
		Stderr:                 io.Discard,
	}
}

func benchProgram(b *testing.B, sh driver.Shell, program string) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if status := driver.RunCommand(sh, program, nil); status != 0 {
			b.Fatalf("the program exited %d", status)
		}
	}
}

func BenchmarkGateWorkloadBash(b *testing.B) { benchProgram(b, gateBashShell(), inProcessWorkload) }
func BenchmarkGateBareBash(b *testing.B)     { benchProgram(b, gateBashShell(), ":") }
func BenchmarkGateWorkloadZsh(b *testing.B)  { benchProgram(b, gateZshShell(), inProcessWorkload) }
func BenchmarkGateBareZsh(b *testing.B)      { benchProgram(b, gateZshShell(), ":") }

// The workload's loop ten times over, so that what a profile of it shows is
// interpretation rather than the start each run also pays.
var inProcessLoop = strings.Replace(inProcessWorkload, "2000", "20000", 1)

func BenchmarkGateLoopBash(b *testing.B) { benchProgram(b, gateBashShell(), inProcessLoop) }
func BenchmarkGateLoopZsh(b *testing.B)  { benchProgram(b, gateZshShell(), inProcessLoop) }
