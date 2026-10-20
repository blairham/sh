// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// TestThisPresetSendsASignalNumberItCannotName is the zsh half of #3139.
//
// A number out of range is not checked against the signal table here: it goes
// to `kill(2)`, and this shell prints the errno the kernel answered with.
// Measured 2026-09-16 against zsh 5.9.2 on macOS arm64, where 31 is the
// highest signal there is:
//
//	kill -99 $$     kill <pid> failed: invalid argument   status 1
//	kill -n 99 $$   kill <pid> failed: invalid argument   status 1
//	kill -s 99 $$   unknown signal: SIG99                 status 1
//	kill -0 <gone>  kill <pid> failed: no such process    status 1
//
// The first and the fourth are the same sentence with a different errno in
// it, which is what makes the errno the thing to assert: a row reading only
// the status would pass in a shell that reported the wrong one. The third is
// why the axis is not "never checks" — `-s` takes a name, so digits there are
// already the wrong kind of word.
//
// It was `unknown signal: SIG99` for all three before, which is the `-s`
// answer given to a word that never went near the signal table in the real
// shell.
func TestThisPresetSendsASignalNumberItCannotName(t *testing.T) {
	if got := zsh.Semantics().KillSendsASignalNumberItCannotName; got != interp.Yes {
		t.Errorf("KillSendsASignalNumberItCannotName is %v, want Yes", got)
	}
	for _, src := range []string{`kill -99 $$`, `kill -n 99 $$`} {
		out, _ := runZsh(t, t.TempDir(), src+" 2>&1 >/dev/null; echo \"st=$?\"\n")
		if !strings.Contains(out, "failed: invalid argument") {
			t.Errorf("%s said %q, want the errno spelled out", src, out)
		}
		if strings.Contains(out, "unknown signal") {
			t.Errorf("%s said %q, and the number never reached the signal table here", src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", src, out)
		}
	}
	// `-s` takes a name, and a number there is refused before anything is
	// sent — with the hint after it, which the failed send does not have.
	out, _ := runZsh(t, t.TempDir(), "kill -s 99 $$ 2>&1 >/dev/null; echo \"st=$?\"\n")
	if !strings.Contains(out, "unknown signal: SIG99") || !strings.Contains(out, "kill -L") {
		t.Errorf("kill -s 99 said %q, want the refusal and its hint", out)
	}
	// And a pid that is really absent, so the errno above is known to be the
	// kernel's word and not this shell's one sentence for every failure.
	out, _ = runZsh(t, t.TempDir(), "kill -0 4194303 2>&1 >/dev/null\n")
	if !strings.Contains(out, "failed: no such process") {
		t.Errorf("kill -0 on an absent pid said %q, want a different errno", out)
	}
}
