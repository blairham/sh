// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `unset 'a[(r)y]'` — a subscript flag group on the third side of the
// construct, where the subscript arrives as a **runtime string** rather than
// as a word the parser lexed.
//
// That is why it was carved out of the change that answered the same group on
// the left of an assignment (#1102): the group could be scanned off the text,
// but its operand had never been lexed and so had nowhere to hang, and the
// whole of `(r)y` went to the arithmetic as `bad math expression`. It is read
// as the reference it is now — one parse, from which the group, its operand
// and the plain reading all come (#1275).
//
// What is left behind is the dialect's answer and not this construct's, so
// every row runs with the span policy that leaves one empty element; all this
// decides is *which* element.

// flaggedUnsetRun answers the base, the subscript-on-unset reading and the
// span policy, and adds the one grammar this construct needs beyond the
// range's: the flag group inside a subscript. Named for the constructs.
func flaggedUnsetRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArraySubscript = true
		d.ArrayLiteral = true
		d.ArraySubscriptFlags = true
	}, func(r *Runner) {
		sem := *r.Semantics
		sem.UnsetTakesASubscript = Yes
		sem.ArrayBaseIsZero = No
		sem.UnsetArraySpan = UnsetArraySpanLeavesOneEmptyElement
		sem.ScalarSubscriptIsACharacter = Yes
		sem.GlobExpansionResults = No
		sem.GlobNoMatchIsError = Yes
		r.Semantics = &sem
	})
}

const showB = `; printf "[%s]" "${b[@]}"; echo " n=${#b[@]}"`

// Every row is a measurement on zsh 5.9.2, the one shell with the construct,
// 2026-09-12.
func TestUnsettingThroughASubscriptFlagGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the element whose value the search matched",
			`b=(x y z); unset 'b[(r)y]'`, "[x][][z] n=3\n",
		},
		{
			"exact matching selects the same one",
			`b=(x y z); unset 'b[(re)y]'`, "[x][][z] n=3\n",
		},
		{
			"and the operand is a pattern",
			`b=(x y z); unset 'b[(r)y*]'`, "[x][][z] n=3\n",
		},
		{
			"the index form names the same element",
			`b=(x y z); unset 'b[(i)y]'`, "[x][][z] n=3\n",
		},
		{
			"a reverse search takes the last match",
			`b=(x y x); unset 'b[(R)x]'`, "[x][y][] n=3\n",
		},
		{
			// One past the last element, which no element has, so the array
			// is untouched — the same index that makes `b[(r)new]=v` an
			// append on the other side.
			"a missed search removes nothing",
			`b=(x y z); unset 'b[(r)nomatch]'`, "[x][y][z] n=3\n",
		},
		{
			"and neither does its index form",
			`b=(x y z); unset 'b[(i)nomatch]'`, "[x][y][z] n=3\n",
		},
		{
			"a group with no selecting flag is an ordinary subscript",
			`b=(x y z); unset 'b[(e)2]'`, "[x][][z] n=3\n",
		},
		{
			// The operand had never been lexed, which is the whole of what
			// this construct was missing: the substitution is performed
			// when the reference is read.
			"the operand is expanded",
			`b=(x y z); w=y; unset 'b[(r)$w]'`, "[x][][z] n=3\n",
		},
		{
			"the modifiers are read too",
			`b=(x y z); unset 'b[(rn:1:)y]'`, "[x][][z] n=3\n",
		},
		{
			"and two operands each name their own element",
			`b=(x y z); unset 'b[(r)y]' 'b[(r)z]'`, "[x][][] n=3\n",
		},
		{
			"a plain subscript beside them is unchanged",
			`b=(x y z); unset 'b[2]'`, "[x][][z] n=3\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := flaggedUnsetRun(t, tc.src+showB)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// A search over a plain string names a *character* position, and the same
// store that answers `unset 's[3]'` removes it.
func TestUnsettingThroughASearchOverAString(t *testing.T) {
	out, st := flaggedUnsetRun(t, `s=hello; unset 's[(r)l]'; printf "[%s]" "$s"; echo`)
	if out != "[helo]\n" || st != 0 {
		t.Errorf("got %q (status %d), want [helo] at 0", out, st)
	}
}

// The two sides part company over what a refusal does to the line. An
// assignment's is fatal, because nothing was written and the line must not
// report the status of whatever came before it; `unset` writes one complaint
// per operand and goes on to the rest, which is the shape it already has for
// a subscript that will not evaluate — and is what the shell does, measured.
func TestARefusedSubscriptFlagLeavesUnsetRunningOn(t *testing.T) {
	out, st := flaggedUnsetRun(t, `b=(x y z); unset 'b[(w)y]'; echo ran`+showB)
	if !strings.Contains(out, "the (w) subscript flag is not implemented") {
		t.Errorf("got %q, want the flag refused by name", out)
	}
	if !strings.Contains(out, "ran\n[x][y][z] n=3\n") {
		t.Errorf("got %q, want the builtin to report and the script to run on", out)
	}
	if st != 0 {
		t.Errorf("status %d, want the following command's 0", st)
	}
}

// An association is answered before the group is reached and is measured to
// want that: the brackets are a key there, so nothing matches and nothing
// goes.
func TestASubscriptFlagGroupOverATableRemovesNothing(t *testing.T) {
	out, st := flaggedUnsetRun(t, `typeset -A m=(a 1 b 2); unset 'm[(r)2]'; printf "[%s]" "${m[a]}" "${m[b]}"; echo`)
	if out != "[1][2]\n" || st != 0 {
		t.Errorf("got %q (status %d), want the table untouched at 0", out, st)
	}
}
