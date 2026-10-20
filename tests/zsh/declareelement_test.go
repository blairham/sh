// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// A declaration's subscripted operand writes the element here, and it takes
// one for `export` as well as for `typeset`.
//
// The `export` answer read No before this and was never reachable: the operand
// was glob-matched before the builtin saw it, so the line died as
// `no matches found: a[1]=v` and the name check was never asked (#1203).
func TestASubscriptedOperandWritesTheElement(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`typeset a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`export a[1]=v; echo "st=$? [${a[1]}]"`, "st=0 [v]\n"},
		{`typeset -x a[1]=v; echo "[${a[1]}]"`, "[v]\n"},
		{`typeset -A m; typeset m[k]=v; echo "[${m[k]}]"`, "[v]\n"},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// And the three things it will not let an element carry, each in its own
// words — measured 2026-09-07 against zsh 5.9.2, where all three end the
// script.
func TestASubscriptedOperandRefusesWhatBelongsToTheName(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{
			`readonly a[1]=v; echo after`,
			"zsh:readonly:1: a[1]: can't create readonly array elements\n",
		},
		{
			`typeset -r a[1]=v; echo after`,
			"zsh:typeset:1: a[1]: can't create readonly array elements\n",
		},
		{
			`typeset -i a[1]=5; echo after`,
			"zsh:typeset:1: a[1]: inconsistent array element or slice assignment\n",
		},
		{
			`f() { typeset a[1]=v; }; f; echo after`,
			"f:typeset: a[1]: can't create local array elements\n",
		},
		{
			`f() { local a[1]=v; }; f; echo after`,
			"f:local: a[1]: can't create local array elements\n",
		},
	} {
		out, st := runZsh(t, dir, tc.src)
		if out != tc.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", tc.src, out, st, tc.want)
		}
	}
}

// The store's own complaint about a subscript does not name the builtin where
// the refusals above do — and the first element is number one here, so `a[0]`
// is what asks the question.
func TestTheSubscriptRangeComplaintDoesNotNameTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`export a[0]=v`, `typeset a[0]=v`, `a[0]=v`} {
		out, st := runZsh(t, dir, src)
		if want := "zsh:1: a: assignment to invalid subscript range\n"; out != want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", src, out, st, want)
		}
	}
}

// One subscript and no more. `a[1][2]=v` is not a name with a subscript in any
// column, so it stays an ordinary word — and here that word is a pattern with
// no match, which is what makes the difference visible at all.
func TestASecondSubscriptIsNotPartOfTheName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset a[1][2]=v`)
	if want := "zsh:1: no matches found: a[1][2]=v\n"; out != want || st != 1 {
		t.Errorf("typeset a[1][2]=v = %q (status %d), want %q at 1", out, st, want)
	}
}

// `-g` takes no shadow, so it never asks the local refusal: the element lands
// on the global array and is there after the function returns.
func TestAGlobalElementDeclarationIsNotRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { typeset -g a[1]=v; }; f; echo "[${a[1]}]"`)
	if want := "[v]\n"; out != want || st != 0 {
		t.Errorf("typeset -g a[1]=v = %q (status %d), want %q at 0", out, st, want)
	}
}

// The status a declaration leaves behind when the *store* refuses its element
// depends on how the program arrived: 0 from `-c` and 1 from a script file.
//
// Measured 2026-09-14 against zsh 5.9.2, `-f` with a scratch `HOME`, with the
// output discarded so the number is the shell's own. The stderr is
// byte-identical on both routes and nothing after the refusal runs on either,
// so the status is the only observable and a probe that read the diagnostic
// cannot see this at all. #1770 was filed from `-c` alone and read as a
// divergence on every route; from a file this shell already agreed.
func TestAStoreRefusalOfADeclaredElementLeavesZeroFromACommandString(t *testing.T) {
	for _, src := range []string{
		`a=(x y); typeset a[0]=v`,
		`a=(x y); export a[0]=v`,
		`a=(x y); declare a[0]=v`,
		`a=(x y); local a[0]=v`,
		`a=(x y); typeset -g a[0]=v`,
		`a=(x y); typeset -x a[0]=v`,
		`a=(x y); typeset "a[0,0]"=v`,
		`v=abc; typeset v[0]=X`,
	} {
		for _, tc := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, 0},
			{interp.RouteScriptFile, 1},
			// Standard input is the third route and answers as a file does,
			// which is what says the rule is `-c`'s rather than "not a file".
			{interp.RouteStandardInput, 1},
		} {
			out, st := runZshRoute(t, t.TempDir(), src, tc.route)
			if st != tc.want {
				t.Errorf("%s from %v = status %d, want %d", src, tc.route, st, tc.want)
			}
			// The same sentence on every route, and the `echo` after it
			// never runs: a status that moved because the shell carried on
			// would be a different answer wearing the same number.
			want := "zsh:1: a: assignment to invalid subscript range\n"
			if strings.HasPrefix(src, "v=abc") {
				want = "zsh:1: v: assignment to invalid subscript range\n"
			}
			if out != want {
				t.Errorf("%s from %v = %q, want %q", src, tc.route, out, want)
			}
		}
	}
}

// And the refusals raised *before* the store keep their status on both routes,
// which is what makes the rule the store's rather than one about a bad
// subscript, about a declaration, or about the route. Each names the builtin
// in its location where the store's own complaint does not — the same seam.
func TestOnlyTheStoresOwnRefusalOfADeclaredElementFollowsTheRoute(t *testing.T) {
	for _, src := range []string{
		`a=(x y); readonly a[0]=v`,
		`a=(x y); integer a[0]=5`,
		`a=(x y); typeset -a a[0]=v`,
		`a=(x y); typeset -i a[0]=v`,
		`a=(x y); typeset -A a[0]=v`,
		`a=(x y); typeset 1bad=v`,
		// And the bare assignment, which reaches the identical complaint by a
		// route that is not a declaration at all.
		`a=(x y); a[0]=v`,
		`a=(x y); a[0]+=v`,
	} {
		for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile} {
			if _, st := runZshRoute(t, t.TempDir(), src, route); st != 1 {
				t.Errorf("%s from %v = status %d, want 1 on both routes", src, route, st)
			}
		}
	}
}

func runZshRoute(t *testing.T, dir, src string, route interp.Route) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir}, Route: route,
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
