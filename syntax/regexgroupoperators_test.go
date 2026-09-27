// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A regular expression's group keeps the shell's operators in four of the
// five shells that have `=~`, and one dialect stops the group at them. See
// [Dialect.RegexGroupEndsAtAShellOperator], where the rows are.

func regexGroupKeepsOperators() Dialect {
	d := Core()
	d.DoubleBracket = true
	return d
}

func regexGroupStopsAtOperators() Dialect {
	d := regexGroupKeepsOperators()
	d.RegexGroupEndsAtAShellOperator = true
	return d
}

func TestARegexGroupStopsAtAShellOperatorWhereTheFlagSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		stopped bool
	}{
		{"a redirection operator", "[[ \"a<b\" =~ (a<b) ]]\n", true},
		{"a terminator", "[[ \"a;b\" =~ (a;b) ]]\n", true},
		{"the other redirection operator", "[[ \"a>b\" =~ (a>b) ]]\n", true},
		{"a background operator", "[[ \"a&b\" =~ (a&b) ]]\n", true},
		// The two exceptions, and they are the whole reason the set is four
		// characters rather than "an operator": a blank and a `|` are the
		// group's text in that shell as they are in a pattern group.
		{"a blank", "[[ \"a b\" =~ (a b) ]]\n", false},
		{"an alternation bar", "[[ \"a|b\" =~ (a|b) ]]\n", false},
		// And the controls: a group with none of them, and one that nests.
		{"no operator at all", "[[ abc =~ (b) ]]\n", false},
		{"nested groups", "[[ abc =~ ^((a)(b))c$ ]]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(tc.src, regexGroupKeepsOperators()); err != nil {
				t.Errorf("the dialect that keeps its operators refused the line: %v", err)
			}
			_, err := Parse(tc.src, regexGroupStopsAtOperators())
			if got := err != nil; got != tc.stopped {
				t.Errorf("the dialect that stops at one: refused=%v (%v), want %v", got, err, tc.stopped)
			}
		})
	}
}

// Outside a group the question does not arise, which is what keeps the flag
// about the group: the word ends at the operator in both dialects.
func TestAnOperatorOutsideARegexGroupEndsTheWordEitherWay(t *testing.T) {
	const src = "[[ \"a<b\" =~ a<b ]]\n"
	for i, d := range []Dialect{regexGroupKeepsOperators(), regexGroupStopsAtOperators()} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("dialect %d parsed the line, so the `<` was taken as regex text", i)
		}
	}
}
