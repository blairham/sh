// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `readonly` written inside a function: a declaration word with a scope of
// its own, or POSIX's freeze on the name the shell already has —
// Semantics.ReadonlyDeclaresALocal. Tests name the axis, never shells.

// scopedReadonly is declRun's setter for the answer that takes a scope, with
// the empty a declared name holds answered so the valueless form has
// something to report.
func scopedReadonly(s *Semantics) {
	s.ReadonlyDeclaresALocal = Yes
	s.DeclaredNameWithoutValueIsEmpty = Yes
}

// frozenReadonly is the other answer: the name the shell already has is the
// one that is frozen, wherever the line was written.
func frozenReadonly(s *Semantics) {
	s.ReadonlyDeclaresALocal = No
	s.DeclaredNameWithoutValueIsEmpty = Yes
}

// Where the scope is taken the declaration belongs to the call: the body
// sees its own value and the name is gone when the function returns.
func TestReadonlyInsideAFunctionCanDeclareALocal(t *testing.T) {
	const src = `b() { readonly B=1; echo "in=[$B]"; }
b
echo "out=[${B-unset}]"`
	out, errs, st := declRun(t, src, scopedReadonly, Diagnostics{})
	want := "in=[1]\nout=[unset]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a scoped readonly = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declRun(t, src, frozenReadonly, Diagnostics{})
	want = "in=[1]\nout=[1]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a freezing readonly = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// And the caller's own value comes back, which is the half that says a scope
// was taken rather than the name merely having been absent afterwards.
func TestAScopedReadonlyLeavesTheCallerSValueAlone(t *testing.T) {
	const src = `B=out
b() { readonly B=1; echo "in=[$B]"; }
b
echo "out=[$B]"`
	out, errs, st := declRun(t, src, scopedReadonly, Diagnostics{})
	want := "in=[1]\nout=[out]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a scoped readonly over a caller's value = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The outer name is writable again afterwards too: the freeze the call added
// went away with the scope, so a shadow that had merely hidden the value
// while leaving the attribute standing is a different answer from this one.
func TestAScopedReadonlySFreezeGoesAwayWithTheCall(t *testing.T) {
	out, errs, st := declRun(t, `B=out
b() { readonly B=1; }
b
B=9
echo "out=[$B]"`, scopedReadonly, Diagnostics{})
	want := "out=[9]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("writing the outer name after the call = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// What the scope is worth to a script rather than to a listing: the function
// is an ordinary function and calling it twice prints twice. Without the
// scope the second call meets the name the first left behind, which is a
// refusal — and the fatal answer to that is what makes such a function
// callable exactly once.
func TestAFunctionDeclaringAScopedReadonlyIsCallableTwice(t *testing.T) {
	out, errs, st := declRun(t, `rf() { readonly RF=fixed; echo "in=[$RF]"; }
rf
rf
echo after`, scopedReadonly, Diagnostics{})
	want := "in=[fixed]\nin=[fixed]\nafter\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("two calls = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A valueless declaration into the scope reports what a declared name holds
// rather than what the caller held: the cell the shadow made is new, which
// is what `fresh` says everywhere else a declaration word takes one.
func TestAValuelessScopedReadonlyDeclaresTheLocalRatherThanReadingTheCaller(t *testing.T) {
	out, errs, st := declRun(t, `B=out
b() { readonly B; echo "in=[${B-unset}]"; }
b
echo "out=[$B]"`, scopedReadonly, Diagnostics{})
	want := "in=[]\nout=[out]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a valueless scoped readonly = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The ordering corner, and the one a scope changes the answer to. An array
// literal has to be assigned before a *freezing* `readonly` locks the name,
// or the command refuses its own value — and it has to be assigned after a
// *scoping* one shadows it, or the elements land in the caller. A shell
// doing only the first answers an empty array inside the function and the
// elements outside it, at status 0.
func TestAScopedReadonlySArrayLiteralLandsInTheLocal(t *testing.T) {
	const src = `k() { readonly -a R=(a b); echo "in=[${R[*]}]"; }
k
echo "out=[${R[*]-unset}]"`
	out, errs, st := declRun(t, src, scopedReadonly, Diagnostics{})
	want := "in=[a b]\nout=[unset]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a scoped readonly array literal = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
	out, errs, st = declRun(t, src, frozenReadonly, Diagnostics{})
	want = "in=[a b]\nout=[a b]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a freezing readonly array literal = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// At the top level there is no scope to take and the axis is never asked, so
// a dialect that has not answered it still freezes a name written there.
// That is what keeps the question at the disagreement: an unanswered axis
// refuses the line it is asked on, and every shell in the panel agrees about
// this one.
func TestReadonlyAtTheTopLevelDoesNotAskTheAxis(t *testing.T) {
	out, errs, st := declRun(t, `readonly T=1
echo "[$T]"`, func(s *Semantics) {
		s.ReadonlyDeclaresALocal = Unspecified
	}, Diagnostics{})
	want := "[1]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a top-level readonly = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
