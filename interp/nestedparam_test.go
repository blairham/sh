// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

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
		// The colon-less test asks whether the inner *came to* anything, and
		// it always did: an inner naming nothing comes to the empty string,
		// which is set. Measured — `${${u}-d}` on an unset u is empty in the
		// shell with the grammar, where `${u-d}` is `d`.
		{"a colon-less default over an unset inner does not fire", `printf "[%s]" "${${nosuch}-d}"`, "[]"},
		{"and the colon form does", `printf "[%s]" "${${nosuch}:-d}"`, "[d]"},
		// The same from the emptiest end: an inner that produces no field at
		// all still came to the empty string. `${${a[@]}-d}` on an empty
		// array is empty in the shell with the grammar.
		{"an inner that produced no field is still set", `a=(); printf "[%s]" "${${a[@]}-d}"`, "[]"},
		{"and its colon form still fires", `a=(); printf "[%s]" "${${a[@]}:-d}"`, "[d]"},
		// A metacharacter in the inner's value is a character of it. The
		// marks the expander carries a pattern with are the expander's own
		// bookkeeping, and an inner that kept them answered `a\*b`.
		{"a metacharacter in the value stays one character", `v="a*b"; printf "[%s]" "${${v}}"`, "[a*b]"},
		{"and the operator matches against it", `v="a*b"; printf "[%s]" "${${v}#a}"`, "[*b]"},
		{"a metacharacter through a flag group", `v="a*b"; printf "[%s]" "${${(U)v}}"`, "[A*B]"},
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
	// The element boundary is the thing a join destroys and no later
	// splitting puts back, so it is asserted with a separator *inside* an
	// element rather than with a count — and with the splitting axis pinned,
	// because a dialect that splits an unquoted expansion takes that boundary
	// apart again for reasons of its own. The shell with this grammar does
	// not split.
	unsplit := func(r *Runner) {
		sem := *r.Semantics
		sem.SplitParamExpansion = No
		r.Semantics = &sem
	}
	if out, st := runGrammar(t, `a=("one two" three); printf "[%s]" ${${a[@]}}`, nesting, unsplit); out != "[one two][three]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[one two][three]")
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

// An inner that comes to a **list** keeps every one of its elements, and the
// outer half applies to all of them.
//
// This shape was refused by name, and one spelling slipped past the refusal:
// an inner that had already lost its elements is a list of *one*, which joins
// to a plausible field at status 0 and says nothing (#1509). So every
// assertion here is on the elements themselves — a one-element list and a
// two-element list differ in the count and in the status too, and only the
// elements say which of the two this is.
//
// Measured on zsh 5.9.2, the one panel shell with the grammar. The elements
// survive quoting as a *join*, which is the reading a nesting with no
// subscript of its own has: `"${${a[@]}}"` is one field holding both, where
// `"${${a[@]}[@]}"` — the subscript said outright — is one field each. See
// nestedsub_test.go for that half.
func TestANestedInnerThatIsAListKeepsItsElements(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"unquoted, one field per element", `a=(one two); printf "[%s]" ${${a[@]}}`, "[one][two]"},
		{"quoted, one field with them joined", `a=(one two); printf "[%s]" "${${a[@]}}"`, "[one two]"},
		// The element boundary is the thing a join destroys and no later
		// splitting puts back, so it is asserted with a space inside an
		// element rather than with a count.
		{"the operator applies to each element", `a=(one two); printf "[%s]" ${${a[@]}#o}`, "[ne][two]"},
		{"and the quoted spelling joins what it made", `a=(one two); printf "[%s]" "${${a[@]}#o}"`, "[ne two]"},
		{"a substring slices the list", `a=(one two three); printf "[%s]" ${${a[@]}:1}`, "[two][three]"},
		{"an exclusion drops an element", `a=(one two); printf "[%s]" ${${a[@]}:#two}`, "[one]"},
		{"a length counts the elements", `a=(one two); printf "n=%s" ${#${a[@]}}`, "n=2"},
		// The count is where one element is not the same question as one
		// string: `${#${v}}` on the same five characters is 5.
		{"one element is counted, not measured", `a=(hello); printf "n=%s" ${#${a[@]}}`, "n=1"},
		// And an inner that produced no field at all is not a list of one
		// empty field: measured, `a=(); ${#${a[@]}}` is 0 where a list
		// holding one empty element would answer 1.
		{"an empty inner counts as none", `a=(); printf "n=%s" ${#${a[@]}}`, "n=0"},
		{"a further nesting keeps them", `a=(one two); printf "[%s]" ${${${a[@]}}}`, "[one][two]"},
		{"and joins them when the outermost is quoted", `a=(one two); printf "[%s]" "${${${a[@]}}}"`, "[one two]"},
		// An empty inner is not a list, and the colon test still fires on it.
		{"an empty inner still takes its default", `a=(); printf "[%s]" ${${a[@]}:-d}`, "[d]"},
		{"a set list does not", `a=(one two); printf "[%s]" ${${a[@]}:-d}`, "[one][two]"},
		{"an alternate over a list is its own word", `a=(one two); printf "[%s]" ${${a[@]}:+y}`, "[y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nesting, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
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
			sem := testSemantics()
			sem.SubstringRangeReadsModifiers = Yes
			r.Semantics = &sem
		})
	if want := "[/a/b]"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// The glob marks the expander carries a pattern with come off the inner's
// value, and the axis that puts them there is the one to assert it under: a
// dialect that reads an expansion's result as a pattern never escapes it, so
// the test that matters is the one where it does.
func TestANestedValueCarriesNoGlobMarks(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare nesting", `v="a*b"; printf "[%s]" "${${v}}"`, "[a*b]"},
		{"and one with an operator", `v="a*b"; printf "[%s]" "${${v}#a}"`, "[*b]"},
		{"and one through a flag group", `v="a*b"; printf "[%s]" "${${(U)v}}"`, "[A*B]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, nesting, func(r *Runner) {
				sem := testSemantics()
				// The answer that escapes an expansion's result, which is
				// what leaves marks for this to take off. Asserted under it
				// rather than under the answer where there is nothing to
				// take off and the code below could be missing entirely.
				sem.GlobExpansionResults = No
				r.Semantics = &sem
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestANestedBareArrayNameIsTheListLikeTheSubscriptedOne — the bare spelling
// reaches the same construct, now that a bare array name is the list where
// the dialect says so (#929).
//
// `${${a}}` and `${${a[@]}}` are one construct in the shell that has the
// grammar — measured, both are `[one][two]` there — so they must not be one
// answer and one refusal here, nor one answer and one *plausible* answer.
// Before #929 the bare inner joined to a single field and this expansion
// returned `one two` at status 0, which is the silent shape of the same gap.
func TestANestedBareArrayNameIsTheListLikeTheSubscriptedOne(t *testing.T) {
	listly := func(r *Runner) {
		sem := *r.Semantics
		sem.ArrayScalarIsTheWholeArray = Yes
		sem.ArrayNameWithoutSubscriptIsTheList = Yes
		r.Semantics = &sem
	}
	for _, tc := range []struct{ src, want string }{
		{`a=(one two); printf "[%s]" ${${a}}`, "[one][two]"},
		{`a=(one two); printf "[%s]" ${${a}#o}`, "[ne][two]"},
		{`a=(one two); printf "n=%s" ${#${a}}`, "n=2"},
	} {
		out, st := runGrammar(t, tc.src, nesting, listly)
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
	// Quoted, the same characters are a joined value and the operator applies
	// to it once, which is measured — so the refusal is about the list and
	// not about the spelling.
	if out, st := runGrammar(t, `a=(one two); printf "[%s]" "${${a}#o}"`, nesting, listly); out != "[ne two]" || st != 0 {
		t.Errorf("quoted: got %q at %d, want [ne two] at 0", out, st)
	}
	// A one-element array is not a list and still answers, so the refusal is
	// about the shape rather than about the name having been an array.
	out, st := runGrammar(t, `a=(one); printf "[%s]" ${${a}#o}`, nesting, listly)
	if out != "[ne]" || st != 0 {
		t.Errorf("got %q at %d, want [ne] at 0", out, st)
	}
}
