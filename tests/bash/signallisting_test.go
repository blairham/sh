// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// TestTrapListsTheSameNumberedTableKillLists is #3474.
//
// Measured 2026-09-17 from a script file on macOS arm64, bash 5.3.20: `trap
// -l` writes the numbered table `kill -l` writes, byte for byte — ` 1)
// SIGHUP<tab> 2) SIGINT…`, five to a line — and bash 3.2.57 writes the same
// table in its own width. This shell wrote one bare name per line for `trap
// -l` while its `kill -l` already had the table, so one shell answered one
// question two ways.
//
// The equality is the assertion rather than a written-out table: the table is
// the host's, and a case holding this machine's could not pass on the other.
// What is written out is only the shape of the first row, which is this
// dialect's and not the platform's.
func TestTrapListsTheSameNumberedTableKillLists(t *testing.T) {
	fromTrap, st := answersRun(t, `trap -l`)
	if st != 0 {
		t.Fatalf("trap -l: status %d", st)
	}
	fromKill, st := answersRun(t, `kill -l`)
	if st != 0 {
		t.Fatalf("kill -l: status %d", st)
	}
	if fromTrap != fromKill {
		t.Errorf("trap -l wrote %q, kill -l wrote %q — one table, one listing", fromTrap, fromKill)
	}
	if !strings.HasPrefix(fromTrap, " 1) SIGHUP\t 2) SIGINT\t") {
		t.Errorf("trap -l began %q, want the numbered table", fromTrap)
	}
	// Five to a line, and the last row carries the separator it was written
	// with — measured: bash's final line is `31) SIGUSR2<tab>` on a machine
	// with 31 signals and `64) SIGRTMAX<tab>` on one with 64.
	lines := strings.Split(strings.TrimSuffix(fromTrap, "\n"), "\n")
	if got := len(strings.Split(lines[0], "\t")); got != 5 {
		t.Errorf("first row has %d entries, want 5", got)
	}
	if last := lines[len(lines)-1]; !strings.HasSuffix(last, "\t") && len(lines) > 1 {
		if got := len(strings.Split(last, "\t")); got != 5 {
			t.Errorf("last row %q neither fills a row nor carries its separator", last)
		}
	}
}

// TestThisPresetWritesNothingForASignalItCannotName is the one column that
// answers `kill -l N` for an unnamed signal with **nothing at all** rather
// than with the number.
//
// Nothing at all, and not an empty line — which is what this test and the
// axis comment both said while the code wrote a newline, and what cost
// `bash/builtins.tests` the whole file (#3984). Re-measured 2026-09-21 in
// the panel's pinned image, `kill -l 32 | od -c` under GNU bash 5.3.20 is an
// empty file.
//
// Measured 2026-09-17 in the panel's Alpine image with Debian-built bash 5.3
// beside dash 0.5.12, zsh 5.9 and BusyBox v1.37.0, which is where the
// question arises at all — Linux has 64 signals and names for 33 of them:
//
//	kill -l 32     (no output)       status 0   here
//	kill -l 32     32                status 0   dash, zsh, BusyBox ash
//	kill -l 160    (no output)       status 0   here — the same row less 128
//	kill -l 65     invalid signal specification, 1 — outside the kernel's range
//
// The last row is the reason this is not KillListPrintsANumberItCannotName:
// that axis is the same question asked *outside* the range, where this shell
// refuses and dash refuses too. Inside the range dash prints the number and
// this shell does not, so the two columns that agree out of range disagree in
// it (#3287).
func TestThisPresetWritesNothingForASignalItCannotName(t *testing.T) {
	if got := bash.Semantics().KillListLeavesAnUnnamedSignalBlank; got != interp.Yes {
		t.Errorf("KillListLeavesAnUnnamedSignalBlank is %v, want Yes", got)
	}
	if runtime.GOOS != "linux" {
		// macOS names every signal it has, so nothing here can reach the
		// answer through a number; interp's own cases reach it through a
		// vector missing a name instead.
		t.Skipf("no unnamed signal in range on %s", runtime.GOOS)
	}
	out, st := answersRun(t, `kill -l 32; echo "st=$?"`)
	if out != "st=0\n" {
		t.Errorf("kill -l 32 wrote %q at %d, want no output at all at 0", out, st)
	}
	// And the same row reached through the one subtraction of 128, which is
	// the shape `bash/builtins.tests` writes and the one that was failing.
	out, st = answersRun(t, `kill -l 160; echo "st=$?"`)
	if out != "st=0\n" {
		t.Errorf("kill -l 160 wrote %q at %d, want no output at all at 0", out, st)
	}
}
