// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Three answers on one column's fatal path: what a refused `set` option leaves
// applied, what an error the shell reported does to the EXIT trap under
// `set -e`, and what a `return` written inside a function-local EXIT trap
// returns from.

// fatalRun answers the axes the cases here need and leaves the rest alone.
func fatalRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := permissive()
		set(&sem)
		r.Semantics = &sem
		r.Diagnostics = &Diagnostics{}
	})
}

// A shell ending over an error it reported, with `set -e` on, where one column
// runs no EXIT trap at all.
//
// The discriminator is the pair and not either half: the same errors keep the
// handler with the option off, and a stop the script *asked* for keeps it with
// the option on. Both controls are rows here, because an axis proved by only
// the case it moves cannot say which of the two inputs is doing the work.
func TestWhetherAFatalErrorUnderErrexitSkipsTheExitTrap(t *testing.T) {
	const handler = `trap 'printf TRAP_RAN' EXIT` + "\n"
	for _, c := range []struct {
		name  string
		src   string
		skips bool
	}{
		// Errors the shell reports and gives up over.
		{"a readonly reassignment", "readonly r=1\nreadonly r=2\nprintf after", true},
		{"an unset parameter under nounset", "set -u\nprintf '[%s]' \"$nosuch\"\nprintf after", true},
		{"a division by zero", "printf '[%s]' \"$((1/0))\"\nprintf after", true},
		// Stops the script asked for.
		{"an ordinary failure errexit stopped for", "false\nprintf after", false},
		{"an explicit exit", "exit 3\nprintf after", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, answer := range []Answer{Yes, No} {
				out, _ := fatalRun(t, handler+"set -e\n"+c.src, func(s *Semantics) {
					s.FatalErrorUnderErrexitSkipsTheExitTrap = answer
					s.ReadonlyReassignmentByDeclarationFatal = Yes
					s.ReadonlyReassignmentBySpecialBuiltinFatal = Yes
					s.ArithDivisionByZeroYieldsAValue = No
				})
				ran := strings.Contains(out, "TRAP_RAN")
				want := !c.skips || answer != Yes
				if ran != want {
					t.Errorf("answer=%v: trap ran=%v, want %v — got %q", answer, ran, want, out)
				}
				if strings.Contains(out, "after") {
					t.Errorf("answer=%v: the script should have ended, got %q", answer, out)
				}
			}
		})
	}
}

// And with the option off the handler runs whatever the axis says, which is
// the control that says the option is half the discriminator.
func TestTheExitTrapSurvivesAFatalErrorWithoutErrexit(t *testing.T) {
	out, _ := fatalRun(t, "trap 'printf TRAP_RAN' EXIT\nreadonly r=1\nreadonly r=2\nprintf after",
		func(s *Semantics) {
			s.FatalErrorUnderErrexitSkipsTheExitTrap = Yes
			s.ReadonlyReassignmentByDeclarationFatal = Yes
			s.ReadonlyReassignmentBySpecialBuiltinFatal = Yes
		})
	if !strings.Contains(out, "TRAP_RAN") {
		t.Errorf("without the option the handler still runs, got %q", out)
	}
}

// What a `return` written inside a function-local EXIT trap returns from: the
// frame the shell is returning *into*, because the frame whose trap it is has
// already been left by the time the handler runs.
//
// Core rather than an axis. The question can only be put to a shell that fires
// such a trap at all, and both answers there agree; the columns that never
// fire one cannot be asked.
func TestAReturnInAFunctionExitTrapIsTheCallersReturn(t *testing.T) {
	local := func(s *Semantics) { s.ExitTrapIsFunctionLocal = Yes }
	const p = "p() { trap 'return 9' EXIT; return 3; }\n"

	// With no caller, there is no frame to return from and the script ends.
	if out, st := fatalRun(t, p+"p\nprintf after", local); out != "" || st != 9 {
		t.Errorf("at the top level: got %q/%d, want nothing and 9", out, st)
	}
	// With one, the caller returns and its own caller carries on.
	out, _ := fatalRun(t, p+"q() { p; printf 'in q'; }\nq\nprintf 'after -> %s' \"$?\"", local)
	if out != "after -> 9" {
		t.Errorf("one frame out: got %q, want %q", out, "after -> 9")
	}
	// Exactly one frame, and not "the shell": two deep, the outer call runs on.
	out, _ = fatalRun(t, p+"q() { p; printf 'in q'; }\nr() { q; printf 'in r'; }\nr\nprintf ' after'", local)
	if out != "in r after" {
		t.Errorf("two frames out: got %q, want %q", out, "in r after")
	}
	// `exit` in the same body needs none of it and never did.
	if out, st := fatalRun(t, "p() { trap 'exit 4' EXIT; return 3; }\np\nprintf after", local); out != "" || st != 4 {
		t.Errorf("exit in the body: got %q/%d, want nothing and 4", out, st)
	}
	// And a body that runs to its end still hands back the call's status,
	// which is what says the change is about the `return` and not about the
	// handler.
	if out, _ := fatalRun(t, "g() { trap ':' EXIT; return 2; }\ng\nprintf 'g -> %s' \"$?\"", local); out != "g -> 2" {
		t.Errorf("a body that returned nothing: got %q, want %q", out, "g -> 2")
	}
}

// Whether the option loop goes on *applying* the words behind one it refused,
// which is a different question from how many of them it reports.
//
// The `-o` listing is what makes the two separable: it is written by the
// applying loop, so a shell that stops at the bad word never prints one.
func TestWhetherSetAppliesTheWordsAfterARefusedOption(t *testing.T) {
	const src = "set -Z -x -o\nprintf NOT-REACHED"
	for _, c := range []struct {
		name            string
		applies, everyB Answer
		wantApplied     bool
	}{
		{"stops at the bad word", No, No, false},
		{"carries on applying", Yes, No, true},
		{"reports every bad word and applies none", No, Yes, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := fatalRun(t, src, func(s *Semantics) {
				s.SetAppliesTheWordsAfterARefusedOption = c.applies
				s.SetReportsEveryBadOption = c.everyB
				s.BadSetOptionLetterFatal = Yes
			})
			// The option table is written by the *applying* loop, so a shell
			// that stopped at the bad word never prints one — and the `-x`
			// behind the refusal shows in it as `on`, which is the half that
			// says the words were applied and not only read.
			if applied := strings.Contains(out, "xtrace"); applied != c.wantApplied {
				t.Errorf("option table written=%v, want %v — got %q", applied, c.wantApplied, out)
			}
			if c.wantApplied && !strings.Contains(out, "xtrace") {
				t.Errorf("the letter behind the refusal was not applied, got %q", out)
			}
			if strings.Contains(out, "NOT-REACHED") {
				t.Errorf("the refusal is still fatal whatever the loop did, got %q", out)
			}
		})
	}
}

// And only the *first* bad word is reported where the loop carries on, which
// is the other column's answer and not this one's.
func TestOnlyTheFirstRefusedSetOptionIsReportedWhereTheLoopCarriesOn(t *testing.T) {
	count := func(out string) int { return strings.Count(out, "invalid option") }
	carries, _ := fatalRun(t, "set -Z -Y -x -o", func(s *Semantics) {
		s.SetAppliesTheWordsAfterARefusedOption = Yes
		s.SetReportsEveryBadOption = No
		s.BadSetOptionLetterFatal = Yes
	})
	if n := count(carries); n != 1 {
		t.Errorf("the loop that applies reports one bad word, got %d in %q", n, carries)
	}
	every, _ := fatalRun(t, "set -Z -Y -x -o", func(s *Semantics) {
		s.SetAppliesTheWordsAfterARefusedOption = No
		s.SetReportsEveryBadOption = Yes
		s.BadSetOptionLetterFatal = Yes
	})
	if n := count(every); n != 2 {
		t.Errorf("the loop that reports every one says two, got %d in %q", n, every)
	}
}
