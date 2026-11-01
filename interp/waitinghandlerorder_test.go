// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.WaitingHandlersRun: the order the handlers of several signals run
// in when all of them arrived while one command ran (#7039).
//
// A subshell's `kill` aimed at `$$` is the parent's to handle once the
// subshell is done, so three of them in one subshell are three arrivals
// waiting together when it ends. They are sent in an order that is neither
// the numbers' nor the traps': USR2, then HUP, then USR1. HUP < USR1 < USR2
// holds on every host this builds for, whatever the numbers themselves are.
const waitingThree = "for s in HUP USR1 USR2; do trap \"echo $s\" $s; done\n" +
	"(kill -USR2 $$; kill -HUP $$; kill -USR1 $$)\n" +
	"echo after\n"

// waitingSem is killSem with the axes a handler's run asks on the way to it
// answered, and the one under test set to order.
func waitingSem(order WaitingHandlerOrder) Semantics {
	sem := killSem()
	sem.SignalHandlerSeesEarlierStatus = No
	sem.WaitingHandlersRun = order
	// Every arrival runs, so a case about the order sees each of them. How
	// many run has cases of its own in waitingsignalagain_test.go.
	sem.WaitingSignalArrivingAgainRunsOnce = No
	return sem
}

func TestSeveralWaitingHandlersRunInTheOrderTheDialectNames(t *testing.T) {
	for _, tc := range []struct {
		order WaitingHandlerOrder
		want  string
	}{
		{WaitingHandlersRunAsTheyArrived, "USR2\nHUP\nUSR1\nafter\n"},
		{WaitingHandlersRunUpTheSignalNumbers, "HUP\nUSR1\nUSR2\nafter\n"},
		{WaitingHandlersRunDownTheSignalNumbers, "USR2\nUSR1\nHUP\nafter\n"},
	} {
		t.Run(tc.order.String(), func(t *testing.T) {
			out, errs, st := killRun(t, waitingThree, waitingSem(tc.order), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, tc.want)
			}
		})
	}
}

// A condition that arrived twice keeps both arrivals, side by side: the sort
// is stable and says nothing about how many times one condition runs, which is
// Semantics.WaitingSignalArrivingAgainRunsOnce — answered No here, so both
// arrivals are seen.
func TestASortedBatchKeepsARepeatedConditionsArrivalsTogether(t *testing.T) {
	src := "for s in HUP USR1; do trap \"echo $s\" $s; done\n" +
		"(kill -USR1 $$; kill -HUP $$; kill -USR1 $$)\n" +
		"echo after\n"
	out, errs, st := killRun(t, src, waitingSem(WaitingHandlersRunUpTheSignalNumbers), Diagnostics{})
	if want := "HUP\nUSR1\nUSR1\nafter\n"; out != want || errs != "" || st != 0 {
		t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, want)
	}
}

// A handler that replaces the trap of one sorted after it hands that one the
// new action, read at its turn: the sort moves the arrivals and not the table.
// Measured 2026-10-31, bash 5.3.20, bash 3.2.57 and dash run `HUP NEW` for
// this with the two signals sent from a pipeline element.
func TestAWaitingHandlerCanReplaceOneThatSortsAfterIt(t *testing.T) {
	src := "trap 'echo HUP; trap \"echo NEW\" USR2' HUP\n" +
		"trap 'echo USR2' USR2\n" +
		"(kill -USR2 $$; kill -HUP $$)\n" +
		"echo after\n"
	out, errs, st := killRun(t, src, waitingSem(WaitingHandlersRunUpTheSignalNumbers), Diagnostics{})
	if want := "HUP\nNEW\nafter\n"; out != want || errs != "" || st != 0 {
		t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, want)
	}
}

// Unanswered, the axis is refused by name — none of the batch runs, rather
// than one shell's order standing in for an answer — and only where two
// different conditions with a handler are waiting together.
func TestAnUnansweredHandlerOrderIsRefusedOnlyWhereItDecides(t *testing.T) {
	out, errs, _ := killRun(t, waitingThree, waitingSem(WaitingHandlerOrderUnspecified), Diagnostics{})
	if out != "after\n" || !strings.Contains(errs, "the order several waiting signal handlers run in") {
		t.Errorf("got %q, %q; want no handler run and the axis refused by name", out, errs)
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"one handler",
			"trap 'echo U' USR1\n(kill -USR1 $$)\necho after\n",
			"U\nafter\n",
		},
		{
			"one condition twice",
			"trap 'echo U' USR1\n(kill -USR1 $$; kill -USR1 $$)\necho after\n",
			"U\nU\nafter\n",
		},
		{
			"a second condition with no handler to run",
			"trap 'echo U' USR1\ntrap '' HUP\n(kill -HUP $$; kill -USR1 $$)\necho after\n",
			"U\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := killRun(t, tc.src, waitingSem(WaitingHandlerOrderUnspecified), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, tc.want)
			}
		})
	}
}

// Two signals that land in a `read` at different moments run in the order
// they came, whatever the axis says, where the read stops to run each handler
// as its signal lands (Semantics.ReadHandlersRun): the two never wait
// together there. Measured 2026-10-31 with USR1 and HUP 0.2 s apart into a
// `read` that goes on: bash 5.3.20, bash 3.2.57 and zsh 5.9.2 run USR1 HUP,
// and USR1 > HUP by number.
func TestSignalsLandingInAReadRunInTheOrderTheyCame(t *testing.T) {
	src := "/usr/bin/mkfifo p; exec 4<>p 3<p\n" +
		"for s in HUP USR1; do trap \"echo $s\" $s; done\n" +
		"(/bin/sleep 0.2; kill -USR1 $$; /bin/sleep 0.2; kill -HUP $$; /bin/sleep 0.2; echo cd >p) &\n" +
		"read -r l <&3\n" +
		"echo \"l=$l\"\n" +
		"wait\n"
	// Whether the descriptors `exec` parks reach the commands is POSIX's
	// here, which is not the question.
	sem := waitingSem(WaitingHandlersRunUpTheSignalNumbers)
	sem.ExecOpenedFdReachesACommand = PosixSemantics().ExecOpenedFdReachesACommand
	sem.ReadHandlersRun = ReadHandlersRunWhenAWaitIsInterrupted
	out, errs, st := killRun(t, src, sem, Diagnostics{})
	if want := "USR1\nHUP\nl=cd\n"; out != want || errs != "" || st != 0 {
		t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, want)
	}
}
