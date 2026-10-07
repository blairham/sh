// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// TestAnEditNeverReachesAVectorSomebodyHolds is what keeps writing the vector
// in place from being a leak: every way another holder comes to point at the
// runner's vector — a kept snapshot, a subshell, the parent of a subshell, a
// runner built from this one — must leave that holder's vector as it was when
// the runner next changes an option.
//
// Each row edits once before the take, so the vector taken is one the runner
// was writing in place; a row that took a vector the runner never owned would
// pass without the take doing anything.
func TestAnEditNeverReachesAVectorSomebodyHolds(t *testing.T) {
	rows := []struct {
		name string
		// take hands the runner's vector to somebody and returns what they
		// hold, and the runner that goes on editing.
		take func(r *Runner) (held *Semantics, editor *Runner)
	}{
		{"a kept snapshot", func(r *Runner) (*Semantics, *Runner) {
			return r.KeepSemantics(), r
		}},
		{"a subshell, the parent editing", func(r *Runner) (*Semantics, *Runner) {
			c := r.clone()
			return c.Semantics, r
		}},
		{"a subshell, the subshell editing", func(r *Runner) (*Semantics, *Runner) {
			c := r.clone()
			return r.Semantics, c
		}},
		{"a kept snapshot put back and kept again", func(r *Runner) (*Semantics, *Runner) {
			kept := r.KeepSemantics()
			r.EditSemantics().ArithLeadingZeroIsOctal = Yes
			r.RestoreSemantics(kept)
			return r.KeepSemantics(), r
		}},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			r := newTestRunner(t, &Runner{Semantics: &Semantics{}})
			r.EditSemantics().SplitParamExpansion = Yes
			if r.semOwned != r.Semantics {
				t.Fatal("the first edit did not leave the runner a vector of its own")
			}
			held, editor := row.take(r)
			was := held.SplitParamExpansion
			editor.EditSemantics().SplitParamExpansion = No
			if editor.Semantics.SplitParamExpansion != No {
				t.Fatal("the edit did not take")
			}
			if held.SplitParamExpansion != was {
				t.Error("an edit after the vector was handed on changed the holder's copy")
			}
		})
	}
}

// TestAFunctionsVectorIsReusedAtItsNextCall is the half that makes this worth
// having: a save, an edit and a restore — a function call that moves an
// option — allocates nothing once the runner has a spare.
func TestAFunctionsVectorIsReusedAtItsNextCall(t *testing.T) {
	r := newTestRunner(t, &Runner{Semantics: &Semantics{}})
	call := func() {
		saved := r.KeepSemantics()
		r.EditSemantics().FunctionLocalTraps = TrapsGoBackAtTheReturn
		r.EditSemantics().SplitParamExpansion = Yes
		r.RestoreSemantics(saved)
	}
	call()
	if allocs := testing.AllocsPerRun(100, call); allocs != 0 {
		t.Errorf("a call that moves two options allocated %v times, want 0", allocs)
	}
	if r.Semantics.FunctionLocalTraps == TrapsGoBackAtTheReturn || r.Semantics.SplitParamExpansion == Yes {
		t.Error("the restore did not put the caller's vector back")
	}
}

// TestAVectorASubshellTookIsNotTheNextSpare is the restore's half of the
// same guarantee: a function call that started a subshell hands the subshell
// the vector the call was writing, so that vector must not become the spare
// the next call fills, however the call ends.
func TestAVectorASubshellTookIsNotTheNextSpare(t *testing.T) {
	r := newTestRunner(t, &Runner{Semantics: &Semantics{}})
	saved := r.KeepSemantics()
	r.EditSemantics().SplitParamExpansion = Yes
	child := r.clone()
	r.RestoreSemantics(saved)

	again := r.KeepSemantics()
	r.EditSemantics().SplitParamExpansion = No
	r.RestoreSemantics(again)

	if child.Semantics.SplitParamExpansion != Yes {
		t.Error("a later call wrote the vector a subshell started inside an earlier one was holding")
	}
}
