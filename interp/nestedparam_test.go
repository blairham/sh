// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// nesting turns on the grammar these expansions need, by the construct's name
// rather than by a shell's.
func nesting(d *syntax.Dialect) {
	d.NestedParamExpansion = true
	d.ParamExpansionFlags = true
	d.ParamElementSelection = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
}

// An expansion standing where a parameter name would: the outer operator acts
// on what the inner came to.
//
// Measured on zsh 5.9.2, which is the only panel shell with the grammar —
// bash 3.2, bash 5.3 and dash answer `bad substitution` and ksh93 a syntax
// error, and the corpus records that half.
func TestAnExpansionStandsWhereANameWould(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"the bare shape is the inner's value", `v=abc; printf "[%s]" "${${v}}"`, "[abc]"},
		{"a prefix trim on the result", `v=abc; printf "[%s]" "${${v}#a}"`, "[bc]"},
		{"a long prefix trim", `v=abc; printf "[%s]" "${${v}##*b}"`, "[c]"},
		{"a suffix trim", `v=abc; printf "[%s]" "${${v}%c}"`, "[ab]"},
		{"a long suffix trim", `v=abc; printf "[%s]" "${${v}%%b*}"`, "[a]"},
		{"a replacement", `v=abc; printf "[%s]" "${${v}/b/X}"`, "[aXc]"},
		{"a substring", `v=abc; printf "[%s]" "${${v}:1}"`, "[bc]"},
		{"a substring with a length", `v=abc; printf "[%s]" "${${v}:0:2}"`, "[ab]"},
		{"a length", `v=abc; printf "[%s]" "${#${v}}"`, "[3]"},
		{"an element exclusion", `v=abc; printf "[%s]" "${${v}:#a*}"`, "[]"},
		// The operator on the inside rather than the outside, which is the
		// reading a parser gets by accident if it splits at the first brace.
		{"the operator on the inner", `v=abc; printf "[%s]" "${${v#a}}"`, "[bc]"},
		{"nested twice", `v=abc; printf "[%s]" "${${${v}}}"`, "[abc]"},
		{"nested four deep", `v=abc; printf "[%s]" "${${${${v}}}}"`, "[abc]"},
		{"an operator at each level", `v=abc; printf "[%s]" "${${${v}#a}%c}"`, "[b]"},
		{"a flag group on the outer", `v=abc; printf "[%s]" "${(U)${v}}"`, "[ABC]"},
		{"a flag group on the inner", `v=abc; printf "[%s]" "${${(U)v}}"`, "[ABC]"},
		{"one on each", `v=abc; printf "[%s]" "${(U)${(L)v}}"`, "[ABC]"},
		{"a flag group and an operator", `v=abc; printf "[%s]" "${(U)${v}#a}"`, "[BC]"},
		// The inner is any expansion, not only another `${`.
		{"a command substitution as the inner", `printf "[%s]" "${$(echo abc)#a}"`, "[bc]"},
		{"an arithmetic expansion as the inner", `printf "[%s]" "${$((20+3))#2}"`, "[3]"},
		// The conditionals test what the inner came to, which is the whole
		// reason the idiom exists.
		{"a default over an empty result", `v=; printf "[%s]" "${${v}:-d}"`, "[d]"},
		{"a default over a set result", `v=abc; printf "[%s]" "${${v}:-d}"`, "[abc]"},
		{"an alternate over a set result", `v=abc; printf "[%s]" "${${v}:+y}"`, "[y]"},
		{"an unset inner is empty and not an error", `printf "[%s]" "${${nosuch}}"`, "[]"},
		// The startup-file idiom the issue was filed from, with the pieces it
		// needs: an exclusion inside a default, both nested.
		{"a pattern exclusion inside a default", `z=/bin/zsh; printf "[%s]" "${${z:#/bin/*}:-fallback}"`, "[fallback]"},
		{"the same when the exclusion does not fire", `z=/opt/zsh; printf "[%s]" "${${z:#/bin/*}:-fallback}"`, "[/opt/zsh]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nesting, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The inner expansion is the whole of the name position.
//
// Measured: `${x${v}}` and `${${v}x}` are a bad substitution in the shell that
// *has* the construct, so text either side of the nesting is not a longer name
// and an implementation that appended it would be answering a shape no shell
// reads.
func TestTextBesideANestedExpansionIsNotAName(t *testing.T) {
	for _, src := range []string{
		`v=abc; printf "[%s]" "${x${v}}"`,
		`v=abc; printf "[%s]" "${${v}x}"`,
	} {
		out, st := runGrammar(t, src, nesting, nil)
		if st == 0 {
			t.Errorf("%s: status 0, want the expansion refused", src)
		}
		if !strings.Contains(out, "bad substitution") {
			t.Errorf("%s: got %q, want a bad substitution", src, out)
		}
	}
}

// Two shapes are read and not implemented, and each says which.
//
// This is the half that keeps the gap findable. An inner that comes to a list
// keeps its fields in the shell with the grammar and the outer operator then
// applies to each; a subscript after the inner brace indexes the result. Both
// are measured, neither is built, and joining or ignoring would answer with a
// plausible value and say nothing — which is how the *last* gap on this
// surface stayed hidden.
func TestTheNestedShapesNotBuiltSayWhichTheyAre(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an inner that comes to a list",
			`a=(one two); printf "[%s]" "${${a[@]}}"`,
			"sh: ${${a[@]}}: a nested expansion of a list is not implemented\n",
		},
		{
			"a subscript on the result",
			`v=abc; printf "[%s]" "${${v}[2]}"`,
			"sh: ${${v}[2]}: a subscript on a nested expansion is not implemented\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nesting, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the unbuilt shape refused")
			}
		})
	}
}

// Without the grammar flag the same characters are not this shape at all, and
// the expansion is refused rather than read one shell's way.
func TestNestingIsAGrammarFlag(t *testing.T) {
	out, st := runGrammar(t, `v=abc; printf "[%s]" "${${v}#a}"`, func(d *syntax.Dialect) {
		d.ParamExpansionFlags = true
	}, nil)
	if st == 0 {
		t.Errorf("status 0, want the expansion refused without the flag")
	}
	if !strings.Contains(out, "bad substitution") {
		t.Errorf("got %q, want a bad substitution", out)
	}
}

// A nested expansion's inner runs its substitutions once, not once per
// operator — the same promise an ordinary operand makes.
func TestANestedInnerRunsOnce(t *testing.T) {
	const src = `printf "[%s]" "${$(printf x >>marks; echo abc)#a}"; printf "n=%s" "$(cat marks)"`
	out, st := runGrammar(t, src, nesting, nil)
	if want := "[bc]n=x"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The value the outer operator sees is the inner's, not the outer name's —
// there is no outer name. A runner that fell back to an empty name would give
// the same answer for `${${v}}` and `${${w}}`, which this tells apart.
func TestTheOuterOperatorSeesTheInnersValue(t *testing.T) {
	out, st := runGrammar(t, `v=abc; w=xyz; printf "[%s][%s]" "${${v}#a}" "${${w}#x}"`, nesting, nil)
	if want := "[bc][yz]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// A modifier after the inner brace is read too — `${${v}:h}` is the head of
// the path the inner came to. Its own semantics axis says whether `:h` is a
// modifier at all, so the test names it rather than relying on a default.
func TestAModifierFollowsANestedExpansion(t *testing.T) {
	out, st := runGrammar(t, `v=/a/b/c.txt; printf "[%s]" "${${v}:h}"`, nesting,
		func(r *Runner) {
			sem := bash.Semantics()
			sem.SubstringRangeReadsModifiers = Yes
			r.Semantics = &sem
		})
	if want := "[/a/b]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
