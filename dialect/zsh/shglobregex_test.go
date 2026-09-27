// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// A `=~` operand's parentheses are the regular expression's here, and
// `shglob` takes them back off it so that the two operands of `[[ ]]` split
// the same way.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, run `-f` over a script file under `set -n`, with
// the options moved on the line before, 2026-09-27. Every `want` is the
// reference's answer.
var regexParenRows = []struct {
	name                  string
	src                   string
	neither, shGlob, both bool
}{
	{"a group where the operand begins", "[[ a =~ (a) ]]\n", false, true, true},
	{"a group inside the operand", "[[ abc =~ ^(a|x)bc$ ]]\n", false, true, false},
	{"nested groups inside the operand", "[[ abc =~ ^((a)(b))c$ ]]\n", false, true, false},
	{"two groups where the operand begins", "[[ abcd =~ (b)(c) ]]\n", false, true, true},
	// The controls, untouched in every state.
	{"no parenthesis at all", "[[ abc =~ ^a.c$ ]]\n", false, false, false},
	{"a quoted group", "[[ abc =~ \"^(a)\" ]]\n", false, false, false},
	{"an escaped parenthesis", "[[ abc =~ a\\(b ]]\n", false, false, false},
}

// TestTheOptionPairDecidesARegexOperandsParentheses is the row that says the
// regular expression's operand is read by the same rules as the pattern's.
//
// The second row is the discriminator again: with both names on, a group
// where the operand *begins* is still refused while one inside it is taken,
// which is the split a pattern operand has in the same state.
func TestTheOptionPairDecidesARegexOperandsParentheses(t *testing.T) {
	for _, row := range regexParenRows {
		t.Run(row.name, func(t *testing.T) {
			for _, state := range []struct {
				name            string
				shGlob, kshGlob bool
				want            bool
			}{
				{"neither", false, false, row.neither},
				{"shglob", true, false, row.shGlob},
				{"shglob and kshglob", true, true, row.both},
				{"kshglob alone", false, true, row.neither},
			} {
				t.Run(state.name, func(t *testing.T) {
					r := caseListRunner(t)
					if code := setOption(r, "shglob", state.shGlob); code != 0 {
						t.Fatalf("setting shglob answered %d", code)
					}
					if code := setOption(r, "kshglob", state.kshGlob); code != 0 {
						t.Fatalf("setting kshglob answered %d", code)
					}
					if got := !parsesHere(t, r, row.src); got != state.want {
						t.Errorf("refused=%v, want %v", got, state.want)
					}
				})
			}
		})
	}
}

// And through the emulations, which is the route #4818's cases take.
func TestAnEmulationReachesARegexOperandsParentheses(t *testing.T) {
	for _, tc := range []struct {
		mode            string
		shGlob, kshGlob bool
	}{
		{"zsh", false, false},
		{"sh", true, false},
		{"ksh", true, true},
		{"csh", false, false},
	} {
		t.Run("emulate "+tc.mode, func(t *testing.T) {
			r := caseListRunner(t)
			applyEmulation(r, tc.mode, false)
			for _, row := range regexParenRows {
				want := row.neither
				switch {
				case tc.shGlob && tc.kshGlob:
					want = row.both
				case tc.shGlob:
					want = row.shGlob
				}
				if got := !parsesHere(t, r, row.src); got != want {
					t.Errorf("%s: refused=%v, want %v", row.name, got, want)
				}
			}
		})
	}
}
