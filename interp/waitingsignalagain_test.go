// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.WaitingSignalArrivingAgainRunsOnce: a signal that arrives again
// while it is already waiting, before its handler had a turn (#7166).
func againSem(a Answer) Semantics {
	sem := waitingSem(WaitingHandlersRunUpTheSignalNumbers)
	sem.WaitingSignalArrivingAgainRunsOnce = a
	return sem
}

func TestASignalArrivingAgainWhileItWaitsRunsAsTheDialectSays(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		once, every string
	}{
		{
			// The issue's row: one `kill` that names the shell twice.
			"one kill, twice",
			"trap 'echo T' USR1\nkill -USR1 $$ $$\necho after\n",
			"T\nafter\n", "T\nT\nafter\n",
		},
		{
			// The same two arrivals from two commands of a subshell, which
			// the shell runs once the subshell is done: what decides is that
			// both were waiting, not that one builtin sent them.
			"two kills, both waiting",
			"trap 'echo T' USR1\n(kill -USR1 $$; kill -USR1 $$)\necho after\n",
			"T\nafter\n", "T\nT\nafter\n",
		},
		{
			// A turn between them makes them two, whatever the answer.
			"a turn between",
			"trap 'echo T' USR1\nkill -USR1 $$\nkill -USR1 $$\necho after\n",
			"T\nT\nafter\n", "T\nT\nafter\n",
		},
		{
			// Two different signals waiting both run; only the repeat folds.
			"another signal between",
			"for s in HUP USR1; do trap \"echo $s\" $s; done\n" +
				"(kill -USR1 $$; kill -HUP $$; kill -USR1 $$)\necho after\n",
			"HUP\nUSR1\nafter\n", "HUP\nUSR1\nUSR1\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.once}, {No, tc.every}} {
				out, errs, st := killRun(t, tc.src, againSem(a.answer), Diagnostics{})
				if out != a.want || errs != "" || st != 0 {
					t.Errorf("%v: got %q, %q, status %d; want %q, no diagnostic, 0",
						a.answer, out, errs, st, a.want)
				}
			}
		})
	}
}

// Unanswered, the axis is refused by name where a condition with a handler is
// waiting twice, nothing of that batch runs, and it is asked nowhere else.
func TestAnUnansweredArrivingAgainIsAskedOnlyWhereAHandledSignalWaitsTwice(t *testing.T) {
	out, errs, st := killRun(t, "trap 'echo T' USR1\nkill -USR1 $$ $$\necho after\n",
		againSem(Unspecified), Diagnostics{})
	if out != "after\n" || !strings.Contains(errs, "arrived again while it was waiting") || st != 0 {
		t.Errorf("got %q, %q, status %d; want after alone and the axis refused by name", out, errs, st)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"once", "trap 'echo T' USR1\nkill -USR1 $$\necho after\n", "T\nafter\n"},
		{"ignored, twice", "trap '' USR1\nkill -USR1 $$ $$\necho after\n", "after\n"},
		{
			"two different, once each",
			"for s in HUP USR1; do trap \"echo $s\" $s; done\n(kill -USR1 $$; kill -HUP $$)\necho after\n",
			"HUP\nUSR1\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := killRun(t, tc.src, againSem(Unspecified), Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("got %q, %q, status %d; want %q, no diagnostic, 0", out, errs, st, tc.want)
			}
		})
	}
}
