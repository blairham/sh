// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// TestThisPresetHasNoNameForSignal29 records a table that is one name short
// of the machine's.
//
// Measured 2026-09-17 on ksh93u+ (AJM 93u+ 2012-08-01) on macOS arm64, under
// a matching `argv[0]`, where signal 29 is INFO to every other column:
//
//	kill -l 29      29                                     status 0
//	kill -l INFO    kill: INFO: unknown signal name        status 1
//	trap 'x' INFO   trap: INFO: bad trap                   status 1
//	trap 'x' 29                                            status 0
//	kill -l EMT     7                                      status 0
//
// So the gap is in the *names* and not in the signals: the same shell that
// will not read the word takes the number for it, sets a trap on it and sends
// it. That is why Semantics.SignalNamesTheShellLacks is consulted where a
// name is read and nowhere a number is.
//
// EMT is the control. This engine carried neither name in any dialect until
// the table became the platform's, so a case asserting only the absence would
// pass on the build that had no platform table at all.
func TestThisPresetHasNoNameForSignal29(t *testing.T) {
	if got := ksh.Semantics().SignalNamesTheShellLacks; got != "INFO" {
		t.Errorf("SignalNamesTheShellLacks is %q, want INFO", got)
	}
	if runtime.GOOS != "darwin" {
		// 29 is IO on Linux and every column names it, so there is nothing
		// to see there — the preset carries the name either way, because a
		// name the platform does not have is never looked up.
		t.Skipf("signal 29 is only INFO on darwin, not on %s", runtime.GOOS)
	}
	if out, _ := answersRun(t, `kill -l 29`); strings.TrimSpace(out) != "29" {
		t.Errorf("kill -l 29 said %q, want the number back", out)
	}
	if out, _ := answersRun(t, `kill -l EMT 2>&1`); strings.TrimSpace(out) != "7" {
		t.Errorf("kill -l EMT said %q, want 7 — the platform's other extra name", out)
	}
	if out, _ := answersRun(t, `kill -l INFO 2>&1 >/dev/null; echo "st=$?"`); !strings.Contains(out, "st=1") {
		t.Errorf("kill -l INFO said %q, want a refusal", out)
	}
	// The number reaches the trap table in the shell that has no word for it.
	if out, _ := answersRun(t, `trap 'echo caught' 29; kill -29 $$; echo after`); !strings.Contains(out, "caught") {
		t.Errorf("trap on 29 said %q, want the handler to run", out)
	}
}
