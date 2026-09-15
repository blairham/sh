// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// Who can see a name a function declared local. Every shell in the panel but
// one hands it down to whatever that function calls; one hands down the
// shell's own name instead. See interp/staticscope.go and
// Semantics.CallerLocalsReachTheCallee (#2865).

// staticRun runs src with the `function` word available and the two axes
// these tests move set, and nothing else about declarations changed.
func staticRun(t *testing.T, src string, reach, keywordGated Answer) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(d *syntax.Dialect) { d.FunctionKeyword = true },
		func(r *Runner) {
			s := testSemantics()
			s.TypesetLocalNeedsKeywordFunction = keywordGated
			s.CallerLocalsReachTheCallee = reach
			r.Semantics = &s
		})
}

// TestACalleeSeesTheCallerLocalOrTheGlobal is the axis, both ways, over the
// issue's own case.
//
// The global is set on purpose rather than left absent: with no `v` at all
// both answers print the same thing under `${v-…}` only by accident, and a
// value there makes the two sides differ in what they *say* rather than in
// what they omit.
func TestACalleeSeesTheCallerLocalOrTheGlobal(t *testing.T) {
	const src = `function callee { printf '[%s]\n' "${v-UNSET}"; }
function caller { typeset v=local; callee; }
v=global
caller
`
	for _, tc := range []struct {
		name  string
		reach Answer
		want  string
	}{
		{"handed down", Yes, "[local]\n"},
		{"read past", No, "[global]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := staticRun(t, src, tc.reach, Yes)
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// TestACalleeWithNoGlobalToSeeReadsNothing is the same axis where the shell
// has no such name at all, which is the half a test with a global cannot
// show: reading past the caller's local means the name is *unset*, not that
// it holds some other value.
func TestACalleeWithNoGlobalToSeeReadsNothing(t *testing.T) {
	const src = `function callee { printf '[%s]\n' "${v-UNSET}"; }
function caller { typeset v=local; callee; }
caller
`
	if out, _ := staticRun(t, src, No, Yes); out != "[UNSET]\n" {
		t.Errorf("out=%q, want the name to read as unset", out)
	}
	if out, _ := staticRun(t, src, Yes, Yes); out != "[local]\n" {
		t.Errorf("out=%q, want the caller's local", out)
	}
}

// TestReadingPastACallerLocalIsASwapAndNotAHiding is the half a value-only
// reading gets wrong. The name the sealed body writes is the shell's own, so
// the write has to outlive the seal — and the caller's local has to be
// untouched by it.
func TestReadingPastACallerLocalIsASwapAndNotAHiding(t *testing.T) {
	const src = `function writes { v=written; }
function around { typeset v=local; writes; printf 'kept [%s]\n' "$v"; }
v=global
around
printf 'global [%s]\n' "$v"
`
	out, st := staticRun(t, src, No, Yes)
	const want = "kept [local]\nglobal [written]\n"
	if st != 0 || out != want {
		t.Errorf("out=%q st=%d, want %q", out, st, want)
	}
	// And the other answer, where the write lands on the caller's local and
	// the shell's own name is left as it was.
	out, _ = staticRun(t, src, Yes, Yes)
	const dynamic = "kept [written]\nglobal [global]\n"
	if out != dynamic {
		t.Errorf("out=%q, want %q", out, dynamic)
	}
}

// TestOnlyACallWithAScopeOfItsOwnReadsPastTheCaller: where the dialect ties a
// local scope to the `function` word, a body written the other way has no
// scope of its own and is not a boundary — it shares the caller's, so it sees
// the local whatever the axis says.
func TestOnlyACallWithAScopeOfItsOwnReadsPastTheCaller(t *testing.T) {
	const src = `posix() { printf '[%s]\n' "${v-UNSET}"; }
function caller { typeset v=local; posix; }
v=global
caller
`
	if out, st := staticRun(t, src, No, Yes); st != 0 || out != "[local]\n" {
		t.Errorf("out=%q st=%d, want the POSIX-form callee to share the scope", out, st)
	}
	// And where the dialect gives every call a scope, the same body is a
	// boundary like any other — which is what keeps the gate above from
	// being a test on the spelling.
	if out, _ := staticRun(t, src, No, No); out != "[global]\n" {
		t.Errorf("out=%q, want every call to be a boundary there", out)
	}
}

// TestMoreThanTheValueTravelsPastACallerLocal: the sealed body reads the
// shell's own array, table and unset-ness, and carries none of the caller's
// attributes. Each is a store of its own, and a seal over the scalar alone
// would pass the tests above and fail every row here.
func TestMoreThanTheValueTravelsPastACallerLocal(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an array",
			`function callee { printf '[%s]\n' "${a[0]-UNSET}"; }
function caller { typeset -a a=(local); callee; }
a=(global)
caller
`, "[global]\n",
		},
		{
			"a table",
			`function callee { printf '[%s]\n' "${m[k]-UNSET}"; }
function caller { typeset -A m=([k]=local); callee; }
typeset -A m=([k]=global)
caller
`, "[global]\n",
		},
		{
			"a local the caller unset",
			`function callee { printf '[%s]\n' "${v-UNSET}"; }
function caller { typeset v; unset v; callee; }
v=global
caller
`, "[global]\n",
		},
		{
			"an integer attribute",
			`function callee { v=010; printf '[%s]\n' "$v"; }
function caller { typeset -i v=1; callee; }
v=global
caller
`, "[010]\n",
		},
		{
			"a frozen local",
			`function callee { v=assigned; printf '[%s]\n' "$v"; }
function caller { typeset -r v=frozen; callee; }
v=global
caller
`, "[assigned]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := staticRun(t, tc.src, No, Yes)
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// TestTheSealIsTheShellsNamesAndNotTheDefinitionSite: a function defined
// inside another one still reads the shell's own name, so the parent of a
// sealed scope is the global and not wherever the body was written.
func TestTheSealIsTheShellsNamesAndNotTheDefinitionSite(t *testing.T) {
	const src = `function outer {
  typeset v=hidden
  function inner { printf '[%s]\n' "${v-UNSET}"; }
  inner
}
v=global
outer
`
	if out, st := staticRun(t, src, No, Yes); st != 0 || out != "[global]\n" {
		t.Errorf("out=%q st=%d, want the shell's own name", out, st)
	}
}

// TestASealedCallReachesPastEveryCallerAndNotOnlyTheNearest: two nested
// declarations of one name, read from a third call. A seal that undid only
// the innermost would print the outer function's local.
func TestASealedCallReachesPastEveryCallerAndNotOnlyTheNearest(t *testing.T) {
	const src = `function deep { printf '[%s]\n' "${v-UNSET}"; }
function mid { typeset v=two; deep; }
function top { typeset v=one; mid; }
v=global
top
`
	if out, st := staticRun(t, src, No, Yes); st != 0 || out != "[global]\n" {
		t.Errorf("out=%q st=%d, want the shell's own name past both calls", out, st)
	}
}

// TestEveryCallerGetsItsOwnDeclarationBack is the return path, asserted at
// each depth: the seal is lifted in the reverse of the order it went up, so
// the two nested locals come back to the calls that made them.
func TestEveryCallerGetsItsOwnDeclarationBack(t *testing.T) {
	const src = `function deep { v=written; }
function mid { typeset v=two; deep; printf 'mid [%s]\n' "$v"; }
function top { typeset v=one; mid; printf 'top [%s]\n' "$v"; }
v=global
top
printf 'global [%s]\n' "$v"
`
	out, st := staticRun(t, src, No, Yes)
	const want = "mid [two]\ntop [one]\nglobal [written]\n"
	if st != 0 || out != want {
		t.Errorf("out=%q st=%d, want %q", out, st, want)
	}
}
