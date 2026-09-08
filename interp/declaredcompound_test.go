// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration that names an array letter and no value: `local -a opts`.
//
// The panel is unanimous that the name is an array holding nothing rather
// than a scalar holding nothing — `f() { local -a a; echo ${#a[@]}; }` is 0 in
// every shell that takes the letter — so it is a rule of the core and not an
// axis. What the shells disagree about is a different question this does not
// ask: whether the declared name is *set*, which is
// DeclaredNameWithoutValueIsEmpty and is answered on both sides below.
//
// `${#a[@]}` is the read that tells the two apart, and it is the only one
// that does: an empty scalar and an empty array both answer 0 to `${#a}` and
// both print nothing. That is what made #1535 silent — `local` had the
// table's half of the mark and not the array's, so a name a function declared
// `local -a` was a string for the rest of the function, and every later
// `${opts[@]}` was a plausible answer to the wrong question.
func withArrayLetters(empty Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.LocalOptions = "aAilprux"
		s.DeclaredNameWithoutValueIsEmpty = empty
		s.DeclareListing = DeclareListingClustered
		s.BareDeclarationListing = DeclareListingPlainAssignment
		// Not what any of this is about: which functions have a scope, and
		// whether a subscripted operand declares one. Answered flat so an
		// unanswered axis cannot stand in for the array the tests look for.
		s.TypesetLocalNeedsKeywordFunction = No
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = Yes
		s.SubscriptedOperandTakesALocalDeclaration = Yes
		s.ArraysAreSparse = No
		s.DeclarePrintReportsAMissingName = Yes
	}
}

func TestAValuelessArrayDeclarationLeavesAnArray(t *testing.T) {
	// One line per word that declares, because they are three loops rather
	// than one and the mark went missing from exactly one of them.
	for _, decl := range []string{"local -a a", "typeset -a a"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; echo "n=${#a[@]}"; a+=(z); echo "[${a[@]}] n=${#a[@]}"; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "n=0\n[z] n=1\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// Note what this does *not* claim. Where a declared name without a value is
// unset rather than empty, the panel says the attribute survives the
// declaration anyway — bash 5.3.15 lists `declare -a a` for a `local -a a`
// that `${a+S}` calls unset — and this shell drops the name entirely there.
// That is a second gap and not this one: `${#a[@]}` is 0 either way, so it
// costs a listing rather than handing a script a string where it declared an
// array. It wants its own issue and its own measurement of what an unset name
// with an attribute even is here.

// The table letter, which is the half `local` already had — here so that the
// shared mark cannot lose it while gaining the other.
func TestAValuelessTableDeclarationLeavesATable(t *testing.T) {
	for _, decl := range []string{"local -A m", "typeset -A m"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; m[k]=v; echo "[${m[k]}] n=${#m[@]}"; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "[v] n=1\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// The `+` spelling of the same letters takes nothing off and — the half a
// shared mark could lose — brings nothing into being either. Measured: real
// zsh's `typeset +a a` leaves `typeset a=”`, a scalar, and bash 5.3.15's
// `declare +a a` leaves `declare -- a` with no array attribute at all; ksh93
// refuses the spelling outright. So a mark that ignored the `+` would answer
// a declaration that removes an attribute by creating the store for it.
func TestThePlusSpellingBringsNoCompoundIntoBeing(t *testing.T) {
	for _, decl := range []string{"typeset +a a", "local +a a", "typeset +A a", "local +A a"} {
		t.Run(decl, func(t *testing.T) {
			src := "f() { " + decl + `; typeset -p a; }
f`
			out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
			const want = "declare -- a=\"\"\n"
			if out != want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", decl, out, errs, st, want)
			}
		})
	}
}

// A subscripted operand carries the attribute to the *base* name, which is
// the third loop through the shared mark. It is a guard rather than a probe:
// the element write brings the array into being on its own here, so the mark
// changes nothing — the reason to run it is that the fold must not make the
// third loop start doing something the other two do not.
func TestASubscriptedArrayDeclarationLeavesAnArray(t *testing.T) {
	src := `f() { local -a a[2]=v; echo "n=${#a[@]} [${a[2]}]"; }
f`
	out, errs, st := declRun(t, src, withArrayLetters(Yes), Diagnostics{})
	const want = "n=3 [v]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("= %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
