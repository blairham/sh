// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration in a function defined without the `function` word, where only
// a keyword-defined call has a scope (Semantics.TypesetLocalNeedsKeywordFunction):
// it declares into the keyword call it runs in, and onto the shell's own name
// only where there is none. Tests name the axis, never shells.
func TestTypesetLocalNeedsKeywordFunctionDeclaresIntoTheKeywordCaller(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, want string
	}{
		{
			"the caller's local, gone with the caller",
			`function g { h; echo "g=${x-u}"; }; h() { typeset x=5; }; g; echo "top=${x-u}"`,
			"g=5\ntop=u\n",
		},
		{
			"the caller's local shadows a set name, which is back after it",
			`x=1; function g { h; echo "g=$x"; }; h() { typeset x=5; x=6; }; g; echo "top=$x"`,
			"g=6\ntop=1\n",
		},
		{
			"through two calls without the keyword",
			`function g { k; echo "g=${x-u}"; }; k() { h; echo "k=${x-u}"; }; h() { typeset x=5; }; g; echo "top=${x-u}"`,
			"k=5\ng=5\ntop=u\n",
		},
		{
			"the innermost keyword call, not an outer one",
			`function o { g; echo "o=${x-u}"; }; function g { h; echo "g=${x-u}"; }; h() { typeset x=5; }; o; echo "top=${x-u}"`,
			"g=5\no=u\ntop=u\n",
		},
		{
			"a keyword call the caller makes does not see it",
			`x=0; function k { echo "k=$x"; }; function g { h; k; echo "g=$x"; }; h() { typeset x=5; }; g; echo "top=$x"`,
			"k=0\ng=5\ntop=0\n",
		},
		{
			"no keyword call below: the shell's own name",
			`h() { typeset x=5; }; h; echo "top=${x-u}"`,
			"top=5\n",
		},
		{
			"the freeze of readonly takes the same scope, the value the name the call sees",
			`function g { h; echo "g=${x-u}"; }; h() { readonly x=5; }; g; echo "top=${x-u}"; x=1; echo "top2=$x"`,
			"g=u\ntop=5\ntop2=1\n",
		},
		{
			"a typeset freeze is the caller's and goes with it",
			`function g { h; echo "g=${x-u}"; }; h() { typeset -r x=5; }; g; echo "top=${x-u}"; x=1; echo "top2=$x"`,
			"g=5\ntop=u\ntop2=1\n",
		},
	} {
		out, errs, st := declRun(t, c.src, callFreeze(Yes), Diagnostics{})
		if out != c.want || st != 0 || errs != "" {
			t.Errorf("%s: %q (stderr %q, status %d), want %q", c.name, out, errs, st, c.want)
		}
	}
}

// Where every function has a scope of its own, the same line declares into
// the function it is written in, and nothing is lent.
func TestTypesetLocalNeedsKeywordFunctionNoKeepsItsOwnScope(t *testing.T) {
	t.Parallel()
	set := func(s *Semantics) {
		callFreeze(Yes)(s)
		s.TypesetLocalNeedsKeywordFunction = No
	}
	src := `function g { h; echo "g=${x-u}"; }; h() { typeset x=5; echo "h=$x"; }; g; echo "top=${x-u}"`
	out, errs, st := declRun(t, src, set, Diagnostics{})
	if want := "h=5\ng=u\ntop=u\n"; out != want || st != 0 || errs != "" {
		t.Errorf("%q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}
