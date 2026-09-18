// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `[[ -v a[@] ]]`, which is Semantics.ConditionWholeArraySubscript: the one
// operand shape where the set-ness operator parts company with the
// conditional expansion that otherwise decides it. Tests name the axis and
// never a shell.

// wholeSubscriptRun turns on the flag `-v` needs and the arrays it asks
// about, and sets the axis under test.
func wholeSubscriptRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParameterIsSetTest = true
	}, func(r *Runner) {
		d := syntax.Core()
		d.ParameterIsSetTest = true
		r.Dialect = &d
		if set != nil {
			set(r.Semantics)
		}
	})
}

// Under the reading where the subscript names the **parameter**, the answer
// is the parameter's own set-ness — so an array with elements is set and an
// empty one follows EmptyArrayIsSet.
func TestAWholeArraySubscriptCanNameTheParameter(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		s.ConditionWholeArraySubscript = ConditionWholeArraySubscriptNamesTheParameter
		s.EmptyArrayIsSet = No
	}
	src := `e=(); f=(x); s=plain
[[ -v e[@] ]] && echo "e yes" || echo "e no"
[[ -v f[@] ]] && echo "f yes" || echo "f no"
[[ -v f[*] ]] && echo "f* yes" || echo "f* no"
[[ -v s[@] ]] && echo "s yes" || echo "s no"`
	out, st := wholeSubscriptRun(t, src, set)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "e no\nf yes\nf* yes\ns yes\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A **table** is the exception under that reading, and it is a lookup rather
// than a carve-out: the key `@` answers when it is really there.
func TestAWholeArraySubscriptOverATableIsAKey(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		s.ConditionWholeArraySubscript = ConditionWholeArraySubscriptNamesTheParameter
		s.EmptyArrayIsSet = No
		// The `@` key has to be storable for the lookup to be testable, and
		// which of the two that assignment is is a question of its own.
		s.WholeArraySubscriptAssigningATable = WholeArraySubscriptIsAnOrdinaryKey
	}
	src := `typeset -A n; n[k]=v; typeset -A m; m[@]=x
[[ -v n[@] ]] && echo "n yes" || echo "n no"
[[ -v n[k] ]] && echo "k yes" || echo "k no"
[[ -v m[@] ]] && echo "m yes" || echo "m no"`
	out, st := wholeSubscriptRun(t, src, set)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "n no\nk yes\nm yes\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// Under the other reading the subscript names an **element**, and no array
// has one called `@` — so a name holding either kind of array is never set
// through it however many elements it has, while a scalar answers for itself.
func TestAWholeArraySubscriptCanNameAnElement(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		s.ConditionWholeArraySubscript = ConditionWholeArraySubscriptNamesAnElement
		s.EmptyArrayIsSet = Yes
	}
	src := `e=(); f=(x); s=plain; typeset -A n; n[k]=v
[[ -v e[@] ]] && echo "e yes" || echo "e no"
[[ -v f[@] ]] && echo "f yes" || echo "f no"
[[ -v n[@] ]] && echo "n yes" || echo "n no"
[[ -v n[k] ]] && echo "k yes" || echo "k no"
[[ -v s[@] ]] && echo "s yes" || echo "s no"`
	out, st := wholeSubscriptRun(t, src, set)
	if st != 0 {
		t.Fatalf("status %d: %q", st, out)
	}
	if want := "e no\nf no\nn no\nk yes\ns yes\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And a dialect that has the operator and has not chosen refuses by name,
// the way every unanswered axis does.
func TestAWholeArraySubscriptIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	out, st := wholeSubscriptRun(t, `f=(x); [[ -v f[@] ]]`, nil)
	if st == 0 {
		t.Fatalf("status 0 for an unanswered axis; want a refusal")
	}
	if !strings.Contains(out, "a whole-array subscript") {
		t.Errorf("output = %q, want the axis named", out)
	}
}
