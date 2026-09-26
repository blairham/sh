// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A signal that ended something a subshell *ran* is reported once and not
// twice.
//
// The guard on #4649's fix, from the side the fix could break. A subshell now
// reports the signal that ended the parentheses themselves, and the
// distinction that makes that safe is `diedOfItsOwnSignal` rather than
// `diedOfSig`: the status of a command killed inside the body is carried out
// through the parentheses, so a rule written on the status would write the
// sentence a second time with the subshell's own text on it.
//
// One notice, and it is the subshell's text either way — measured against
// bash 5.3.20, `( /bin/sh -c 'kill -QUIT $$' )` writes one line naming the
// parentheses — so the count is what this row is about and the text is the
// control beside it.
func TestASubshellReportsTheSignalThatEndedItOnlyOnce(t *testing.T) {
	dg := Diagnostics{KilledCommandNotice: "%5[1]d %-27[2]s%[3]s"}
	got, status := killedRun(t, "( /bin/sh -c 'kill -USR1 $$' )\n", dg, Yes)
	if n := strings.Count(got, "Boom"); n != 1 {
		t.Errorf("got %q: %d notices, want one", got, n)
	}
	if !strings.Contains(got, "( /bin/sh -c 'kill -USR1 $$' )") {
		t.Errorf("got %q, want the parentheses written back", got)
	}
	if want := 128 + int(syscall.SIGUSR1); status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}
