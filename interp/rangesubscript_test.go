// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A substring's range is arithmetic, so a subscript inside one is read the way
// a subscript inside `$(( ))` is: a bracket that came out of a quotation or
// out of a value is a character of the *key*, not the end of the brackets, and
// a `$` that came out of a value begins no expansion.
//
// The joined word answers none of that — quote removal takes the apostrophes
// off before any reader sees them, and an expansion's own `]` or `$(` arrives
// as the same byte the script would have written. Tested by axis and never by
// shell: Semantics.ArithSubscriptRereadsItsExpandedText is what decides
// whether the marking is honored, and it is the same answer every other
// arithmetic site takes.

// rangeSubGrammar is what these probes need: a subscript, a declaration
// utility to make the keyed table with, the quoting a subscript may carry, and
// the `${x:off:len}` the range belongs to.
func rangeSubGrammar(d *syntax.Dialect) {
	d.ArraySubscript = true
	d.ArrayLiteral = true
	d.DeclarationUtilities = map[string]bool{"typeset": true}
	d.ArithSubscriptQuoting = true
	d.ParamSubstring = true
}

// rangeSubAxis answers the axis under test and the two neighbors that would
// otherwise decide the rows for a reason that is not it.
func rangeSubAxis(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		arraySemantics(&s)
		s.ArithSubscriptRereadsItsExpandedText = a
		s.SubscriptIsAQuotingContext = Yes
		s.ArrivedSubscriptIsAQuotingContext = Yes
		r.Semantics = &s
	}
}

// rangeSubTable is the table every probe below reads from: one element under
// the one-character key `]`, and one under a key that looks like a command
// substitution and is not.
const rangeSubTable = `typeset -A a; a[']']=5; a['$(echo 9)']=3; ` +
	`k=']'; kk='$(echo 9)'; s=abcdefghij; `

func TestASubstringRangeReadsASubscriptAsArithmeticNotAsAJoinedWord(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The apostrophes are the script's, so the key is the one
			// character between them and the brackets close where the script
			// closed them.
			"a quotation's bracket is a character of the key",
			`printf '[%s]' "${s:0:a[']']}"`,
			"[abcde]",
		},
		{
			// The `]` arrived out of a value, and an arrived bracket does not
			// close a subscript the script opened.
			"a value's bracket is a character of the key",
			`printf '[%s]' "${s:0:a[$k]}"`,
			"[abcde]",
		},
		{
			// The `$(` arrived out of a value too. Running it would be a
			// second round of expansion, and the count is the assertion: the
			// key is the text and nothing is executed.
			"a value's dollar begins no substitution",
			`printf '[%s]' "${s:0:a[$kk]}"`,
			"[abc]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runGrammar(t, rangeSubTable+c.src, rangeSubGrammar, rangeSubAxis(No))
			if out != c.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
			}
		})
	}
}

// The other answer, which is what makes the rows above an axis rather than a
// rule: a dialect that hands a subscript's expanded text back to the bracket
// scanner reads the value's `]` as the closing bracket and finds no element.
func TestASubstringRangeRereadsAnExpandedSubscriptByAxis(t *testing.T) {
	const src = rangeSubTable + `printf '[%s]' "${s:0:a[$k]}"`
	out, _ := runGrammar(t, src, rangeSubGrammar, rangeSubAxis(Yes))
	if out == "[abcde]" {
		t.Errorf("out %q, want the element missed where the expanded text is read again", out)
	}
}
