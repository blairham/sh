// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// selecting turns on the grammar these tests need, by the construct's name
// rather than by a shell's.
func selecting(d *syntax.Dialect) {
	d.ParamElementSelection = true
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
}

// `${a:#pattern}` drops the elements the pattern matches, through all three
// spellings that reach a list: the flag group, the `[@]` subscript, and the
// two together.
//
// One test over the three because they are three routes into one operator and
// the way to get this wrong is to implement it on one of them — which is what
// happened first: the `[@]` route mapped each element through a function and
// so returned the array unchanged, because the shape of that route is one
// output per input and this operator has fewer.
func TestExclusionDropsMatchingElements(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"through a flag group",
			`a=(one two three); printf "[%s]" "${(@)a:#t*}"`,
			"[one]",
		},
		{
			"through an [@] subscript",
			`a=(one two three); printf "[%s]" "${a[@]:#t*}"`,
			"[one]",
		},
		{
			"a pattern with no metacharacter matches a whole element",
			`a=(one two three); printf "[%s]" "${(@)a:#two}"`,
			"[one][three]",
		},
		{
			// Counted rather than printed, because `printf` runs its format
			// once even with nothing to fill it: `[]` comes out both from an
			// empty list and from a list of one empty string, and those are
			// different answers. `$#` tells them apart, and it is zero here.
			"a pattern that matches everything leaves no field at all",
			`a=(one two); set -- "${(@)a:#*}"; printf "n=%d" "$#"`,
			"n=0",
		},
		{
			"a pattern that matches nothing leaves everything",
			`a=(one two); printf "[%s]" "${(@)a:#zzz}"`,
			"[one][two]",
		},
		{
			// The whole-match rule read from its sharpest end: an empty
			// pattern is not "no pattern" and not `*`, it matches the empty
			// string and only that.
			"an empty pattern takes the empty element and no other",
			`a=("" one); printf "[%s]" "${(@)a:#}"`,
			"[one]",
		},
		{
			"an empty array stays empty",
			`a=(); set -- "${(@)a:#x}"; printf "n=%d" "$#"`,
			"n=0",
		},
		{
			"matching is case-sensitive",
			`a=(One two); printf "[%s]" "${(@)a:#one}"`,
			"[One][two]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, tc.src, selecting, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A whole element and never a part of one, which is the entire difference
// between `:#` and `#`.
//
// Asserted side by side on one value, because the two operators are one
// character apart and one shell in the panel reads the pair as the *same*
// operator: `${v:#hel*}` is `lo` there and empty here. A test that only
// checked `:#` in isolation would pass for an implementation that had quietly
// become prefix removal.
func TestExclusionMatchesAWholeValueNotAPrefix(t *testing.T) {
	const src = `v=hello; printf "[%s][%s][%s]" "${v:#hel*}" "${v#hel*}" "${v:#xyz}"`
	out, _ := runGrammar(t, src, selecting, nil)
	if want := "[][lo][hello]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A value that is one string is the one-element list it is: the operator
// leaves it or empties it, and never returns part of it.
func TestExclusionOnAScalarIsAllOrNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"the pattern matches, so nothing is left", `v=/abs/p; printf "[%s]" "${v:#/*}"`, "[]"},
		{"it does not, so the value stands", `v=rel/p; printf "[%s]" "${v:#/*}"`, "[rel/p]"},
		{"an empty value against an empty pattern", `v=; printf "[%s]" "${v:#}"`, "[]"},
		{
			// The other half of the row above, and the reason a scalar is not
			// simply routed through the list code: emptying a *value* leaves
			// one empty field where emptying a *list* leaves none. Measured,
			// and the two are one character apart in the source.
			"an emptied value is still one field",
			`v=hello; set -- "${v:#hel*}"; printf "n=%d" "$#"`,
			"n=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, tc.src, selecting, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The pattern is built from the operand's *word*, so whether a metacharacter
// out of a parameter is a metacharacter is the dialect's answer and not a
// constant.
//
// Both sides of the axis on one source. The shell that has this operator
// answers No — measured, `p=t*; ${(@)a:#$p}` removes nothing there — and an
// implementation that handed the matcher a plain string would remove `two`
// under both answers and match the shell under neither.
func TestTheExclusionPatternFollowsTheGlobbingAxis(t *testing.T) {
	const src = `p='t*'; a=(one two); printf "[%s]" "${(@)a:#$p}"`
	for _, tc := range []struct {
		name string
		ans  Answer
		want string
	}{
		{"an expansion's result is not a pattern", No, "[one][two]"},
		{"an expansion's result is a pattern", Yes, "[one]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, src, selecting, func(r *Runner) {
				s := permissive()
				s.GlobExpansionResults = tc.ans
				r.Semantics = &s
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	// Quoted, it is a literal under either answer — the pattern's own rule,
	// not this operator's, and the row that says the operator did not reach
	// past the word to the text.
	out, _ := runGrammar(t, `a=(one two); printf "[%s]" "${(@)a:#"two"}"`, selecting, nil)
	if want := "[one]"; out != want {
		t.Errorf("quoted operand: got %q, want %q", out, want)
	}
}

// `:|` and `:*` name another array and compare elements for equality rather
// than by pattern.
//
// The pattern row is what separates them from `:#`: a metacharacter in the
// other array is a character, so an element `t*` removes only a literal `t*`.
// Without it, an implementation that routed all three through the matcher
// would pass every other row here.
func TestTheSetOperatorsCompareElementsForEquality(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			"difference keeps what the other does not hold",
			`a=(x y z); b=(y w); printf "[%s]" "${(@)a:|b}"`,
			"[x][z]",
		},
		{
			"intersection keeps only what it does",
			`a=(x y z); b=(y w); printf "[%s]" "${(@)a:*b}"`,
			"[y]",
		},
		{
			"a name nothing is stored under holds nothing, so difference keeps all",
			`a=(x y z); printf "[%s]" "${(@)a:|nope}"`,
			"[x][y][z]",
		},
		{
			"and intersection keeps none",
			`a=(x y z); set -- "${(@)a:*nope}"; printf "n=%d" "$#"`,
			"n=0",
		},
		{
			"the other array's elements are values and not patterns",
			`a=(one two); b=('t*'); printf "[%s]" "${(@)a:|b}"`,
			"[one][two]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runGrammar(t, tc.src, selecting, nil)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// Every other colon spelling still means what it always did, evaluated rather
// than only parsed.
//
// The parser has its own version of this guard; this is the half that would
// catch an operator wired into the wrong branch of the evaluator, where the
// parse is right and the answer is not.
func TestTheOtherColonFormsStillEvaluateAsBefore(t *testing.T) {
	const src = `v=abcdef; printf "[%s][%s][%s][%s][%s]" "${v:2}" "${v:2:2}" "${v: -2}" "${v:-alt}" "${v:+set}"`
	out, _ := runGrammar(t, src, selecting, nil)
	if want := "[cdef][cd][ef][abcdef][set]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The real line this change was made for, whole rather than reduced: a startup
// file taking its own hook out of the hook list and assigning the rest back.
func TestTheRealHookListLine(t *testing.T) {
	const src = `precmd_functions=(a fig_precmd b); ` +
		`precmd_functions=(${(@)precmd_functions:#fig_precmd}); ` +
		`printf "[%s]" "${precmd_functions[@]}"`
	out, _ := runGrammar(t, src, selecting, nil)
	if want := "[a][b]"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
