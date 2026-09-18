// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runUnitStatus runs src with the register an operand-less `exit` or `return`
// reads either on or off, and with the grammar the rows need.
func runUnitStatus(t *testing.T, own Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.CurrentShellSubstitution = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.BareExitReportsTheUnitsOwnStatus = own
		sem.CurrentShellSubstitutionBoundsAnUnwind = Yes
		// Whether a `return` outside a function is refused, answered so the
		// rows below can ask about the *status* rather than about the place.
		sem.ReturnOutsideAFunctionIsRefused = No
		r.Semantics = &sem
	})
}

// An operand-less `exit` or `return` reports the last status of the unit it
// stands in, where the dialect keeps such a register, and `$?` however it got
// there where it does not. The unit is what a bare word is judged against,
// and it starts at a subshell, a command substitution, a `${ …;}` body, a
// function call, an `eval` and a sourced text — so all six read alike.
func TestABareExitReportsTheUnitsOwnStatusWhereTheDialectKeepsOne(t *testing.T) {
	for _, c := range []struct{ name, src, own, inherited string }{
		{"a command substitution", `false; a=$( exit ); echo "s=$?"`, "s=0", "s=1"},
		{"a subshell", `false; ( exit ); echo "s=$?"`, "s=0", "s=1"},
		{"a current-shell body", `false; a=${ exit; }; echo "s=$?"`, "s=0", "s=1"},
		{"a function call", `g() { return; }; false; g; echo "s=$?"`, "s=0", "s=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := runUnitStatus(t, Yes, c.src); !strings.Contains(out, c.own) {
				t.Errorf("the unit's own: got %q, want %q", out, c.own)
			}
			if out, _ := runUnitStatus(t, No, c.src); !strings.Contains(out, c.inherited) {
				t.Errorf("the inherited value: got %q, want %q", out, c.inherited)
			}
		})
	}
}

// `eval` starts a unit too, read off the status the script ends at: a bare
// `exit` in the text leaves nothing behind to print.
func TestAnEvalStartsAUnitOfItsOwn(t *testing.T) {
	if _, st := runUnitStatus(t, Yes, `false; eval 'exit'`); st != 0 {
		t.Errorf("the unit's own: status %d, want 0", st)
	}
	if _, st := runUnitStatus(t, No, `false; eval 'exit'`); st != 1 {
		t.Errorf("the inherited value: status %d, want 1", st)
	}
	// And once the text has run a command, that command's status.
	if _, st := runUnitStatus(t, Yes, `true; eval 'false; exit'`); st != 1 {
		t.Errorf("after a command: status %d, want 1", st)
	}
}

// Once the unit has run a command of its own, that command's status is the
// one reported — which is what rules out "a bare word in a unit is always 0".
func TestAUnitThatHasRunACommandReportsThatCommandsStatus(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`false; a=$( true; exit ); echo "s=$?"`, "s=0"},
		{`false; a=$( false; exit ); echo "s=$?"`, "s=1"},
		{`true; a=${ false; exit; }; echo "s=$?"`, "s=1"},
		{`true; g() { false; return; }; g; echo "s=$?"`, "s=1"},
	} {
		if out, _ := runUnitStatus(t, Yes, c.src); !strings.Contains(out, c.want) {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}

// The controls that make it a *unit* rather than a nesting depth: a brace
// group starts none, and neither does the top of the script. Both leave the
// value the shell was already holding under either answer — read off the
// status the script ends at, since `exit` is what these rows are about.
func TestABraceGroupAndTheTopOfTheScriptStartNoUnit(t *testing.T) {
	for _, own := range []Answer{Yes, No} {
		if _, st := runUnitStatus(t, own, `false; { exit; }`); st != 1 {
			t.Errorf("brace group, own=%v: status %d, want 1", own, st)
		}
		if _, st := runUnitStatus(t, own, `echo a; false; exit`); st != 1 {
			t.Errorf("top level, own=%v: status %d, want 1", own, st)
		}
	}
}

// The second register is a second register: `$?` inside the body still reads
// the value the shell came in with, which is what says the two are not one
// field read twice.
func TestTheUnitsRegisterIsNotTheOneQuestionMarkReads(t *testing.T) {
	out, _ := runUnitStatus(t, Yes, `(exit 3); j=${ echo "saw=$?"; }; echo "j=[$j]"`)
	if !strings.Contains(out, "j=[saw=3]") {
		t.Errorf("got %q, want the body to read the inherited 3 out of $?", out)
	}
}

// An operand wins over the register, which is the only spelling both readings
// agree on and the reason the axis is asked where no operand was written.
func TestAWrittenOperandIsUnaffectedByTheUnitsRegister(t *testing.T) {
	for _, own := range []Answer{Yes, No} {
		if out, _ := runUnitStatus(t, own, `false; a=${ exit 7; }; echo "s=$?"`); !strings.Contains(out, "s=7") {
			t.Errorf("own=%v: got %q, want s=7", own, out)
		}
	}
}
