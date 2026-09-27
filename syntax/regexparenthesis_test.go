// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A `=~` operand's parentheses are the regular expression's in every dialect
// that has the operator, and one flag takes them back off it — after which the
// operand is read by the word rules every other word is read by. See
// [Dialect.RegexParenthesisIsTheShellsOwn].

// regexOwnsItsParens is the grammar with `=~` and the operand keeping its own
// parentheses, which is what every dialect here does today.
func regexOwnsItsParens() Dialect {
	d := Core()
	d.DoubleBracket = true
	return d
}

// regexParensAreTheShells hands the parentheses back to the word rules, which
// in this fixture have no bare group at all.
func regexParensAreTheShells() Dialect {
	d := regexOwnsItsParens()
	d.RegexParenthesisIsTheShellsOwn = true
	return d
}

// regexParensAreTheShellsInsideAWord is the same with the narrowed bare-group
// reading in place, which is the third state and the one that says the flag
// hands the question over rather than answering it.
func regexParensAreTheShellsInsideAWord() Dialect {
	d := regexParensAreTheShells()
	d.BarePatternGroupInsideAWord = true
	return d
}

func TestARegexOperandsParenthesesFollowTheWordRulesWhenToldTo(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		// refused with the operand owning its parentheses, with them handed
		// to a grammar with no bare group, and with them handed to one whose
		// bare group opens inside a word.
		want [3]bool
	}{
		{"a group where the operand begins", "[[ a =~ (a) ]]\n", [3]bool{false, true, true}},
		{"a group inside the operand", "[[ abc =~ ^(a|x)bc$ ]]\n", [3]bool{false, true, false}},
		{"nested groups inside the operand", "[[ abc =~ ^((a)(b))c$ ]]\n", [3]bool{false, true, false}},
		{"two groups where the operand begins", "[[ abcd =~ (b)(c) ]]\n", [3]bool{false, true, true}},
		// The controls. An operand with no bare parenthesis in it, and the
		// two quotings, are untouched in every state — so a flag that had
		// broken the operand rather than handed it over would show here.
		{"no parenthesis at all", "[[ abc =~ ^a.c$ ]]\n", [3]bool{false, false, false}},
		{"a quoted group", "[[ abc =~ \"^(a)\" ]]\n", [3]bool{false, false, false}},
		{"an escaped parenthesis", "[[ abc =~ a\\(b ]]\n", [3]bool{false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, d := range []Dialect{
				regexOwnsItsParens(), regexParensAreTheShells(),
				regexParensAreTheShellsInsideAWord(),
			} {
				_, err := Parse(tc.src, d)
				if got := err != nil; got != tc.want[i] {
					t.Errorf("dialect %d: refused=%v (%v), want %v", i, got, err, tc.want[i])
				}
			}
		})
	}
}
