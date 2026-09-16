// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// TestThisPresetSendsASignalNumberItCannotName is the ksh93 half of #3139.
//
// A number out of range is not checked against the signal table here: it goes
// to `kill(2)`, and what comes back is this shell's one sentence for a send
// that failed, errno regardless. Measured 2026-09-16 against ksh93u+ (AJM
// 93u+ 2012-08-01) on macOS arm64, where 31 is the highest signal there is:
//
//	kill -99 $$     kill: <pid>: no such process       status 1
//	kill -n 99 $$   kill: <pid>: no such process       status 1
//	kill -s 99 $$   kill: 99: unknown signal name      status 1
//	kill -0 <gone>  kill: <pid>: no such process       status 1
//
// The third row is the reason the axis is not "never checks": `-s` takes a
// name, so digits there are already the wrong kind of word. The fourth is why
// the first is not a coincidence — the same sentence covers a real ESRCH, so
// this shell is lumping the errnos rather than reporting one.
//
// It was `kill: -99: unknown option` and a usage block at **2** before, which
// is dash's reading of the word and not this shell's.
func TestThisPresetSendsASignalNumberItCannotName(t *testing.T) {
	if got := ksh.Semantics().KillSendsASignalNumberItCannotName; got != interp.Yes {
		t.Errorf("KillSendsASignalNumberItCannotName is %v, want Yes", got)
	}
	for _, src := range []string{`kill -99 $$`, `kill -n 99 $$`} {
		out, _ := answersRun(t, src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, "no such process") || strings.Contains(out, "unknown option") {
			t.Errorf("%s said %q, want the failed-send sentence", src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", src, out)
		}
	}
	// `-s` takes a name, and a number there is refused before anything is
	// sent — a different sentence, and the row that stops this from being
	// read as "this shell never checks a signal".
	out, _ := answersRun(t, `kill -s 99 $$ 2>&1 >/dev/null; echo "st=$?"`)
	if !strings.Contains(out, "unknown signal name") {
		t.Errorf("kill -s 99 said %q, want the refusal", out)
	}
	// And a letter is still an option here, which is the half of the old
	// behaviour that was right: only the all-digit word was misread.
	if out, _ := answersRun(t, `kill -Q $$ 2>&1 >/dev/null; echo "st=$?"`); !strings.Contains(out, "unknown option") {
		t.Errorf("kill -Q said %q, want an option complaint", out)
	}
}
