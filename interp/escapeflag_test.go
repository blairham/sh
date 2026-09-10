// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// withTestExpansionEscapes installs an escape reader of two letters, one of
// which the flag's option letters turn on.
//
// Two letters and not the measured forty, for the reason withTestEscapes
// carries two: what this package answers is *when* the reader runs, over what
// text, and with which letters — not what any shell's set contains, which is
// asserted where it was measured, in that shell's own package. A reader
// carrying the real alphabet would let a test pass on the alphabet while the
// position rule was wrong.
//
// `\t` is read always and `\T` only under the `e` option, so a row can tell
// an option letter that arrived from one that did not. The text it writes for
// `\T` is a marker rather than a tab: a reader that lost the letter and fell
// back to the unconditional branch would be invisible if both wrote the same
// thing.
func withTestExpansionEscapes(r *interp.Runner) {
	r.SetExpansionEscapes(func(text, opts string) string {
		var b strings.Builder
		for i := 0; i < len(text); i++ {
			if text[i] != '\\' || i+1 == len(text) {
				b.WriteByte(text[i])
				continue
			}
			i++
			switch {
			case text[i] == 't':
				b.WriteByte('\t')
			case text[i] == 'T' && strings.ContainsRune(opts, 'e'):
				b.WriteString("<E>")
			default:
				b.WriteByte('\\')
				b.WriteByte(text[i])
			}
		}
		return b.String()
	})
}

// The escape flag reads the escapes in the value, and its argument says which
// parts of the set are live.
func TestTheEscapeFlagReadsTheValuesEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an empty argument still reads escapes",
			`v='a\tb'; printf "[%s]" "${(g::)v}"`, "[a\tb]",
		},
		{
			"a letter in the argument turns its part on",
			`v='a\Tb'; printf "[%s]" "${(g:e:)v}"`, "[a<E>b]",
		},
		{
			"and without the letter that part is text",
			`v='a\Tb'; printf "[%s]" "${(g::)v}"`, `[a\Tb]`,
		},
		{
			"a value with no escape in it is the value",
			`v=plain; printf "[%s]" "${(g::)v}"`, "[plain]",
		},
		{
			"the letters of two arguments union",
			`v='a\T\tb'; printf "[%s]" "${(g::g:e:)v}"`, "[a<E>\tb]",
		},
		{
			"in either order",
			`v='a\T\tb'; printf "[%s]" "${(g:e:g::)v}"`, "[a<E>\tb]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestExpansionEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Where the reading stands among the other flags: after the case conversion,
// before the quoting, and over each of the words the pipeline holds.
//
// The case rows are the discriminating pair, and they are a pair on purpose.
// Lowering `A\TB` produces an escape the reader can use and raising `a\tb`
// destroys one, so a reader that ran first answers both rows the other way
// round — and neither order of the letters in the group changes the answer,
// which is the third row of each pair.
func TestTheEscapeFlagRunsWhereTheRulesPutIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the case conversion runs first, making an escape",
			`v='A\TB'; printf "[%s]" "${(Lg::)v}"`, "[a\tb]",
		},
		{
			"and destroying one",
			`v='a\tb'; printf "[%s]" "${(Ug::)v}"`, `[A\TB]`,
		},
		{
			"whichever side of the group it is written on",
			`v='a\tb'; printf "[%s]" "${(g::U)v}"`, `[A\TB]`,
		},
		{
			"the quoting runs after",
			`v='a\tb'; printf "[%s]" "${(qg::)v}"`, `[a$'\t'b]`,
		},
		{
			"and the unquoting before",
			`v="'a\tb'"; printf "[%s]" "${(Qg::)v}"`, "[a\tb]",
		},
		{
			"each word of a list is read",
			`a=('x\ty' 'p\tq'); printf "[%s]" ${(g::)a}`, "[x\ty][p\tq]",
		},
		{
			"and a separator the join inserted is read with them",
			`b=(m n); printf "[%s]" "${(g::j:\t:)b}"`, "[m\tn]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestExpansionEscapes)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A runner nobody handed an escape reader refuses the flag by name.
//
// This is the half that would rot into a stub, and it is a sharper case than
// the argument flag's: a `(g)` read as a no-op is *right* for every value with
// no backslash in it, so it would pass the first thing anyone tried and answer
// at status 0 wherever it mattered.
func TestTheEscapeFlagIsRefusedWithoutAnEscapeReader(t *testing.T) {
	out, st := runGrammar(t, `v='a\tb'; printf "[%s]" "${(g::)v}"`, escapingFlags, nil)
	want := "sh: ${(g::)v}: the (g) expansion flag is not implemented\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the refusal to be fatal")
	}
}

// The argument's letters are the grammar's, so one the flag does not have is
// an error in the flags at its own position rather than a refusal when the
// expansion is reached. Asserted with a reader installed, because the two
// failures are answered in different places and only this order proves the
// parser is the one answering.
func TestAnEscapeOptionLetterTheFlagLacksIsAFlagsError(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a letter outside the set",
			`v=x; printf "[%s]" "${(g:x:)v}"`,
			"sh: error in flags near position 6 in '${(g:x:)v}'\n",
		},
		{
			"reported at its own position among letters that are in it",
			`v=x; printf "[%s]" "${(g:oex:)v}"`,
			"sh: error in flags near position 8 in '${(g:oex:)v}'\n",
		},
		{
			"and the flag with no argument at all points at what arrived",
			`v=x; printf "[%s]" "${(g)v}"`,
			"sh: error in flags near position 5 in '${(g)v}'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, escapingFlags, withTestExpansionEscapes)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the flags error to be fatal")
			}
		})
	}
}

// The marked-separator flag is refused beside this one, which is rule 13's
// half of a rule the `(%)` flag already has: the reading rewrites the joined
// text, and a separator held out of the escape would be read along with
// everything else. Asserted with a reader installed, so the refusal that
// fires is this one rather than the by-name refusal of an absent reader.
func TestTheMarkedSeparatorIsRefusedBesideTheEscapeFlag(t *testing.T) {
	out, st := runGrammar(t, `a=(p q); printf "[%s]" "${(~g::j.|.)a}"`,
		markedSeparatorWithEscapes, withTestExpansionEscapes)
	want := "sh: ${(~g::j.|.)a}: the (~) expansion flag is not implemented beside the (g) flag\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st == 0 {
		t.Errorf("status 0, want the refusal to be fatal")
	}
}

// markedSeparatorWithEscapes is escapingFlags plus the written `~` the marked
// separator needs a slot for.
func markedSeparatorWithEscapes(d *syntax.Dialect) {
	escapingFlags(d)
	d.ParamTildeFlag = true
}
