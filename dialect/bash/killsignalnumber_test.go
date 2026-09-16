// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// TestThisPresetChecksASignalNumberBeforeSending is the column #3139 did not
// touch, pinned so that a fix for the other three cannot reach it.
//
// A number out of range is checked against this shell's own table and refused
// there, however it was written, and the number never reaches a system call.
// Measured 2026-09-16 against bash 5.3.20 and bash 3.2.57 alike on macOS
// arm64, where 31 is the highest signal there is — all three forms, both
// builds, one sentence:
//
//	kill -99 $$ / kill -n 99 $$ / kill -s 99 $$
//	    kill: 99: invalid signal specification    status 1
//
// ksh93 and zsh answer the other way: they hand the number to `kill(2)` and
// report what came back. So this is a real split and not a wording, which is
// why it is an axis rather than a Diagnostics string.
func TestThisPresetChecksASignalNumberBeforeSending(t *testing.T) {
	if got := bash.Semantics().KillSendsASignalNumberItCannotName; got != interp.No {
		t.Errorf("KillSendsASignalNumberItCannotName is %v, want No", got)
	}
	for _, src := range []string{`kill -99 $$`, `kill -n 99 $$`, `kill -s 99 $$`} {
		out, _ := answersRun(t, src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, "kill: 99: invalid signal specification") {
			t.Errorf("%s said %q, want the refusal", src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", src, out)
		}
	}
}
