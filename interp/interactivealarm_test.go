// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// signalRun runs src in a shell with the given vector and reports what it
// wrote to each stream and its status.
func signalRun(t *testing.T, sem *Semantics, interactive bool, src string) (out, errs string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var o, e bytes.Buffer
	// Located as the dialect that times out locates, with a speaking
	// builtin's name beside the line, so that a `timeout` written as the
	// `kill` that sent the signal is visible.
	dg := Diagnostics{Location: LocationTightLine, NamesBuiltinInLocation: true}
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: sem, Diagnostics: &dg,
		Name: "testsh", Interactive: interactive,
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run: %v", rerr)
	}
	return o.String(), e.String(), st
}

// An interactive shell answers an untrapped ALRM it sends itself in one of
// three ways, and every shell dies of it without `-i`.
//
// Measured 2026-10-30, `kill -ALRM $$; echo after $?` under `-i -c` and at a
// pseudo-terminal: bash 5.3.20, bash 3.2.57, dash and BusyBox ash die of it;
// ksh93u+ writes `after 0`, inside `( … )` too, and runs no trap set on it;
// zsh 5.9.2 writes `zsh:1: timeout` and exits 14, running its EXIT trap,
// until `trap - ALRM` gives the signal back its default action (#7058).
func TestAnInteractiveShellAnswersAnAlarmItsOwnWay(t *testing.T) {
	const alrmKilled = 128 + int(syscall.SIGALRM)
	for _, c := range []struct {
		name        string
		answer      InteractiveAlarmAnswer
		interactive bool
		src         string
		wantOut     string
		timesOut    bool
		wantStatus  int
		timeoutLine int
	}{
		{
			"the default action", InteractiveAlarmKills, true,
			"kill -ALRM $$\necho after\n", "", false, alrmKilled, 0,
		},
		{
			"unanswered, the default action", InteractiveAlarmUnspecified, true,
			"kill -ALRM $$\necho after\n", "", false, alrmKilled, 0,
		},
		{
			"ignored", InteractiveAlarmIsIgnored, true,
			"kill -ALRM $$\necho after $?\n", "after 0\n", false, 0, 0,
		},
		{
			"ignored through a reset", InteractiveAlarmIsIgnored, true,
			"trap - ALRM\nkill -ALRM $$\necho after\n", "after\n", false, 0, 0,
		},
		{
			"ignored inside parentheses as well", InteractiveAlarmIsIgnored, true,
			"(kill -ALRM $$; echo inner); echo st=$?\n", "inner\nst=0\n", false, 0, 0,
		},
		{
			"ignored, and a trap on it does not run", InteractiveAlarmIsIgnored, true,
			"trap 'echo T' ALRM\nkill -ALRM $$\necho after\n", "after\n", false, 0, 0,
		},
		{
			"ignored only when interactive", InteractiveAlarmIsIgnored, false,
			"kill -ALRM $$\necho after\n", "", false, alrmKilled, 0,
		},
		{
			"a trap runs when not interactive", InteractiveAlarmIsIgnored, false,
			"trap 'echo T' ALRM\nkill -ALRM $$\necho after\n", "T\nafter\n", false, 0, 0,
		},
		{
			"a timeout", InteractiveAlarmTimesOut, true,
			"kill -ALRM $$\necho after\n", "", true, int(syscall.SIGALRM), 1,
		},
		{
			"a timeout runs the EXIT trap", InteractiveAlarmTimesOut, true,
			"trap 'echo \"bye $?\"' EXIT\nkill -ALRM $$\necho after\n", "bye 14\n", true, int(syscall.SIGALRM), 2,
		},
		{
			"a timeout's EXIT trap names the status", InteractiveAlarmTimesOut, true,
			"trap 'exit 3' EXIT\nkill -ALRM $$\necho after\n", "", true, 3, 2,
		},
		{
			"a timeout after parentheses run on", InteractiveAlarmTimesOut, true,
			"(kill -ALRM $$; echo in); echo after\n", "in\n", true, int(syscall.SIGALRM), 1,
		},
		{
			"a timeout given back to the default", InteractiveAlarmTimesOut, true,
			"trap - ALRM\nkill -ALRM $$\necho after\n", "", false, alrmKilled, 0,
		},
		{
			"a timeout only when interactive", InteractiveAlarmTimesOut, false,
			"kill -ALRM $$\necho after\n", "", false, alrmKilled, 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.QuitResetRestoresTheDefault = No
			sem.SignalHandlerSeesEarlierStatus = Yes
			// The two answers of the two shells that keep a handler: ksh93
			// runs `( … )` in its own process and zsh does not, which is what
			// the parentheses rows are about.
			sem.SubshellRunsOnAfterSignalingTheShell = No
			if c.answer == InteractiveAlarmTimesOut {
				sem.SubshellRunsOnAfterSignalingTheShell = Yes
			}
			sem.InteractiveAlarm = c.answer
			out, errs, st := signalRun(t, &sem, c.interactive, c.src)
			if out != c.wantOut {
				t.Errorf("output %q, want %q", out, c.wantOut)
			}
			if st != c.wantStatus {
				t.Errorf("status %d, want %d", st, c.wantStatus)
			}
			switch {
			case c.timesOut && errs != "testsh:"+itoa(c.timeoutLine)+": timeout\n":
				t.Errorf("diagnostic %q, want the shell's own `timeout` on line %d alone", errs, c.timeoutLine)
			case !c.timesOut && errs != "":
				t.Errorf("diagnostic %q, want none", errs)
			}
		})
	}
}

// An interactive shell ignores an untrapped XCPU or XFSZ it sends itself in one
// dialect, as it ignores TERM there.
//
// Measured 2026-10-30, `kill -XCPU $$; echo after $?` under `-i -c` and at a
// pseudo-terminal: ksh93u+ writes `after 0`, and bash 5.3.20, bash 3.2.57, zsh
// 5.9.2, dash and BusyBox ash die of it; without `-i` ksh93 dies too. The
// ignore survives `trap -` there, and `( … )` ends on the signal as it does on
// TERM: `(kill -XCPU $$; echo inner); echo st=$?` writes `st=280` (#7058).
func TestAnInteractiveShellIgnoresAResourceLimitSignal(t *testing.T) {
	for _, c := range []struct {
		name        string
		ignores     Answer
		interactive bool
		src         string
		wantOut     string
		wantStatus  int
	}{
		{"XCPU ignored", Yes, true, "kill -XCPU $$\necho after $?\n", "after 0\n", 0},
		{"XFSZ ignored", Yes, true, "kill -XFSZ $$\necho after $?\n", "after 0\n", 0},
		{"through a reset", Yes, true, "trap - XCPU\nkill -XCPU $$\necho after\n", "after\n", 0},
		{
			"ending parentheses", Yes, true, "(kill -XCPU $$; echo inner); echo st=$?\n",
			"st=" + itoa(128+int(syscall.SIGXCPU)) + "\n", 0,
		},
		{"not interactive", Yes, false, "kill -XCPU $$\necho after\n", "", 128 + int(syscall.SIGXCPU)},
		{"not in the dialect", No, true, "kill -XFSZ $$\necho after\n", "", 128 + int(syscall.SIGXFSZ)},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := killSem()
			sem.QuitResetRestoresTheDefault = No
			sem.SubshellRunsOnAfterSignalingTheShell = No
			sem.InteractiveIgnoresResourceLimitSignals = c.ignores
			out, errs, st := signalRun(t, &sem, c.interactive, c.src)
			if out != c.wantOut || st != c.wantStatus || errs != "" {
				t.Errorf("wrote %q and %q, status %d; want %q, nothing, %d",
					out, errs, st, c.wantOut, c.wantStatus)
			}
		})
	}
}
