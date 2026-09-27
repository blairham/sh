// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// The escapes an **extended** `~(…)` flavor keeps, through the shell rather
// than through tildeKeepsBackslash's table.
//
// The two are not the same test: the table says which span keeps its
// backslash and this says that the backslash reaches an engine that reads it,
// which is the half a table cannot show. Every row is ksh93u+ 2012-08-01's,
// measured 2026-09-27, and nothing here names a shell — the construct is a
// grammar flag and the escapes are that construct's.
//
// **Each letter is a pair**, and that is the whole design of the table. A
// class reading and a literal reading agree on half of all subjects, so a row
// asserting only that `~(E)za\db` matches `za1b` would pass for a shell that
// read `\d` as the letter `d` and happened to be handed a subject with a `d`
// in it. The second row of each pair is the subject that separates them.
func TestAnExtendedTildeFlavorKeepsTheExpressionsEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The control, and it is the same question written with a bracket
		// expression: it matched before this and after it, so a row below
		// answering YES is the escape being read rather than the flavor
		// being reached at all.
		{"a bracket says the engine runs", `[[ za1b == ~(P)za[0-9]b ]] && echo YES || echo NO`, "YES"},

		{"a digit class", `[[ za1b == ~(P)za\db ]] && echo YES || echo NO`, "YES"},
		{"and not the letter", `[[ zadb == ~(P)za\db ]] && echo YES || echo NO`, "NO"},
		{"its negation", `[[ zaXb == ~(P)za\Db ]] && echo YES || echo NO`, "YES"},
		{"and not the letter either", `[[ za1b == ~(P)za\Db ]] && echo YES || echo NO`, "NO"},

		{"a word class", `[[ za1b == ~(P)za\wb ]] && echo YES || echo NO`, "YES"},
		{"which a period is not", `[[ za.b == ~(P)za\wb ]] && echo YES || echo NO`, "NO"},
		{"its negation", `[[ za.b == ~(P)za\Wb ]] && echo YES || echo NO`, "YES"},
		{"which a digit is not", `[[ za1b == ~(P)za\Wb ]] && echo YES || echo NO`, "NO"},

		{"a space class", `[[ "za b" == ~(P)za\sb ]] && echo YES || echo NO`, "YES"},
		{"which a letter is not", `[[ zasb == ~(P)za\sb ]] && echo YES || echo NO`, "NO"},
		{"its negation", `[[ za1b == ~(P)za\Sb ]] && echo YES || echo NO`, "YES"},
		{"which a space is not", `[[ "za b" == ~(P)za\Sb ]] && echo YES || echo NO`, "NO"},

		// **The same escapes in `E` and `X`**, which is what says this is
		// the extended family rather than the one letter #4894 named.
		{"the ERE flavor has them too", `[[ za1b == ~(E)za\db ]] && echo YES || echo NO`, "YES"},
		{"and not as the letter", `[[ zadb == ~(E)za\db ]] && echo YES || echo NO`, "NO"},
		{"the augmented one as well", `[[ za1b == ~(X)za\wb ]] && echo YES || echo NO`, "YES"},
		{"and not as the letter", `[[ za.b == ~(X)za\wb ]] && echo YES || echo NO`, "NO"},

		// **A basic flavor has none of them**, measured rather than assumed,
		// and its own control says the flavor is reached.
		{"a basic flavor has no class", `[[ za1b == ~(G)za\db ]] && echo YES || echo NO`, "NO"},
		{"and reads the letter instead", `[[ zadb == ~(G)za\db ]] && echo YES || echo NO`, "YES"},
		{"with a bracket as its control", `[[ za1b == ~(G)za[0-9]b ]] && echo YES || echo NO`, "YES"},

		// The control characters, each with the letter it is not.
		{"a tab", "[[ $'x\\ty' == ~(E)x\\ty ]] && echo YES || echo NO", "YES"},
		{"and not a `t`", `[[ xty == ~(E)x\ty ]] && echo YES || echo NO`, "NO"},
		{"a newline", "[[ $'x\\ny' == ~(E)x\\ny ]] && echo YES || echo NO", "YES"},
		{"and not an `n`", `[[ xny == ~(E)x\ny ]] && echo YES || echo NO`, "NO"},
		{"a return", "[[ $'x\\ry' == ~(E)x\\ry ]] && echo YES || echo NO", "YES"},
		{"a form feed", "[[ $'x\\fy' == ~(E)x\\fy ]] && echo YES || echo NO", "YES"},
		{"a vertical tab", "[[ $'x\\vy' == ~(E)x\\vy ]] && echo YES || echo NO", "YES"},
		{"a bell", "[[ $'x\\ay' == ~(E)x\\ay ]] && echo YES || echo NO", "YES"},
		{"and not an `a`", `[[ xay == ~(E)x\ay ]] && echo YES || echo NO`, "NO"},

		// The boundaries and the anchors.
		{"a word boundary", `[[ xy == ~(E)\bxy ]] && echo YES || echo NO`, "YES"},
		{"where there is none", `[[ axy == ~(E)a\bxy ]] && echo YES || echo NO`, "NO"},
		{"the absence of one", `[[ xy == ~(E)x\By ]] && echo YES || echo NO`, "YES"},
		{"where there is one", `[[ "x y" == ~(E)x\B ]] && echo YES || echo NO`, "NO"},
		{"the two text anchors", `[[ xy == ~(E)\Axy\z ]] && echo YES || echo NO`, "YES"},
		{"which really anchor", `[[ axy == ~(E)\Axy ]] && echo YES || echo NO`, "NO"},
		{"and are not their letters", `[[ xAy == ~(E)x\Ay ]] && echo YES || echo NO`, "NO"},

		// **A letter the two engines read differently keeps the reading it
		// had**, which is a stated divergence rather than a gap. `\Z` is an
		// end anchor in that shell and the engine here has `\z` only, so it
		// stays the letter the shell quoted — and **both** halves of that
		// pair differ from ksh93u+, which is what makes it a divergence
		// rather than a narrower reading: there the first row is yes and the
		// second is no.
		{"a letter left out anchors nothing", `[[ xy == ~(E)xy\Z ]] && echo YES || echo NO`, "NO"},
		{"and is the letter it quoted", `[[ xyZ == ~(E)xy\Z ]] && echo YES || echo NO`, "YES"},

		// **And the group need not be at the head of the word.** That is
		// #4894's third row: the flavor decides whose backslash it is, and
		// the flavor is read where it stands.
		{"a flavor group one along", `[[ za1b == z~(P)a\db ]] && echo YES || echo NO`, "YES"},
		{"and not as the letter", `[[ zadb == z~(P)a\db ]] && echo YES || echo NO`, "NO"},
		{"behind a fold group", `[[ zA1b == ~(i)z~(P)a\db ]] && echo YES || echo NO`, "YES"},

		// A group naming **no** flavor leaves the backslash to the shell, so
		// the escape is the character it quotes — the rows that say the
		// reading is keyed on the flavor rather than on there being a group
		// at all. Both are ksh93u+'s answers, and `~(i)` is the group here
		// rather than `~(K)`: `K` reads `\d` as a digit class there even
		// though it names that shell's own glob, which `p`, `s`, `i` and an
		// empty group do not, and is its own row rather than this one's.
		{"a fold group keeps no escape", `[[ zadb == z~(i)a\db ]] && echo YES || echo NO`, "YES"},
		{"and the class does not fire", `[[ za1b == z~(i)a\db ]] && echo YES || echo NO`, "NO"},
		{"nor in a pattern with no group", `[[ zadb == za\db ]] && echo YES || echo NO`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
