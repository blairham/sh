// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A flag group in a context with room for exactly one word.
//
// An assignment's value, a `case` subject, a `[[ ]]` operand and a
// here-string keep no fields, so a group's words are joined there — and every
// step below the join then sees one word. That is the whole of #1705: the
// ordering flags find nothing left to order, where the same characters on a
// command line sort five fields.
//
// Measured on zsh 5.9.2, 2026-09-12. The instrument throughout is a value
// with a **double** space in it, because a value left whole and a value split
// and rejoined are the same nine characters otherwise.
func TestAScalarContextJoinsAFlagGroupsWords(t *testing.T) {
	const f = `f() { printf "b  b\na a\nc\n"; }; `
	const g = `g() { printf "b\na\nb\n"; }; `
	const h = `h() { printf "3\n1\n2\n"; }; `
	for _, tc := range []struct{ name, src, want string }{
		// The control rows: the split happens, and the separator the group
		// named is honored. Both readings agree on these, which is what
		// makes the rows below a claim about the ordering step alone.
		{"the fields are joined", f + `x=${$(f)}; printf "[%s]" "$x"`, "[b b a a c]"},
		{"on the separator the group named", f + `x=${(j:-:)$(f)}; printf "[%s]" "$x"`, "[b-b-a-a-c]"},

		// #1705 itself.
		{"so a sort has one word to order", f + `x=${(o)$(f)}; printf "[%s]" "$x"`, "[b b a a c]"},
		{"descending too", f + `x=${(O)$(f)}; printf "[%s]" "$x"`, "[b b a a c]"},
		{"and the numeric sort", h + `x=${(n)$(h)}; printf "[%s]" "$x"`, "[3 1 2]"},
		{"and the folded one", h + `x=${(i)$(h)}; printf "[%s]" "$x"`, "[3 1 2]"},
		{"and the signed one", h + `x=${(-)$(h)}; printf "[%s]" "$x"`, "[3 1 2]"},
		{"and the index key reversed", h + `x=${(Oa)$(h)}; printf "[%s]" "$x"`, "[3 1 2]"},
		{"nor is there a repeat left to drop", g + `x=${(u)$(g)}; printf "[%s]" "$x"`, "[b a b]"},
		{"which the named separator also survives", g + `x=${(uj:-:)$(g)}; printf "[%s]" "$x"`, "[b-a-b]"},

		// The same characters where fields have somewhere to go. Every
		// ordering row above is refuted by its own list-context twin, which
		// is what says the flags themselves still work.
		{"a list context sorts them", f + `printf "[%s]" ${(o)$(f)}`, "[a][a][b][b][c]"},
		{"and dedups them", g + `printf "[%s]" ${(u)$(g)}`, "[b][a]"},

		// Where the join sits, from below: every step under it sees one
		// word. A pad that ran on the fields would measure each of the three
		// against the width instead of the nine characters they came to.
		{"the pad measures the joined word", h + `x=${(l:3::_:)$(h)}; printf "[%s]" "$x"`, "[1 2]"},
		{"and a wider one leaves it alone", h + `x=${(l:9::_:)$(h)}; printf "[%s]" "$x"`, "[____3 1 2]"},

		// And from above: the operator has already run on the elements, so
		// the join is rule 10's and not the quoted join at rule 5. The
		// element-selecting operators are the ones that can say so without a
		// dialect's answer for what a bare array name comes to — see
		// dialect/zsh for the trims, which need it.
		{"an element replacement runs elementwise", `z=(x y); x=${(@)z:/x/Q}; printf "[%s]" "$x"`, "[Q y]"},
		{"and so does a rejection", `q=(one two); x=${(o)q:#one}; printf "[%s]" "$x"`, "[two]"},
		{"and what they leave is joined on the group's separator", `z=(x y); x=${(j:+:)z:/x/Q}; printf "[%s]" "$x"`, "[Q+y]"},

		// A length is asked of the words ahead of the join, exactly as it is
		// in quotes: `${(U)#a}` is the element count there and here.
		{"a length still counts the elements", `a=(abc de f); x=${(U)#a}; printf "[%s]" "$x"`, "[3]"},

		// The `@` letter keeps fields in quotes and cannot here: a scalar
		// context has nowhere to put a second one.
		{"the @ letter does not exempt the join", `y=(c a b); x=${(o@)y}; printf "[%s]" "$x"`, "[c a b]"},
		{"quoted in the same context, the same", `y=(c a b); x="${(o@)y}"; printf "[%s]" "$x"`, "[c a b]"},
		{"a hole is kept, being part of the join", `z=(a "" b); x=${(@)z}; printf "[%s]" "$x"`, "[a  b]"},

		// The other three contexts, which is what says this is the context
		// and not the assignment.
		{"a case subject", `v=c,a,b; case ${(s:,:)v} in "c,a,b") printf "[unsplit]";; *) printf "[%s]" "${(s:,:)v}";; esac`, "[unsplit]"},
		{"a [[ ]] operand", `v=c,a,b; [[ ${(s:,:)v} == "c,a,b" ]] && printf "[unsplit]"`, "[unsplit]"},
		{"a here-string", `v=c,a,b; cat <<< ${(s:,:)v}`, "c,a,b\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, scalarJoinGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The splitting flags do not split in these contexts either, which is the
// rule the `=` spelling has always followed and which the letters now follow
// with it.
//
// The discriminating pair is the second and third rows: a split the *quoting*
// turned off would answer one field in the third, and a split nothing turns
// off would answer three in the first.
func TestASplitFlagDoesNotSplitInAScalarContext(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a separator split", `v=c,a,b; x=${(s:,:)v}; printf "[%s]" "$x"`, "[c,a,b]"},
		{"where a list context splits", `v=c,a,b; printf "[%s]" ${(s:,:)v}`, "[c][a][b]"},
		{"and quoting is not what decides it", `v=c,a,b; printf "[%s]" "${(s:,:)v}"`, "[c][a][b]"},
		{"a newline split", "w=$'c\\na\\nb'; x=${(f)w}; printf \"[%s]\" \"$x\"", "[c\na\nb]"},
		{"the @ letter does not turn it back on", `v=c,a,b; x=${(@s:,:)v}; printf "[%s]" "$x"`, "[c,a,b]"},
		{"an array is joined and then not split", `y=("c,a" b); x=${(s:,:)y}; printf "[%s]" "$x"`, "[c,a b]"},
		{"where a list context joins and splits", `y=("c,a" b); printf "[%s]" ${(s:,:)y}`, "[c][a b]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, scalarJoinGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The shell split is the one step below the join that can make a list out of
// one word, so the join is repeated after it — with the first character of
// IFS and not with the separator the group named, which is the last row and
// is what says the two joins are different joins.
func TestTheShellSplitIsJoinedBackInAScalarContext(t *testing.T) {
	const u = `u='b a'; `
	for _, tc := range []struct{ name, src, want string }{
		{"the ordering finds one word", u + `x=${(oZ+n+)u}; printf "[%s]" "$x"`, "[b a]"},
		{"and so does the pad", u + `x=${(l:3::_:Z+n+)u}; printf "[%s]" "$x"`, "[b a]"},
		{"where a list context sorts the words it made", u + `printf "[%s]" ${(oZ+n+)u}`, "[a][b]"},
		{"the join is on IFS", u + `IFS=:; x=${(Z+n+)u}; printf "[%s]" "$x"`, "[b:a]"},
		{"even where the group named a separator", u + `x=${(j:+:Z+n+)u}; printf "[%s]" "$x"`, "[b a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, scalarJoinGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The inner of a nesting arrives with the same splitting policy and is not
// one of these contexts: what it comes to is read by the operator around it
// rather than by a command line, so its fields are values and not words.
func TestANestedInnerIsNotAScalarContext(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a hole survives to the join", `a=(one "" two); printf "[%s]" "${(j:,:)${(@)${a[@]}}}"`, "[one,,two]"},
		{"and to a count", `a=(one "" two); printf "[%s]" "${#${(@)${a[@]}}}"`, "[3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, scalarJoinGrammar, nil)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// scalarJoinGrammar is the grammar these expansions need, named by the
// constructs rather than by a shell.
func scalarJoinGrammar(d *syntax.Dialect) {
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
	d.ArraySubscript = true
	d.NestedParamExpansion = true
	d.ParamElementSelection = true
	d.ParamWholeElementReplace = true
	d.DoubleBracket = true
	d.Herestring = true
	d.DollarSingleQuote = true
}
