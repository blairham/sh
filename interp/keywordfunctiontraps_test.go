// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// TrapsGoBackAtTheReturnOfAKeywordFunction: the same restore the option
// answer makes, keyed on how the function was *written* rather than on an
// option (#2345).
//
// Every row runs the same body twice — once as `function g { … }` and once as
// `g() { … }` — because the pairing is the whole of the answer. A row with
// only the keyword form beside a row with only the POSIX form would pass
// under TrapsGoBackAtTheReturn too, which scopes both.
//
// Every handler prints something distinct, for the reason localtraps_test.go
// gives: an inner trap echoing the outer one's word cannot tell "put back"
// from "left alone".

func keywordTrapsSem() Semantics {
	s := permissive()
	s.FunctionLocalTraps = TrapsGoBackAtTheReturnOfAKeywordFunction
	s.TrapQuoting = ListingQuoteAlwaysEscaped
	return s
}

func keywordTrapsRun(t *testing.T, src string) (string, int) {
	t.Helper()
	s := keywordTrapsSem()
	return run(t, src, withSem(s))
}

// The pairing, on a signal.
func TestKeywordFunctionTrapsAreScopedAndPosixOnesAreNot(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { trap 'echo inner' USR1; }`, "trap -- 'echo outer' USR1\n"},
		{"the POSIX form", `g() { trap 'echo inner' USR1; }`, "trap -- 'echo inner' USR1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := keywordTrapsRun(t, `trap 'echo outer' USR1; `+tc.def+`; g; trap`)
			if out != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q", out, st, tc.want)
			}
		})
	}
}

// And the other answer leaves the two the same, which is what makes this a
// third value rather than a reading of the second.
func TestKeywordFunctionTrapsDifferFromTheOptionAnswer(t *testing.T) {
	const src = `trap 'echo outer' USR1; g() { trap 'echo inner' USR1; }; g; trap`
	s := permissive()
	s.FunctionLocalTraps = TrapsGoBackAtTheReturn
	s.TrapQuoting = ListingQuoteAlwaysEscaped
	if out, _ := run(t, src, withSem(s)); out != "trap -- 'echo outer' USR1\n" {
		t.Errorf("the option answer scopes a POSIX-form call too: got %q", out)
	}
	if out, _ := keywordTrapsRun(t, src); out != "trap -- 'echo inner' USR1\n" {
		t.Errorf("the keyword answer must leave it: got %q", out)
	}
}

// EXIT is this question here, where under the option answer it is a rule of
// its own. Both halves of the row matter: the function's own trap fires at
// the return, and the caller's is standing again afterwards.
func TestKeywordFunctionScopesTheExitTrap(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { trap 'echo inner' EXIT; }`, "inner\nafter\nouter\n"},
		{"the POSIX form", `g() { trap 'echo inner' EXIT; }`, "after\ninner\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := keywordTrapsRun(t, `trap 'echo outer' EXIT; `+tc.def+`; g; echo after`)
			if out != tc.want || st != 0 {
				t.Errorf("got %q/%d, want %q", out, st, tc.want)
			}
		})
	}
}

// A second call fires its own and not the first call's twice, which is what
// says the restore actually happened rather than the handler merely being
// run.
func TestKeywordFunctionExitTrapFiresOncePerCall(t *testing.T) {
	out, _ := keywordTrapsRun(t, `function g { trap 'echo inner' EXIT; echo body; }; g; g; echo after`)
	if want := "body\ninner\nbody\ninner\nafter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The boundary is the `function` word and not the call: a POSIX-form function
// called inside one is not itself a scope, so what it set stands until the
// enclosing `function` returns. A save taken on the innermost scope would put
// `outer` back one line early and print it twice.
func TestKeywordFunctionIsTheBoundaryAndNotEveryCall(t *testing.T) {
	out, _ := keywordTrapsRun(t,
		`trap 'echo outer' USR1; function o { p() { trap 'echo inner' USR1; }; p; kill -USR1 $$; }; o; kill -USR1 $$`)
	if want := "inner\nouter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The same from the EXIT side, which reaches it by the other mechanism: the
// trap `p` set is the one the enclosing `function` fires at its return.
func TestKeywordFunctionFiresAnExitTrapANestedCallSet(t *testing.T) {
	out, _ := keywordTrapsRun(t,
		`trap 'echo outer' EXIT; function o { p() { trap 'echo inner' EXIT; }; p; echo mid; }; o; echo after`)
	if want := "mid\ninner\nafter\nouter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// What comes back is the disposition and not the listing: with no outer trap,
// a signal sent after the return reaches a shell with nothing handling it.
func TestKeywordFunctionRestoresTheDefaultDisposition(t *testing.T) {
	out, _ := keywordTrapsRun(t, `function g { trap 'echo inner' USR1; }; g; trap`)
	if out != "" {
		t.Errorf("got %q, want no trap listed at all", out)
	}
}

// A `trap -` reset is a modification like a set, so the caller's handler
// comes back from one too.
func TestKeywordFunctionRestoresAReset(t *testing.T) {
	out, _ := keywordTrapsRun(t, `trap 'echo outer' USR1; function g { trap - USR1; }; g; trap`)
	if want := "trap -- 'echo outer' USR1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// What the body *sees* is the other half, and it is the half that says the
// table was taken rather than one condition saved at each modification: the
// caller's handler is not merely put back at the return, it is not there
// while the body runs. Measured on AT&T 93u+ — the listing inside the call is
// empty with both a signal and EXIT set outside it.
func TestKeywordFunctionBodySeesAnEmptyTrapTable(t *testing.T) {
	out, _ := keywordTrapsRun(t,
		`trap 'echo outer' USR1`+"\n"+`trap 'echo bye' EXIT`+"\n"+
			`function g { echo in:; trap; echo .; }`+"\n"+`g`+"\n"+`echo top:; trap`)
	// The listing order is the substrate's own and not what is under test.
	if want := "in:\n.\ntop:\ntrap -- 'echo bye' EXIT\ntrap -- 'echo outer' USR1\nbye\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And a POSIX-form call sees the caller's, which is the same pairing again.
func TestPosixFunctionBodySeesTheCallersTrapTable(t *testing.T) {
	out, _ := keywordTrapsRun(t, `trap 'echo outer' USR1; g() { trap; }; g`)
	if want := "trap -- 'echo outer' USR1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Nesting answers at each return, and the inner call's table is the outer
// call's — which is empty, not the top level's. A restore that put the
// *shell's* table back at every return would list the outer handler here.
func TestKeywordFunctionsNestAndEachAnswersForItself(t *testing.T) {
	out, _ := keywordTrapsRun(t,
		`trap 'echo outer' USR1`+"\n"+
			`function o { function i { trap 'echo inner' USR1; }; i; echo in-o:; trap; echo .; }`+"\n"+
			`o`+"\n"+`echo top:; trap`)
	if want := "in-o:\n.\ntop:\ntrap -- 'echo outer' USR1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A `( … )` inside a keyword body is a shell of its own and its changes are
// its own: the enclosing call must not put back something it never displaced.
func TestKeywordFunctionLeavesASubshellsTrapsAlone(t *testing.T) {
	out, _ := keywordTrapsRun(t, `trap 'echo outer' USR1; function g { ( trap 'echo sub' USR1 ); }; g; trap`)
	if want := "trap -- 'echo outer' USR1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The pseudo-conditions go with the table too, which the signal rows cannot
// show: a body under the keyword form does not see the caller's ERR trap, and
// the same body written the other way does. Measured on AT&T 93u+ — the
// POSIX-form call writes the handler's line between its own two and the
// keyword form writes neither.
func TestKeywordFunctionTakesThePseudoConditionsToo(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { echo in; false; echo still; }`, "in\nstill\nafter\n"},
		{"the POSIX form", `g() { echo in; false; echo still; }`, "in\nerr\nstill\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := keywordTrapsSem()
			sem.TrapHasErrCondition = Yes
			sem.ErrTrapRunsInsideFunctions = Yes
			sem.ErrTrapRunsInSubshells = No
			out, _ := run(t, `trap 'echo err' ERR; `+tc.def+`; g; echo after`, withSem(sem))
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
