// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `readonly` inside a function where the value goes to the caller and the
// freeze stays with the call — Semantics.ReadonlyFreezeStaysWithTheCall. Tests
// name the axis, never shells.

// callFreeze answers the axis yes, over the answers that make it reachable:
// `readonly` declares no local of its own, and only a keyword-defined function
// has a scope for `typeset`.
func callFreeze(answer Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.ReadonlyDeclaresALocal = No
		s.ReadonlyFreezeStaysWithTheCall = answer
		s.TypesetLocalNeedsKeywordFunction = Yes
		s.DeclaredNameWithoutValueIsEmpty = No
		// Static scoping, which is what a scope taken only by a
		// keyword-defined function comes with.
		s.CallerLocalsReachTheCallee = No
		// And a refused write is reported and the line goes on, so a row can
		// show what the refusal left behind.
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = No
		s.ReadonlyReassignmentBySpecialBuiltinFatal = No
		s.ReadonlyRefusalInABuiltinIsFatal = No
	}
}

// The caller's name is writable after the call, and gets the value.
func TestReadonlyFreezeStaysWithTheCall(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"valueless, then written by the caller",
			`function f { readonly x; }; f; x=1; echo "x=$x"`,
			"x=1\n",
		},
		{
			"the value reaches the caller and the call reads its own empty local",
			`x=0; function f { readonly x=5; echo "in=${x-u}"; }; f; echo "out=$x"; x=1; echo "x=$x"`,
			"in=u\nout=5\nx=1\n",
		},
		{
			"the caller's array is back whole",
			`a=(p q); function f { readonly a; echo "in=${a[1]-u}"; }; f; echo "out=${a[*]}"`,
			"in=u\nout=p q\n",
		},
		{
			"a function a keyword function calls cannot see its caller's local",
			`function g { typeset x=1; f; echo "g=$x"; x=2; echo "g2=$x"; }; function f { readonly x=5; }; g; echo "top=$x"`,
			"g=1\ng2=2\ntop=5\n",
		},
		{
			"called twice",
			`function f { readonly x=1; }; f; f; echo ok`,
			"ok\n",
		},
	} {
		out, errs, st := declRun(t, c.src, callFreeze(Yes), Diagnostics{})
		if out != c.want || st != 0 || errs != "" {
			t.Errorf("%s: %q (stderr %q, status %d), want %q", c.name, out, errs, st, c.want)
		}
	}
}

// The freeze is still a freeze inside the call, and lands where it always did
// on a name the call has already declared, or where there is no scope.
func TestReadonlyFreezeStaysWithTheCallStillFreezes(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the call's own write is refused",
			`function f { readonly x; (x=6); echo "in=${x-u}"; }; f; echo "out=${x-u}"`,
			"in=u\nout=u\n",
		},
		{
			"a name the call already declared is frozen in the call",
			`function f { typeset x=3; readonly x=5; (x=4); echo "in=$x"; }; f; echo "out=${x-u}"`,
			"in=5\nout=u\n",
		},
		{
			"a second readonly meets the first one's local",
			`function f { readonly x=5; readonly x=6; }; f; echo "out=${x-u}"`,
			"out=5\n",
		},
		{
			"the value half is an assignment to the caller's frozen name",
			`readonly x=0; function f { readonly x=1; }; f; echo "out=$x"`,
			"out=0\n",
		},
	} {
		out, errs, st := declRun(t, c.src, callFreeze(Yes), Diagnostics{})
		if out != c.want || errs == "" {
			t.Errorf("%s: %q (stderr %q, status %d), want %q and a refusal", c.name, out, errs, st, c.want)
		}
	}
}

// Without a keyword-defined function there is no scope for the freeze to stay
// in, and with the axis answered no it never stays: either way the caller's
// name is the one frozen.
func TestReadonlyFreezeOutlivesACallWithNoScope(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src string
		set       func(*Semantics)
	}{
		{"a function with no keyword", `f() { readonly x; }; f; x=1; echo "x=$x"`, callFreeze(Yes)},
		{"the axis answered no", `function f { readonly x; }; f; x=1; echo "x=$x"`, callFreeze(No)},
	} {
		out, errs, _ := declRun(t, c.src, c.set, Diagnostics{})
		if out != "" || !strings.Contains(errs, "x") {
			t.Errorf("%s: %q (stderr %q), want the write refused", c.name, out, errs)
		}
	}
}

// The axis is asked only inside a function, and only where `readonly` declares
// no local of its own: neither a top-level line nor the dialect that scopes
// `readonly` reaches it, so an unanswered axis refuses neither.
func TestAnOrdinaryReadonlyAsksNothingOfTheCallFreeze(t *testing.T) {
	t.Parallel()
	out, errs, st := declRun(t, `readonly T=1; echo "[$T]"`, callFreeze(Unspecified), Diagnostics{})
	if want := "[1]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("a top-level readonly = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	out, errs, st = declRun(t, `function f { readonly T=1; echo "[$T]"; }; f`, func(s *Semantics) {
		callFreeze(Unspecified)(s)
		s.ReadonlyDeclaresALocal = Yes
	}, Diagnostics{})
	if want := "[1]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("a scoped readonly = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
	// And the control: inside a function, where the answers differ, it is asked.
	_, errs, _ = declRun(t, `function f { readonly T=1; }; f`, callFreeze(Unspecified), Diagnostics{})
	if errs == "" {
		t.Errorf("an unanswered axis inside a function asked nothing")
	}
}
