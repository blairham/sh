// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `n` letter of `readonly`, which is Semantics.ReadonlyReferenceLetter.
// It is not `export -n`'s letter under another word: nothing is taken off,
// and the two readings the panel splits into are "the freeze this call would
// have made is suppressed" and "the letter is taken and the name frozen
// anyway". Tests name the axis and never a shell.

// withReadonlyNLetter gives the synthetic dialect the letter, which is the
// half without which the axis is unreachable.
func withReadonlyNLetter(s *Semantics) {
	s.ReadonlyOptions = "pn"
}

func TestTheReadonlyNLetterCanDeclareAnUnfrozenName(t *testing.T) {
	t.Parallel()
	out, errs, st := declRun(t, "readonly -n r=v; typeset -p r; r=5; echo [$r]",
		func(s *Semantics) {
			withReadonlyNLetter(s)
			s.ReadonlyReferenceLetter = ReadonlyReferenceLetterDeclaresAnUnfrozenName
		}, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d, stderr %q; want a silent 0", st, errs)
	}
	if want := "declare -- r=\"v\"\n[5]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The other reading, where the letter buys a script the status and nothing
// else: the name is frozen and the assignment after it is refused.
func TestTheReadonlyNLetterCanBeInert(t *testing.T) {
	t.Parallel()
	out, errs, st := declRun(t, "readonly -n r=v; typeset -p r; r=5",
		func(s *Semantics) {
			withReadonlyNLetter(s)
			s.ReadonlyReferenceLetter = ReadonlyReferenceLetterIsInert
		}, Diagnostics{})
	if want := "declare -r r=\"v\"\n"; out != want {
		t.Errorf("listing = %q, want %q", out, want)
	}
	if st == 0 || errs == "" {
		t.Errorf("status %d, stderr %q; want the write to the frozen name refused", st, errs)
	}
}

// Nothing is taken *off*: a name the shell already froze stays frozen, which
// is what separates this letter from `export -n` and is why the fix is a skip
// of the mark rather than an unmark.
func TestTheReadonlyNLetterUnfreezesNothing(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t, "readonly q=1; readonly -n q; echo st=$?; typeset -p q",
		func(s *Semantics) {
			withReadonlyNLetter(s)
			s.ReadonlyReferenceLetter = ReadonlyReferenceLetterDeclaresAnUnfrozenName
		}, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if want := "st=0\ndeclare -r q=\"1\"\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// With no operand the letter is not a declaration at all: it is the same
// listing the bare word writes, which is what keeps `readonly -n` from
// falling through to the name loop with nothing to declare.
func TestTheReadonlyNLetterWithNoOperandIsTheListing(t *testing.T) {
	t.Parallel()
	out, _, st := declRun(t, "readonly a=1; b=2; readonly -n",
		func(s *Semantics) {
			withReadonlyNLetter(s)
			s.ReadonlyReferenceLetter = ReadonlyReferenceLetterDeclaresAnUnfrozenName
		}, Diagnostics{})
	if st != 0 {
		t.Fatalf("status %d, want 0", st)
	}
	if !strings.Contains(out, "a=\"1\"") || strings.Contains(out, "b=") {
		t.Errorf("listing = %q, want the frozen names alone", out)
	}
}

// And a dialect that spells the letter and has not said which of the two it
// is refuses by name, the way every unanswered axis does — a guess either
// hands a script a freeze it asked not to have or withholds one it did.
func TestTheReadonlyNLetterIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	_, errs, st := declRun(t, "readonly -n r=v", withReadonlyNLetter, Diagnostics{})
	if st == 0 {
		t.Fatalf("status 0 for an unanswered axis; want a refusal")
	}
	if !strings.Contains(errs, "the `n` letter of `readonly`") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
}
