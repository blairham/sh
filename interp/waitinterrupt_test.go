// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `wait` blocked on a background job and cut short by a trapped signal.
//
// The signal is USR1 because it is the one nothing else in a shell means, and
// its number is read from the host rather than written down: it is 30 here and
// 10 on Linux, so a literal would pin the wrong platform.
//
// The handler is deliberately not empty. An empty one is `trap ” USR1`, which
// installs a process-wide ignore that outlives the runner and is inherited
// through exec — a test process is exactly the wrong place for that, so the
// ignored case is pinned in the corpus, where each snippet is its own process.
func waitTrapSem(t *testing.T) Semantics {
	t.Helper()
	s := permissive()
	s.SignalHandlerSeesEarlierStatus = No
	s.TrapBodyRunsWhatParsed = Yes
	return s
}

const waitTrapSrc = `trap 'echo T' USR1; (sleep 0.2; kill -USR1 $$) & `

func TestABareWaitReportsTheSignalThatCutItShort(t *testing.T) {
	sem := waitTrapSem(t)
	sem.SignalDeathStatusIsTwoFiftySix = No
	out, _ := run(t, waitTrapSrc+`wait; echo "st=$?"`, withSem(sem))
	want := "T\nst=" + strconv.Itoa(128+int(syscall.SIGUSR1)) + "\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestAnInterruptedWaitUsesTheSignalDeathEncoding: the same axis that decides
// what a command killed by a signal reports decides this, rather than a second
// copy of the arithmetic.
func TestAnInterruptedWaitUsesTheSignalDeathEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		two  Answer
		base int
	}{
		{"128 plus the signal", No, 128},
		{"256 plus the signal", Yes, 256},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := waitTrapSem(t)
			sem.SignalDeathStatusIsTwoFiftySix = tc.two
			out, _ := run(t, waitTrapSrc+`wait; echo "st=$?"`, withSem(sem))
			want := "st=" + strconv.Itoa(tc.base+int(syscall.SIGUSR1))
			if !strings.Contains(out, want) {
				t.Errorf("got %q, want %q in it", out, want)
			}
		})
	}
}

// TestWaitForAJobFailsWhenInterruptedIsAnAxis: with an operand, one answer
// keeps the signal's own status and the other reports a plain failure. Both
// sides are somebody's, which is what makes it an axis rather than a bug.
func TestWaitForAJobFailsWhenInterruptedIsAnAxis(t *testing.T) {
	for _, form := range []string{`wait $!`, `wait %1`} {
		t.Run(form, func(t *testing.T) {
			sem := waitTrapSem(t)
			sem.SignalDeathStatusIsTwoFiftySix = No
			sem.WaitForAJobFailsWhenInterrupted = No
			out, _ := run(t, waitTrapSrc+form+`; echo "st=$?"`, withSem(sem))
			if want := "st=" + strconv.Itoa(128+int(syscall.SIGUSR1)); !strings.Contains(out, want) {
				t.Errorf("no: got %q, want %q in it", out, want)
			}

			sem.WaitForAJobFailsWhenInterrupted = Yes
			out, _ = run(t, waitTrapSrc+form+`; echo "st=$?"`, withSem(sem))
			if !strings.Contains(out, "st=1") {
				t.Errorf("yes: got %q, want a plain failure", out)
			}
		})
	}
}

// TestTheBareFormDoesNotAskTheOperandAxis: ksh93 is the shell that needs the
// axis and it is the shell whose *bare* wait still reports the signal, so the
// two questions have to stay separate.
func TestTheBareFormDoesNotAskTheOperandAxis(t *testing.T) {
	sem := waitTrapSem(t)
	sem.SignalDeathStatusIsTwoFiftySix = Yes
	sem.WaitForAJobFailsWhenInterrupted = Yes
	out, _ := run(t, waitTrapSrc+`wait; echo "st=$?"`, withSem(sem))
	if want := "st=" + strconv.Itoa(256+int(syscall.SIGUSR1)); !strings.Contains(out, want) {
		t.Errorf("got %q, want %q in it", out, want)
	}
}

// TestAWaitNobodyInterruptedIsUntouched is the control. A trap that never
// fires must leave `wait` reporting what it always did — 0 for the bare form
// and the job's own status for the named one — or the case above would be
// evidence about having a trap rather than about a signal arriving.
func TestAWaitNobodyInterruptedIsUntouched(t *testing.T) {
	sem := waitTrapSem(t)
	sem.SignalDeathStatusIsTwoFiftySix = No
	for _, tc := range []struct{ name, src, want string }{
		{"bare", `trap 'echo T' USR1; (exit 7) & wait; echo "st=$?"`, "st=0"},
		{"named", `trap 'echo T' USR1; (exit 7) & wait $!; echo "st=$?"`, "st=7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, withSem(sem))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
