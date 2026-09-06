// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// subGrammar is the grammar this construct needs, named by the constructs and
// not by a shell: a subscript to put a group in, the group itself, and the
// brace-less spelling one row exercises.
func subGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArraySubscriptFlags = true
	d.BareSubscript = true
}

// runSub runs src with that grammar and the two answers the measured shell
// gives that a search depends on: arrays are indexed from one, and an
// expansion's result is not a pattern — the second so that a row showing a
// substituted value's metacharacters *are* live in a subscript is showing
// this construct's own rule rather than a dialect's.
func runSub(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, subGrammar, func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayBaseIsZero = No
		sem.GlobExpansionResults = No
		sem.SplitParamExpansion = No
		// The measured shell's answer, and the row that says a subscript is
		// never matched against the filesystem: with it off, a subscript
		// that globbed would quietly come back as itself and no test could
		// tell.
		sem.GlobNoMatchIsError = Yes
		r.Semantics = &sem
	})
}

// The array every row below searches. Five elements, with `beta` twice so
// that first and last are different answers, and every element ending in `a`
// so that a pattern has more than one match to choose between.
const subArray = `a=(alpha beta gamma beta delta); `

// The four selecting flags and their two no-match answers.
//
// The exact element and the exact index, never "no error": a subscript-flag
// bug returns a plausible element at status 0, so a row that asserted only
// the status would pass while the shell handed a script the wrong directory.
func TestASubscriptFlagGroupSelectsTheElementItNames(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"r is the first match", `${a[(r)*a]}`, "alpha"},
		{"R is the last match", `${a[(R)*a]}`, "delta"},
		{"i is the first index", `${a[(i)be*]}`, "2"},
		{"I is the last index", `${a[(I)be*]}`, "4"},
		{"r with no match is nothing", `${a[(r)zz]}`, ""},
		{"R with no match is nothing", `${a[(R)zz]}`, ""},
		{"i with no match is one past the end", `${a[(i)zz]}`, "6"},
		{"I with no match is one before the start", `${a[(I)zz]}`, "0"},
		{"the pattern is a pattern", `${a[(r)*mm*]}`, "gamma"},
		{"and e makes it a string", `${a[(re)*mm*]}`, ""},
		{"e finds what it is equal to", `${a[(re)gamma]}`, "gamma"},
		{"e reaches the index flags too", `${a[(ie)be*]}`, "6"},
		{"the last selecting flag wins", `${a[(ir)be*]}`, "beta"},
		{"and it wins the other way", `${a[(ri)be*]}`, "2"},
		{"n asks for the nth match", `${a[(rn:2:)*a]}`, "beta"},
		{"n counts backwards for R", `${a[(In:2:)*a]}`, "4"},
		{"n is an expression", `${a[(rn:1+1:)*ta]}`, "beta"},
		{"n below one is one", `${a[(in:0:)*a]}`, "1"},
		{"n past the last match misses", `${a[(in:9:)*a]}`, "6"},
		{"b moves the start forwards", `${a[(ib:3:)*a]}`, "3"},
		{"b moves it backwards for I", `${a[(Ib:2:)*a]}`, "2"},
		{"b counts back from the end", `${a[(ib:-2:)*a]}`, "4"},
		{"b below the first element is the first", `${a[(ib:0:)*a]}`, "1"},
		{"b past the end searches nothing", `${a[(ib:6:)*a]}`, "6"},
		{"and nothing in reverse either", `${a[(Ib:6:)*a]}`, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, subArray+`printf "[%s]" "`+tc.src+`"`)
			if want := "[" + tc.want + "]"; out != want {
				t.Errorf("%s = %q, want %q", tc.src, out, want)
			}
			if status != 0 {
				t.Errorf("%s: status %d, want 0", tc.src, status)
			}
		})
	}
}

// A group with none of the four selecting flags leaves the subscript read
// exactly as it would have been without a group — with one exception, which
// is measured and has a test of its own below: `@` and `*` stop naming the
// whole array as soon as anything opens a group.
func TestASubscriptFlagGroupThatSelectsNothingChangesNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an empty group", `${a[()2]}`, "beta"},
		{"a bare e", `${a[(e)2]}`, "beta"},
		{"an expression behind one", `${a[(e)1+1]}`, "beta"},
		{"a name behind one is zero, so no element", `${a[(e)beta]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, subArray+`printf "[%s]" "`+tc.src+`"`)
			if want := "[" + tc.want + "]"; out != want {
				t.Errorf("%s = %q, want %q", tc.src, out, want)
			}
			if status != 0 {
				t.Errorf("%s: status %d, want 0", tc.src, status)
			}
		})
	}
}

// The operand is the subscript's text with its substitutions performed and
// nothing else touched, which is one rule with three consequences.
func TestASubscriptSearchOperandIsTheTextAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a substituted value's metacharacters are live",
			`a=(alpha beta gamma); g='be*'; printf "[%s]" "${a[(r)$g]}"`,
			"[beta]",
		},
		{
			"a substitution reaches the operand at all",
			`a=(alpha beta gamma); d=beta; printf "[%s]" "${a[(r)$d]}"`,
			"[beta]",
		},
		{
			"quotes are matched rather than removed",
			`b=('"beta"' beta); printf "[%s]" "${b[(r)"beta"]}"`,
			`["beta"]`,
		},
		{
			"and the value inside them is still substituted",
			`b=('"beta"' beta); h=beta; printf "[%s]" "${b[(r)"$h"]}"`,
			`["beta"]`,
		},
		{
			"exact matching sees them too",
			`b=('"beta"' beta); printf "[%s]" "${b[(re)"beta"]}"`,
			`["beta"]`,
		},
		{
			"single quotes are matched as well",
			`b=("'beta'" beta); printf "[%s]" "${b[(r)'beta']}"`,
			`['beta']`,
		},
		{
			"a metacharacter across two spans is still not a filename",
			`b=('(e)beta' beta); printf "[%s]" "${b[(re)(e)beta]}"`,
			`[(e)beta]`,
		},
		{
			"a backslash escapes a metacharacter",
			`b=(bex 'be*'); printf "[%s]" "${b[(r)be\*]}" "${b[(r)be*]}"`,
			`[be*][bex]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// A name holding nothing and an array holding nothing are different answers,
// which is the row a search over the empty case would otherwise get right by
// accident.
func TestASubscriptSearchOverNothing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an unset name is not searched", `printf "[%s]" "${nope[(i)x]}"`, "[]"},
		{"nor for an element", `printf "[%s]" "${nope[(r)x]}"`, "[]"},
		{"a declared empty array is", `b=(); printf "[%s]" "${b[(i)x]}"`, "[1]"},
		{"in both directions", `b=(); printf "[%s]" "${b[(I)x]}"`, "[0]"},
		{"and holds no element", `b=(); printf "[%s]" "${b[(r)x]}"`, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// The positional parameters are searched like any other list.
func TestASubscriptSearchReachesThePositionalParameters(t *testing.T) {
	grammar := func(d *syntax.Dialect) {
		subGrammar(d)
		// `@` is not a name, so a subscript on it is a grammar question of
		// its own and the flag group rides on that answer rather than
		// widening it.
		d.SpecialParamSubscript = true
	}
	out, status := runGrammar(t, `printf "[%s]" "${@[(r)b]}" "${@[(i)c]}"`, grammar,
		func(r *Runner) {
			sem := *r.Semantics
			sem.ArrayBaseIsZero = No
			sem.GlobExpansionResults = No
			sem.SplitParamExpansion = No
			r.Semantics = &sem
			r.Params = []string{"a", "b", "c"}
			d := syntax.Core()
			grammar(&d)
			r.Dialect = &d
		})
	if out != "[b][3]" {
		t.Errorf("= %q, want %q", out, "[b][3]")
	}
	if status != 0 {
		t.Errorf("status %d, want 0", status)
	}
}

// The brace-less spelling reaches the same reading, which is the half a lexer
// change carries: without it the parenthesis ends the word and the file does
// not parse.
func TestASubscriptSearchWithoutBraces(t *testing.T) {
	out, status := runSub(t, subArray+`printf "[%s]" "$a[(r)*a]" "$a[(i)be*]" "$a[(re)be*]"`)
	if want := "[alpha][2][]"; out != want {
		t.Errorf("= %q, want %q", out, want)
	}
	if status != 0 {
		t.Errorf("status %d, want 0", status)
	}
}

// What is read and not carried is refused by name, and the refusal abandons
// the word rather than answering with a plausible element.
func TestASubscriptFlagThisImplementationDoesNotCarryIsRefusedByName(t *testing.T) {
	for _, tc := range []struct{ src, names string }{
		{`printf "[%s]" "${a[(w)beta]}"`, "(w)"},
		{`printf "[%s]" "${a[(f)beta]}"`, "(f)"},
		{`printf "[%s]" "${a[(p)beta]}"`, "(p)"},
		{`printf "[%s]" "${a[(k)beta]}"`, "(k)"},
		{`printf "[%s]" "${a[(K)beta]}"`, "(K)"},
		{`printf "[%s]" "${a[(s:,:)beta]}"`, "(s)"},
		{`printf "[%s]" "${a[(rw)beta]}"`, "(w)"},
	} {
		out, status := runSub(t, subArray+tc.src)
		if !strings.Contains(out, tc.names+" subscript flag is not implemented") {
			t.Errorf("%s: output %q does not refuse %s by name", tc.src, out, tc.names)
		}
		if !strings.Contains(out, "a[") {
			t.Errorf("%s: output %q does not name the subscript", tc.src, out)
		}
		if status == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
	}
}

// A search over a target this does not carry is refused the same way, and the
// refusal replaces a *silent* wrong answer in both cases: an associative
// array looked the group up as a key and found nothing, and a scalar answered
// out of the one-element list it is read as.
func TestASubscriptSearchOverATargetThisDoesNotCarryIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, why string }{
		{
			"an associative array",
			`typeset -A h; h[k1]=v1; printf "[%s]" "${h[(r)v1]}"`,
			"for an associative array",
		},
		{
			"a scalar",
			`s="one two"; printf "[%s]" "${s[(r)two]}"`,
			"for a scalar",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if !strings.Contains(out, "subscript flag is not implemented "+tc.why) {
				t.Errorf("output %q does not refuse %s", out, tc.why)
			}
			if status == 0 {
				t.Error("status 0, want a failure")
			}
		})
	}
	// A group that selects nothing is not a search, so it does not reach the
	// refusal and the key is still looked up.
	out, status := runSub(t, `typeset -A h; h[k1]=v1; printf "[%s]" "${h[()k1]}"`)
	if out != "[v1]" || status != 0 {
		t.Errorf("= %q status %d, want [v1] 0", out, status)
	}
}

// The shape zi.zsh writes six times, end to end: the answer decides whether a
// directory is prepended to a path that already holds it.
func TestTheShapeAPluginManagerAsksThisWith(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"already on the path, so nothing is added",
			`p=(/u/bin /pfx/bin /x); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"keep",
		},
		{
			"not on it, so it is",
			`p=(/u/bin /x); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"add",
		},
		{
			"and a prefix of an entry is not the entry",
			`p=(/pfx/bindir); Z=/pfx; if [ -z "${p[(re)${Z}/bin]}" ]; then printf add; else printf keep; fi`,
			"add",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runSub(t, tc.src)
			if out != tc.want {
				t.Errorf("= %q, want %q", out, tc.want)
			}
			if status != 0 {
				t.Errorf("status %d, want 0", status)
			}
		})
	}
}

// The one thing a group changes even when it selects nothing: `@` and `*` are
// no longer the whole array, so they reach the arithmetic and fail there.
//
// Measured on the shell that has the construct, and a wrong answer here is
// silent in the worst way — the whole array where the shell gives an error is
// a value a script will happily use.
func TestAFlagGroupTakesTheWholeArraySpellingsAway(t *testing.T) {
	for _, src := range []string{`${a[()@]}`, `${a[()*]}`, `${a[(e)@]}`, `${a[(r)@]}`} {
		out, _ := runSub(t, subArray+`printf "[%s]" "`+src+`"`)
		if strings.Contains(out, "alpha beta") || strings.Contains(out, "[alpha][beta]") {
			t.Errorf("%s = %q, want no array", src, out)
		}
	}
	// And without a group they still are, which is what says the clause is
	// about the group rather than about the brackets.
	out, status := runSub(t, subArray+`printf "[%s]" "${a[*]}"`)
	if want := "[alpha beta gamma beta delta]"; out != want || status != 0 {
		t.Errorf("${a[*]} = %q status %d, want %q 0", out, status, want)
	}
}
