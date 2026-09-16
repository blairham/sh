// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A compound whose last command was exempt from `set -e` is exempt too (#3344).
//
// A group, a loop, an `if` and a `case` answer with the status their body's
// last statement left. Where that statement was one `set -e` and the ERR trap
// do not judge — the left operand of an `&&` that short-circuited, a `!` — the
// compound was judged in its place, and the script ended over a status the
// shell had been told not to act on. Every row below writes `survived` in bash
// 5.3.20, bash 3.2, zsh 5.9.2, ksh93u+ 2012-08-01 and dash; every one ended at
// 1 in all four of our columns.

var exemptLastCommand = []struct{ name, body string }{
	{"a loop ending in a short-circuited test", `for i in 1 2; do [ "$i" = 3 ] && echo three; done`},
	{"a group ending in a negation", `{ ! true; }`},
	{"an if body ending in a negation", `if true; then ! true; fi`},
	{"a loop ending in a negation", `for i in 1; do ! true; done`},
	{"a group ending in a short-circuited chain", `{ false && true; }`},
	{"a case arm ending in a negation", `case x in x) ! true;; esac`},
	{"a while loop whose condition ran last", `i=0; while [ "$i" = 0 ]; do i=1; false && true; done`},
	{"a group in a group", `{ { ! true; }; }`},
}

func TestSetEDoesNotJudgeACompoundWhoseLastCommandWasExempt(t *testing.T) {
	for _, tc := range exemptLastCommand {
		out, status := run(t, "set -e; "+tc.body+"; echo survived", withSem(errSem()))
		if out != "survived\n" || status != 0 {
			t.Errorf("%s: got %q at %d, want survived at 0", tc.name, out, status)
		}
	}
}

func TestTheErrTrapDoesNotFireForACompoundWhoseLastCommandWasExempt(t *testing.T) {
	for _, refiring := range everyRefiring {
		s := errSem()
		s.ErrTrapRefiresForTheCommandItFiredInside = refiring
		for _, tc := range exemptLastCommand {
			if out, _ := run(t, "trap 'echo E' ERR; "+tc.body+"; echo done", withSem(s)); out != "done\n" {
				t.Errorf("%s under %s: got %q, want no E", tc.name, refiring, out)
			}
		}
	}
}

func TestACompoundThatFailedBeforeItsBodyIsStillJudged(t *testing.T) {
	// The status is the compound's own when its body never ran, and nothing
	// inside judged anything: a redirection on it that cannot be opened.
	// bash 5.3.20, zsh 5.9.2 and dash stop there and bash and zsh fire ERR.
	// ksh93 carries on without firing, which is a question about that
	// redirection's failure rather than about the compound, and not this
	// test's.
	out, status := run(t, "exec 2>/dev/null; set -e; { echo body; } >/nonexistent/dir/f; echo survived", withSem(errSem()))
	if out != "" || status == 0 {
		t.Errorf("got %q at %d, want the script stopped", out, status)
	}
	if out, _ := run(t, "exec 2>/dev/null; trap 'echo E' ERR; { echo body; } >/nonexistent/dir/f; echo done", withSem(errSem())); out != "E\ndone\n" {
		t.Errorf("ERR: got %q, want one E", out)
	}
}

func TestAFailureInsideACompoundStillStopsTheScript(t *testing.T) {
	// The judged half, so that a fix that stopped judging compounds by
	// stopping the body from being judged would be seen.
	for _, body := range []string{
		`{ false; }`,
		`for i in 1; do false; done`,
		`if true; then false; fi`,
		`{ true && false; }`,
	} {
		out, status := run(t, "set -e; "+body+"; echo survived", withSem(errSem()))
		if out != "" || status != 1 {
			t.Errorf("%s: got %q at %d, want stopped at 1", body, out, status)
		}
	}
}
