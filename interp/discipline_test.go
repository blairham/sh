// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The hooks are off unless a dialect asks for them, and "off" has to mean the
// dotted name behaves the way three of the four columns behave: an ordinary
// function nobody fires for a variable.
//
// The control is the same source under a dialect that says yes, because a
// probe that only shows the quiet answer cannot tell "the hooks are off" from
// "the hooks do not work".
func TestDisciplineHooksAreOffUntilADialectAsksForThem(t *testing.T) {
	off := func(r *Runner) {
		sem := CoreSemantics()
		sem.PunctuatedFunctionNameIsRefused = No
		sem.DisciplineFunctionIsAVariableHook = No
		r.Semantics = &sem
	}
	src := `g=raw; g.get(){ echo fired; }; echo "[$g]"; g.get`
	out, st := run(t, src, off)
	if st != 0 || out != "[raw]\nfired\n" {
		t.Errorf("out=%q st=%d, want the plain read and an ordinary call", out, st)
	}

	on := func(r *Runner) {
		sem := CoreSemantics()
		sem.PunctuatedFunctionNameIsRefused = Yes
		sem.DisciplineFunctionIsAVariableHook = Yes
		r.Semantics = &sem
	}
	out, st = run(t, src, on)
	if st != 0 || out != "fired\n[raw]\nfired\n" {
		t.Errorf("out=%q st=%d, want the read to fire the hook as well", out, st)
	}
}

// And with the hooks on, the *refusal* is still the answer for a dotted name
// whose suffix is not one of the four events — which is the row that already
// agreed with ksh93 before any of this, and the one a wider acceptance would
// have broken.
func TestOnlyTheFourEventSuffixesEscapeTheRefusal(t *testing.T) {
	on := func(r *Runner) {
		sem := CoreSemantics()
		sem.PunctuatedFunctionNameIsRefused = Yes
		sem.DisciplineFunctionIsAVariableHook = Yes
		sem.FatalErrorStatusIsOne = Yes
		r.Semantics = &sem
		dg := Diagnostics{FunctionNameDiscipline: "%[1]s: invalid discipline function"}
		r.Diagnostics = &dg
	}
	for _, name := range []string{"g.get", "g.set", "g.append", "g.unset"} {
		out, st := run(t, "function "+name+" { :; }; echo after", on)
		if st != 0 || !strings.Contains(out, "after") {
			t.Errorf("%s: out=%q st=%d, want it defined", name, out, st)
		}
	}
	for _, name := range []string{"ns.thing", "g.getx", "g.", "a.b.get"} {
		out, st := run(t, "function "+name+" { :; }; echo after", on)
		if st != 1 || strings.Contains(out, "after") ||
			!strings.Contains(out, "invalid discipline function") {
			t.Errorf("%s: out=%q st=%d, want the fatal refusal", name, out, st)
		}
	}
}
