// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.WaitingSignalWithoutItsTrapIsResent: a signal waiting with a
// handler, which an earlier handler of the same batch takes away before its
// turn (#7170). HUP sorts before USR2 on every host this builds for, so HUP's
// handler runs first and changes USR2's trap.
func waitingTrapGone(change string) string {
	return "trap 'echo HUP; " + change + "' HUP\n" +
		"trap 'echo USR2' USR2\n" +
		"(kill -USR2 $$; kill -HUP $$)\n" +
		"echo after\n"
}

// waitingTrapGoneDiag says what the shell says, so a test can see it said it.
var waitingTrapGoneDiag = Diagnostics{
	WaitingSignalLostItsTrap: "lost %[1]d %[2]d",
	WaitingSignalResent:      "resent %[1]d %[2]s",
}

func waitingTrapGoneSem(a Answer) Semantics {
	sem := waitingSem(WaitingHandlersRunUpTheSignalNumbers)
	sem.WaitingSignalWithoutItsTrapIsResent = a
	return sem
}

func TestAWaitingSignalWhoseTrapWasResetIsResentWhereTheDialectSaysSo(t *testing.T) {
	usr2 := int(syscall.SIGUSR2)
	lost := fmt.Sprintf("lost %d 0", usr2)
	resent := fmt.Sprintf("resent %d USR2", usr2)

	t.Run("yes", func(t *testing.T) {
		out, errs, st := killRun(t, waitingTrapGone("trap - USR2"), waitingTrapGoneSem(Yes), waitingTrapGoneDiag)
		if out != "HUP\n" || st != 128+usr2 ||
			!strings.Contains(errs, lost) || !strings.Contains(errs, resent) ||
			strings.Index(errs, lost) > strings.Index(errs, resent) {
			t.Errorf("got %q, %q, status %d; want HUP alone, %q then %q, and death by USR2", out, errs, st, lost, resent)
		}
	})
	t.Run("no", func(t *testing.T) {
		out, errs, st := killRun(t, waitingTrapGone("trap - USR2"), waitingTrapGoneSem(No), waitingTrapGoneDiag)
		if out != "HUP\nafter\n" || errs != "" || st != 0 {
			t.Errorf("got %q, %q, status %d; want HUP after, no diagnostic, 0", out, errs, st)
		}
	})
	// An ignore is said and not resent: there is no default action to have.
	t.Run("yes, ignored", func(t *testing.T) {
		out, errs, st := killRun(t, waitingTrapGone("trap \"\" USR2"), waitingTrapGoneSem(Yes), waitingTrapGoneDiag)
		want := fmt.Sprintf("lost %d 1", usr2)
		if out != "HUP\nafter\n" || st != 0 || !strings.Contains(errs, want) || strings.Contains(errs, "resent") {
			t.Errorf("got %q, %q, status %d; want HUP after, %q alone, 0", out, errs, st, want)
		}
	})
	// A signal whose default does nothing is resent and does nothing.
	t.Run("yes, a signal whose default is to carry on", func(t *testing.T) {
		src := "trap 'echo HUP; trap - WINCH' HUP\ntrap 'echo W' WINCH\n(kill -WINCH $$; kill -HUP $$)\necho after\n"
		sem := waitingTrapGoneSem(Yes)
		sem.SelfAimedWindowChangeWaitsForInputOrAChild = No
		out, errs, st := killRun(t, src, sem, waitingTrapGoneDiag)
		if out != "HUP\nafter\n" || st != 0 || !strings.Contains(errs, "resent") {
			t.Errorf("got %q, %q, status %d; want HUP after, both said, 0", out, errs, st)
		}
	})
}

// Unanswered, the axis is refused by name where a waiting signal has lost its
// trap, and asked nowhere else: not where a handler replaced it, and not for
// the second arrival of a condition whose own handler reset its trap.
func TestAnUnansweredResentSignalIsAskedOnlyWhereATrapWentAway(t *testing.T) {
	out, errs, _ := killRun(t, waitingTrapGone("trap - USR2"), waitingTrapGoneSem(Unspecified), waitingTrapGoneDiag)
	const axis = "what a waiting signal does once an earlier handler took its trap away"
	if out != "HUP\nafter\n" || !strings.Contains(errs, axis) {
		t.Errorf("got %q, %q; want HUP after and the axis refused by name", out, errs)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"replaced", waitingTrapGone("trap \"echo NEW\" USR2"), "HUP\nNEW\nafter\n"},
		{
			"its own handler, twice",
			"trap 'echo U; trap - USR1' USR1\n(kill -USR1 $$; kill -USR1 $$)\necho after\n",
			"U\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := killRun(t, tc.src, waitingTrapGoneSem(Unspecified), waitingTrapGoneDiag)
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, tc.want)
			}
		})
	}
}
