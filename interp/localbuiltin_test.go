// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `local` needs a function to be local to. What happens when there is none
// is three answers, and these name the axis rather than the shell.

// TestLocalAtTheTopLevelIsRefusedOrTakenAsAGlobal covers both directions,
// and the assertion on the refusing side is two-sided: not setting the
// variable is half the answer, and a refusal that set it anyway would pass a
// test that only looked at stderr.
func TestLocalAtTheTopLevelIsRefusedOrTakenAsAGlobal(t *testing.T) {
	refuse := func(s *Semantics) {
		s.LocalOutsideAFunctionIsAnError = Yes
		s.LocalOutsideAFunctionIsFatal = No
	}
	out, errs, st := trapRun(t, "local x=2\necho x=$x\necho end", refuse,
		Diagnostics{LocalOutsideAFunction: "local: not here"})
	if !strings.Contains(errs, "local: not here") {
		t.Errorf("refused: stderr = %q, want the refusal", errs)
	}
	if !strings.Contains(out, "x=\n") {
		t.Errorf("refused: stdout = %q, want the variable left unset", out)
	}
	if !strings.Contains(out, "end") {
		t.Errorf("refused: stdout = %q, want the script to carry on", out)
	}
	if st != 0 {
		t.Errorf("refused: status = %d, want the last command's", st)
	}

	accept := func(s *Semantics) { s.LocalOutsideAFunctionIsAnError = No }
	out, errs, st = trapRun(t, "local x=2\necho x=$x", accept, Diagnostics{})
	if errs != "" || st != 0 {
		t.Errorf("accepted: stderr %q status %d, want neither", errs, st)
	}
	if !strings.Contains(out, "x=2") {
		t.Errorf("accepted: stdout = %q, want a global set", out)
	}
}

// TestTheRefusalCanEndTheScript is the second axis, and it is a separate
// question from the first: both dialects that refuse say the same kind of
// thing and only one of them stops.
func TestTheRefusalCanEndTheScript(t *testing.T) {
	fatal := func(s *Semantics) {
		s.LocalOutsideAFunctionIsAnError = Yes
		s.LocalOutsideAFunctionIsFatal = Yes
	}
	out, errs, st := trapRun(t, "local x=2\necho end", fatal, Diagnostics{})
	if !strings.Contains(errs, "local") {
		t.Errorf("stderr = %q, want the refusal", errs)
	}
	if strings.Contains(out, "end") {
		t.Errorf("stdout = %q, want the script to stop", out)
	}
	if st == 0 {
		t.Error("status = 0, want the refusal to fail")
	}
}

// TestInsideAFunctionTheAxisIsNotAsked, because there is nothing to answer:
// a local with a scope to live in is the same in every dialect, including
// the one that would otherwise refuse the word outright.
func TestInsideAFunctionTheAxisIsNotAsked(t *testing.T) {
	fatal := func(s *Semantics) {
		s.LocalOutsideAFunctionIsAnError = Yes
		s.LocalOutsideAFunctionIsFatal = Yes
	}
	src := "x=outer\nf() { local x=inner; echo in=$x; }\nf\necho out=$x"
	out, errs, st := trapRun(t, src, fatal, Diagnostics{})
	if !strings.Contains(out, "in=inner") || !strings.Contains(out, "out=outer") {
		t.Errorf("stdout = %q, want the local shadowed and put back", out)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want neither", errs, st)
	}
}
