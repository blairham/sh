// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// countedSurface runs one snippet under the grammar that has counts, with
// the brace balancing a quoted pattern operand needs.
func countedSurface(t *testing.T, src string) string {
	t.Helper()
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.CountedPatternGroup = true
		d.DoubleBracket = true
		d.ParamSubstitution = true
		d.BareBraceNestsInExpansion = true
		d.BareBraceNestsInAQuotedPatternOperand = true
		// The flavor-group control below needs the grammar that lets a `(`
		// in behind a `~`; nothing else here reads it.
		d.TildeGroup = true
	}, nil)
	return strings.TrimSpace(out)
}

// The repetition operator belongs to the **pattern language** and not to one
// reader: a `case` arm, `[[ … ]]` and the three parameter-expansion pattern
// operators each get it.
//
// Measured 2026-09-27 against `/bin/ksh` `Version AJM 93u+ 2012-08-01` —
// `go version -m` says *not a Go executable* — each probe from `-c` under
// `env -i PATH=/usr/bin:/bin`.
func TestACountedGroupReachesEverySurface(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a case arm", `case aaa in {2,3}(a)) echo matches;; *) echo no;; esac`, `matches`},

		// Three is inside the count, **one is too few and four too many** —
		// the two rows that make it a count rather than a repetition. A row
		// that only asked whether `aaa` matched would pass for `+(a)`.
		{"a condition", `[[ aaa == {2,3}(a) ]] && echo matches || echo no`, `matches`},
		{"two", `[[ aa == {2,3}(a) ]] && echo matches || echo no`, `matches`},
		{"one is too few", `[[ a == {2,3}(a) ]] && echo matches || echo no`, `no`},
		{"four is too many", `[[ aaaa == {2,3}(a) ]] && echo matches || echo no`, `no`},

		// Mid-pattern, so it is not a prefix on the word.
		{"mid-pattern", `[[ aaa == a{1,2}(a) ]] && echo matches || echo no`, `matches`},
		{"a group of two characters", `[[ abab == {2}(ab) ]] && echo matches || echo no`, `matches`},
		{"and an alternation in one", `[[ abab == {2}(a|ab|b) ]] && echo matches || echo no`, `matches`},

		// The three parameter-expansion operators. The shortest and the
		// longest are separate rows because the count bounds the search
		// rather than replacing it.
		{"a shortest prefix trim", `s=aaaX; echo "[${s#{2,3}(a)}]"`, `[aX]`},
		{"a longest one", `s=aaaX; echo "[${s##{2,3}(a)}]"`, `[X]`},
		{"a shortest suffix trim", `s=aaaX; echo "[${s%{1,2}(a)X}]"`, `[aa]`},
		{"a longest one too", `s=aaaX; echo "[${s%%{1,2}(a)X}]"`, `[a]`},
		{"a replacement", `s=abababX; echo "[${s//{2}(ab)/-}]"`, `[-abX]`},
		{"a single replacement", `s=abababX; echo "[${s/{2}(ab)/-}]"`, `[-abX]`},

		// A count that cannot be spent trims nothing, and one with no floor
		// still finds the longest.
		{"a floor of nought trims nothing", `s=aaaX; echo "[${s#{0,0}(a)}]"`, `[aaaX]`},
		{"and no ceiling finds the longest", `s=aaaX; echo "[${s##{,}(a)}]"`, `[X]`},

		// The count is read where it stands in an operand too.
		{"mid-operand", `s=xaaaX; echo "[${s#x{2,3}(a)}]"`, `[aX]`},
		{"and a literal in front that is absent", `s=aaaX; echo "[${s#x{2,3}(a)}]"`, `[aaaX]`},

		// A flavor group does not turn the count into an extended-regular-
		// expression repetition, and this control already agreed before the
		// operator existed.
		{"a flavor group is not this", `[[ aaa == ~(E){2,3}(a) ]] && echo matches || echo no`, `no`},
	} {
		if got := countedSurface(t, tc.src); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// **A bare `(` behind a brace that is not a count is a group in an operand
// and a character in a word**, and the same three characters answer the two
// surfaces differently.
//
// This is the pair that keeps the word rule from being applied everywhere.
// A written bare group at the top of a pattern is a syntax error in a word
// and in a condition in this dialect — `[[ ab == a(b) ]]` — so a count is
// the only door a parenthesis has there; in an operand it has one of its
// own, which `s=ab; ${s#a(b)}` being empty says with no brace in it at all.
func TestABareGroupBehindACountIsTheSurfacesAnswer(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The operand: the group is read, so the pattern matches `{z,y}a`.
		{"an operand reads the group", `s='{z,y}a'; echo "[${s#{z,y}(a)}]"`, `[]`},
		{"and not its own text", `s='{z,y}(a)'; echo "[${s#{z,y}(a)}]"`, `[{z,y}(a)]`},
		{"with a tail behind it", `s='{z,y}ab'; echo "[${s#{z,y}(a)}]"`, `[b]`},
		// And it needs no brace to say so.
		{"no brace at all", `s=ab; echo "[${s#a(b)}]"`, `[]`},
		{"nor to leave one alone", `s='a(b)'; echo "[${s#a(b)}]"`, `[a(b)]`},

		// The condition: the same text, and the parentheses are characters.
		{"a condition reads the text", `[[ '{z,y}(a)' == {z,y}(a) ]] && echo matches || echo no`, `matches`},
		{"and not the group", `[[ '{z,y}a' == {z,y}(a) ]] && echo matches || echo no`, `no`},
	} {
		if got := countedSurface(t, tc.src); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}
