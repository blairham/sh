// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A leading `--` ends `.`'s options in every column, so it is read before
// [Semantics.DotReadsOptions] is asked rather than behind it.
//
// Measured 2026-09-22 with an absolute operand, so the PATH search two of the
// columns do is not a confound: `. -- /abs/f.sh` sources the file at 0 in zsh
// 5.9.2, bash 5.3, that binary as `sh`, bash 3.2, dash and ksh93 alike, and
// `source -- /abs/f.sh` agrees everywhere the builtin exists — dash has no
// `source` at all.
//
// It was behind the gate, so the one dialect that answers `DotReadsOptions`
// with No — zsh, which reads `. -p dir f` as a complaint about a file called
// `-p` — took the `--` for the filename too. That is not what it does:
// measured, `source -- f` sources f there while `source -x f` and a *second*
// `--` are both a missing file, so the shell reads exactly one option and it
// is this one. F-Sy-H opens with `builtin source -- "$dir/lib/lifecycle.zsh"
// || return`, so a real `~/.zshrc` printed `no such file or directory: --`
// and loaded no highlighting (#4210).
func TestALeadingDashDashEndsDotsOptionsUnderEitherAnswer(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.sh"), []byte("echo SOURCED\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, reads := range []Answer{Yes, No} {
		t.Run("DotReadsOptions="+reads.String(), func(t *testing.T) {
			sem := permissive()
			sem.DotReadsOptions = reads
			out, st := sourceRun(t, dir, ". -- ./f.sh\n", sem, Diagnostics{})
			if got := strings.TrimSpace(out); got != "SOURCED" || st != 0 {
				t.Errorf(". -- ./f.sh = %q (status %d), want %q at 0", got, st, "SOURCED")
			}
		})
	}
}

// One `--` and one only: the second is an operand, which is what says the
// first is end-of-options rather than a word the builtin skips over. Under
// either answer, because the lift above the gate must not have turned into a
// loop that eats them.
func TestASecondDashDashIsTheOperand(t *testing.T) {
	dir := t.TempDir()
	for _, reads := range []Answer{Yes, No} {
		t.Run("DotReadsOptions="+reads.String(), func(t *testing.T) {
			sem := permissive()
			sem.DotReadsOptions = reads
			dg := Diagnostics{DotCannotOpen: ".: %[1]s: %[2]s"}
			out, st := sourceRun(t, dir, ". -- -- ./f.sh\n", sem, dg)
			if !strings.Contains(out, "--") || st == 0 {
				t.Errorf(". -- -- ./f.sh = %q (status %d), want it to fail naming %q", out, st, "--")
			}
		})
	}
}

// `--` with nothing behind it is the complaint a bare `.` gets and not a
// sourced nothing: measured, zsh says `not enough arguments` at 1 for
// `source --` exactly as it does for `.` alone.
func TestDashDashWithNoOperandIsTheNoOperandComplaint(t *testing.T) {
	dir := t.TempDir()
	sem := permissive()
	sem.DotReadsOptions = No
	dg := Diagnostics{DotNoOperand: ".: not enough arguments"}
	bare, bst := sourceRun(t, dir, ".\n", sem, dg)
	only, ost := sourceRun(t, dir, ". --\n", sem, dg)
	if bare != only || bst != ost {
		t.Errorf(". -- = %q (status %d), want the same as a bare `.`: %q (status %d)", only, ost, bare, bst)
	}
	if bst == 0 {
		t.Errorf("a bare `.` reported success, so this suite is asserting nothing")
	}
}
