// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A list expansion reaching a context that keeps no fields has to be joined
// into one string, and the character it joins with was a hard space chosen
// before there was a rule.
//
// Silent in the shape that is hardest to see: the default IFS begins with a
// space, so every script that leaves IFS alone gets the right answer and the
// one that sets it gets a wrong one at status 0.
//
// Measured 2026-09-06 with `IFS=-` in the four contexts that never split — an
// assignment's value, a `case` subject, a `[[ ]]` operand and a here-document
// body — plus an operator's pattern operand and a redirection's target,
// against bash 5.3.15, bash 3.2.57, that 5.3.15 build invoked as `sh`, dash,
// ksh93u+ 2012-08-01 and zsh 5.9.2. Two spellings and two answers:
//
//	spelling    bash 5.3   bash 3.2   ksh93      zsh    dash
//	${a[*]} $*  IFS        see below  IFS        IFS    IFS
//	${a[@]} $@  space      space      space      IFS    IFS
//
// So the star half is core and the at half is UnsplitAtListJoinsOnIFS. Two
// deviations are recorded rather than modeled: bash 3.2 joins an unquoted
// `${a[*]}` on a space while joining `$*` on IFS in the same build — it
// disagrees with itself, and no dialect here is graded against 3.2 — and
// ksh93 joins the star spelling on a space inside a here-document body while
// using IFS in every other context.

// joinSepRun runs src with the separator axis answered and nothing else moved.
func joinSepRun(t *testing.T, src string, atOnIFS Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.UnsplitAtListJoinsOnIFS = atOnIFS
	})
}

// The `@` spelling in each of the contexts that keeps no fields. Every one of
// them is the same answer in the same shell, which is what makes this one axis
// rather than one per context.
func TestTheAtSpellingsUnsplitJoinIsAnAxis(t *testing.T) {
	for _, c := range []struct{ name, src, onIFS, onSpace string }{
		{
			"an assignment's value",
			`IFS=-; a=(x y z); v=${a[@]}; printf "%s" "$v"`, "x-y-z", "x y z",
		},
		{
			"a case subject",
			`IFS=-; a=(x y z); case ${a[@]} in x-y-z) printf x-y-z;; "x y z") printf "x y z";; esac`, "x-y-z", "x y z",
		},
		{
			"a [[ ]] operand",
			`IFS=-; a=(x y z); [[ ${a[@]} = x-y-z ]] && printf x-y-z || printf "x y z"`, "x-y-z", "x y z",
		},
		{
			// The body is one blob of input, so it keeps the newline the
			// here-document ends with. Asserted rather than trimmed: this is
			// the one context whose output is not a word, and trimming it
			// would hide a join that had put a newline somewhere else.
			"a here-document body",
			"IFS=-; a=(x y z); cat <<E\n${a[@]}\nE", "x-y-z\n", "x y z\n",
		},
		{
			"the positional spelling",
			`IFS=-; set -- x y z; v=${@}; printf "%s" "$v"`, "x-y-z", "x y z",
		},
		{
			// The operand is a *pattern*, so what the join produces decides
			// whether it matches at all: joined on IFS it is the suffix
			// `x-y` and trims, joined on a space it is `x y` and does not.
			// The quietest face of the bug — a trim that silently did
			// nothing.
			"an operator's pattern operand",
			`IFS=-; set -- x y; v="Zx-y"; printf "%s" "${v%$@}"`, "Z", "Zx-y",
		},
	} {
		if out, st := joinSepRun(t, c.src, Yes); out != c.onIFS || st != 0 {
			t.Errorf("%s, joined on IFS: got %q status %d, want %q at 0", c.name, out, st, c.onIFS)
		}
		if out, st := joinSepRun(t, c.src, No); out != c.onSpace || st != 0 {
			t.Errorf("%s, joined on a space: got %q status %d, want %q at 0", c.name, out, st, c.onSpace)
		}
	}
}

// The `*` spelling is core and must not reach the axis: it joins on the first
// character of IFS whichever way the axis is answered, including when it is
// answered not at all — an unanswered axis is a refusal, so a reading that
// routed both spellings through one answer would pass the rows above and turn
// every one of these into a diagnostic.
func TestTheStarSpellingsUnsplitJoinIsNotTheAxis(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`IFS=-; a=(x y z); v=${a[*]}; printf "%s" "$v"`, "x-y-z"},
		{`IFS=-; set -- x y z; v=$*; printf "%s" "$v"`, "x-y-z"},
		{"IFS=-; a=(x y z); cat <<E\n${a[*]}\nE", "x-y-z\n"},
		{`IFS=-; set -- x y; v="Zx-y"; printf "%s" "${v%$*}"`, "Z"},
	} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			if out, st := joinSepRun(t, c.src, a); out != c.want || st != 0 {
				t.Errorf("%s with the axis %v: got %q status %d, want %q at 0", c.src, a, out, st, c.want)
			}
		}
	}
}

// A range subscript is on the star side of the line too — it joins on the
// same side as the *name* it was written on, which is the rule the quoted
// spelling already carried and which starSpelled asks for rather than
// restating. Its own two axes are answered here so the row reaches the
// question it is about; the separator is what is being asserted.
func TestARangeSubscriptJoinsOnIFSWithoutTheAxis(t *testing.T) {
	const src = `IFS=-; a=(x y z); v=${a[1,2]}; printf "%s" "$v"`
	for _, a := range []Answer{Yes, No, Unspecified} {
		out, st := axisRun(t, src, func(s *Semantics) {
			s.ArrayBaseIsZero = No
			s.SubscriptCommaIsARange = Yes
			s.UnsplitAtListJoinsOnIFS = a
		})
		if out != "x-y" || st != 0 {
			t.Errorf("with the separator axis %v: got %q status %d, want %q at 0", a, out, st, "x-y")
		}
	}
}

// An IFS that is set and empty is a live answer here, not a reason to stay
// quiet. That is what separates this question from UnquotedListJoinsOnIFS,
// whose guard skips an empty IFS because there is nothing to join *with* — the
// join happens either way here and joining with nothing is exactly what one
// shell does.
func TestAnEmptyIFSJoinsWithNothing(t *testing.T) {
	const star = `IFS=""; a=(x y); v=${a[*]}; printf "%s" "$v"`
	if out, st := joinSepRun(t, star, No); out != "xy" || st != 0 {
		t.Errorf("star: got %q status %d, want %q at 0", out, st, "xy")
	}
	const at = `IFS=""; a=(x y); v=${a[@]}; printf "%s" "$v"`
	if out, st := joinSepRun(t, at, Yes); out != "xy" || st != 0 {
		t.Errorf("at, joined on IFS: got %q status %d, want %q at 0", out, st, "xy")
	}
	if out, st := joinSepRun(t, at, No); out != "x y" || st != 0 {
		t.Errorf("at, joined on a space: got %q status %d, want %q at 0", out, st, "x y")
	}
}

// Asked only at the disagreement. Under the default IFS the two readings are
// the same character, and a list of one uses no separator at all — so neither
// may demand a dialect. Unspecified is the test: an unanswered axis is a
// refusal, so these would fail rather than merely differ.
func TestTheUnsplitJoinIsNotAskedWhereTheReadingsAgree(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); v=${a[@]}; printf "%s" "$v"`, "x y z"},
		{`unset IFS; a=(x y z); v=${a[@]}; printf "%s" "$v"`, "x y z"},
		{`IFS=" -"; a=(x y z); v=${a[@]}; printf "%s" "$v"`, "x y z"},
		{`IFS=-; a=(solo); v=${a[@]}; printf "%s" "$v"`, "solo"},
		{`IFS=-; a=(); v=${a[@]}; printf "[%s]" "$v"`, "[]"},
	} {
		if out, st := joinSepRun(t, c.src, Unspecified); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0 with no dialect asked", c.src, out, st, c.want)
		}
	}
}

// The quoted spellings are a different question and are already core: they
// must not move. `"${a[*]}"` is one field joined on IFS and `"${a[@]}"` is one
// field per element, in every shell measured.
func TestTheQuotedSpellingsAreUnaffected(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`IFS=-; a=(x y z); set -- "${a[*]}"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[x-y-z]`},
		{`IFS=-; a=(x y z); set -- "${a[@]}"; printf "%d" "$#"; printf "[%s]" "$@"`, `3[x][y][z]`},
		{`IFS=-; set -- x y z; set -- "$*"; printf "%d" "$#"; printf "[%s]" "$@"`, `1[x-y-z]`},
	} {
		for _, a := range []Answer{Yes, No, Unspecified} {
			if out, st := joinSepRun(t, c.src, a); out != c.want || st != 0 {
				t.Errorf("%s with the axis %v: got %q status %d, want %q at 0", c.src, a, out, st, c.want)
			}
		}
	}
}
