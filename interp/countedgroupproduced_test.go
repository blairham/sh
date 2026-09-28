// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// countedProduced is countedWords with the three axes that decide what a
// **produced** parenthesis is pinned to the column these rows were measured
// in, rather than to the column that keeps a produced one text.
//
// `Semantics.BraceFreesProducedGroupSyntax` is the one that matters and it
// is the other end of this rule: a brace suppressed in front of a
// parenthesis that then went literal would be a third answer neither column
// gives, so the two have to be set together and a test that pinned only one
// would be measuring half a rule.
func countedProduced(t *testing.T, dir, src string) string {
	t.Helper()
	out, _ := runGrammar(t, emptyAltCounter+src+"\n", func(d *syntax.Dialect) {
		d.ExtendedPattern = true
		d.ExtendedPatternInCondition = true
		d.CountedPatternGroup = true
	}, func(r *Runner) {
		r.Dir = dir
		s := *r.Semantics
		s.BraceExpansion = Yes
		s.BraceStopsFieldSplitting = Yes
		s.BraceEmptyAlternativeIsAField = Yes
		s.BraceRescanEntersFailedGroup = No
		s.BraceOutputRereadAsText = Yes
		s.BraceScanReadsProducedText = Yes
		s.BraceBodyReadAfterExpansion = Yes
		s.BraceFanExpandsEachNameOnItsOwn = No
		s.BraceRangeEndpointsExpanded = Yes
		s.BraceFreesProducedGroupSyntax = Yes
		s.BraceMakesAProducedStarOrBracketText = Yes
		s.ExpansionResultSuppliesGroupSyntax = No
		r.Semantics = &s
	})
	return strings.TrimSpace(out)
}

// A `(` an expansion produces makes a written brace a count.
//
// Measured 2026-09-27 against `/bin/ksh` `Version AJM 93u+ 2012-08-01` —
// `go version -m` says *not a Go executable* — in #4927's directory, each
// case under `env -i PATH=/usr/bin:/bin`.
//
// **The first two rows are the whole of it**: the same written `{2,3}`,
// expanded in one and suppressed in the other, and the only difference is
// what `$g` holds. Brace expansion runs before parameter expansion, so
// nothing about the word the parse cut distinguishes them.
func TestAProducedParenthesisMakesAWrittenBraceACount(t *testing.T) {
	dir := countedFixture(t)
	for _, tc := range []struct{ name, src, want string }{
		{"the control", `f {z,y}a`, `2 | [za] [ya]`},

		{"a produced letter", `g=x; f {2,3}$g`, `2 | [2x] [3x]`},
		{"a produced group", `g='(a)'; f {2,3}$g`, `2 | [aa] [aaa]`},
		// Not a variable-only path: a command substitution does it too, and
		// it is run **once** — the road that finds a word's braces in the
		// fields it expanded to is what makes that true without a second
		// look at the word.
		{"from a substitution", `f {2,3}$(echo '(a)')`, `2 | [aa] [aaa]`},
		{"from a positional", `g='(a)'; set -- "$g"; f {2,3}$1`, `2 | [aa] [aaa]`},
		{"across an empty run", `g='(a)'; e=; f {2,3}$e$g`, `2 | [aa] [aaa]`},
		{"in a braced expansion", `g='(a)'; f {2,3}${g}`, `2 | [aa] [aaa]`},

		// #4933's rule survives it: a produced `(` behind a brace whose
		// contents are not a count leaves the word entire, exactly as the
		// written spelling does.
		{"behind a count that is not one", `g='(a)'; f {z,y}$g`, `1 | [{z,y}(a)]`},

		// The controls that say what it is **not**.
		{"a produced brace is a list", `g='{2,3}(a)'; f $g`, `2 | [2(a)] [3(a)]`},
		{"a quoted expansion produces no `(`", `g='(a)'; f {2,3}"$g"`, `2 | [2(a)] [3(a)]`},
		{"the `(` must be the next character", `g='x(a)'; f {2,3}$g`, `2 | [2x(a)] [3x(a)]`},
		{"and the brace must be written", `g='(a)'; f "{2,3}"$g`, `1 | [{2,3}(a)]`},

		// And it binds to the one group, as the written spelling does: a
		// brace behind it is a list again.
		{"a brace behind the group", `g='(a)'; f {2,3}$g{x,y}`, `2 | [{2,3}(a)x] [{2,3}(a)y]`},

		// A produced `(` that opens nothing still suppresses the brace —
		// the test is the character and not a group that closes.
		{"an unclosed produced group", `g='(a'; f {2,3}$g`, `1 | [{2,3}(a]`},
		{"and text behind a closed one", `g='(a)x'; f {2,3}$g`, `1 | [{2,3}(a)x]`},

		// Produced text reaches three places and only the parenthesis
		// behind the `}` is this rule's. These two were already right and
		// are kept because each guesses wrong from the rows above.
		{"produced contents of a written brace", `g='a,b'; f {$g}`, `2 | [a] [b]`},
		{"produced contents of a written count", `g=',3'; f {2$g}(a)`, `2 | [aa] [aaa]`},
		{"a produced group body is read as one", `g='a,b'; f {2,3}($g)`, `1 | [{2,3}(a,b)]`},
	} {
		if got := countedProduced(t, dir, tc.src); got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// The substitution behind the brace runs **once**.
//
// This is the row that says nothing here takes a second look at the word: a
// reading that expanded the suffix to decide and then expanded the word
// again would run the command twice, and the reference runs it once.
func TestDecidingTheBraceRunsTheSubstitutionOnce(t *testing.T) {
	dir := countedFixture(t)
	got := countedProduced(t, dir, `f {2,3}$(printf 'x' >&2; echo '(a)')`)
	if want := "x2 | [aa] [aaa]"; got != want {
		t.Errorf("= %q, want %q — one `x` is one run", got, want)
	}
}
