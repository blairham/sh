// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A closure over a POSIX character class, run through the real dialect.
//
// Here rather than in interp/ deliberately. A test inside interp can only be
// handed a synthetic Semantics, so it can pass against a table that is wrong
// — which happened on #1386, where five tests passed while the shipped
// dialect was broken. These rows go through zsh.Semantics() and zsh.Dialect(),
// which is the same table the binary runs.
//
// Measured on zsh 5.9.2, 2026-09-07, `env -i` with a scratch HOME and
// HISTFILE. The fault: the closure's item ended at the class's own `]`, so
// `[[:space:]]##` was an unterminated bracket followed by `]##` — nothing read
// the closure, the bracket matched exactly one character, and the `##` behind
// it repeated a literal `#` (#1409).
func TestAClosureOverACharacterClassRepeatsTheWholeBracket(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The longest-match trims, which are where the extent of a single
		// match is visible. Before the fix these were `[ x  ]` and `[  x ]`
		// — one space each, not the run.
		{
			"a prefix trim takes the whole run",
			`setopt extendedglob; v="  x  "; echo "[${v##[[:space:]]##}]"`,
			"[x  ]",
		},
		{
			"a suffix trim takes the whole run",
			`setopt extendedglob; v="  x  "; echo "[${v%%[[:space:]]##}]"`,
			"[  x]",
		},
		{
			"a class closure can take the whole value",
			`setopt extendedglob; v=abX; echo "[${v##[[:alpha:]]##}]"`,
			"[]",
		},
		// A single `#` is zero-or-more, and it was the same fault seen from
		// the other side: with nothing after it to be a closure, the `#`
		// stayed a literal and the pattern became "one alpha, then a `#`",
		// which matched nothing at all.
		{
			"a zero-or-more closure over a class",
			`setopt extendedglob; v=abX; echo "[${v##[[:alpha:]]#}]"`,
			"[]",
		},
		// The count spelling never touches the `#` scan, so it says the
		// fault was in what the item was and not in how `#` was read.
		{
			"a counted closure over a class",
			`setopt extendedglob; v=abX; echo "[${v##[[:alpha:]](#c2)}]"`,
			"[X]",
		},
		// A negated class, so the `^` and the `[:` are read in the same
		// bracket.
		{
			"a closure over a negated class",
			`setopt extendedglob; v=abX; echo "[${v##[^[:digit:]]##}]"`,
			"[]",
		},
		// Inside a group, which is the route the wild expression takes.
		{
			"a class closure inside a group",
			`setopt extendedglob; v=abX; echo "[${v##([[:alpha:]]##)}]"`,
			"[]",
		},
		// An anchored *single* replacement, where the match happens once and
		// its length is therefore observable.
		{
			"an anchored replacement of a class closure",
			`setopt extendedglob; v=abX; echo "[${v/#[[:alpha:]]##/Z}]"`,
			"[Z]",
		},
		// `(M)` yields the match rather than the remainder, so it reports
		// the extent directly. This is the shape #1409 was filed on.
		{
			"the match flag reports the whole extent",
			`setopt extendedglob; v="  a  "; echo "[${(M)v##(#s)[[:space:]]##}]"`,
			"[  ]",
		},
		// The plugin manager's own formatter, which is what made this a
		// daily-driver bug: both anchors, an alternation, a group opening
		// the pattern (#1408) and two class closures.
		{
			"a real formatter trims both ends",
			`setopt extendedglob; v="  a  "; echo "[${v//((#s)[[:space:]]##|[[:space:]]##(#e))/}]"`,
			"[a]",
		},
		// The controls: a range, an enumeration and a `?` have no inner `]`
		// and always quantified correctly. They are here so that a fix that
		// worked by breaking them would be caught.
		{
			"a range still quantifies",
			`setopt extendedglob; v=abX; echo "[${v##[a-z]##}]"`,
			"[X]",
		},
		{
			"an enumeration still quantifies",
			`setopt extendedglob; v=abX; echo "[${v##[ab]##}]"`,
			"[X]",
		},
		{
			"a wildcard still quantifies",
			`setopt extendedglob; v=abX; echo "[${v##?##}]"`,
			"[]",
		},
		// A bracket with no closure behind it is unchanged: one character.
		{
			"a class with no closure still matches one",
			`setopt extendedglob; v="  x"; echo "[${v#[[:space:]]}]"`,
			"[ x]",
		},
		// `[=a=]` and `[.a.]` are deliberately **not** stepped over, because
		// that shell does not read them either — it ends the bracket at the
		// first `]` and the rest is text. Measured: `aab` there, and `aab`
		// here, so this row is agreement rather than a gap.
		{
			"an equivalence class is not a class",
			`setopt extendedglob; v=aab; echo "[${v##[[=a=]]##}]"`,
			"[aab]",
		},
		// With the option off the same characters are literal, which is what
		// keeps the closure from being read where no shell reads one.
		{
			"no closure without the option",
			`v="  x  "; echo "[${v##[[:space:]]##}]"`,
			"[  x  ]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s\n  = %q (status %d), want %q", tc.src, out, st, tc.want+"\n")
			}
		})
	}
}

// The two spellings that **cannot** see the bug, kept as a warning rather
// than as coverage.
//
// A global substitution re-applies its pattern until nothing matches, so a
// closure matching one character at a time still removes the whole run; and
// the *shortest* match of one-or-more is one character, which is exactly what
// the broken closure produced. Both of these agreed with real zsh throughout
// #1409, with the bug fully present.
//
// They are asserted so that the claim is checkable and so that nobody reads
// them as evidence: a test written on either one would have recorded
// agreement and pinned the bug in place.
func TestTwoSpellingsCannotSeeAClassClosuresExtent(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a global substitution re-applies until nothing matches",
			`setopt extendedglob; v="  x  "; echo "[${v//[[:space:]]##/}]"`,
			"[x]",
		},
		{
			"the shortest match of one-or-more is one character",
			`setopt extendedglob; v=12ab; echo "[${v#[[:digit:]]##}]"`,
			"[2ab]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s\n  = %q (status %d), want %q", tc.src, out, st, tc.want+"\n")
			}
		})
	}
}

// A group opening a pattern operand, run through the real dialect: the other
// half of the daily-driver failure, and the one that was fatal.
//
// The cause is in syntax/ — an operand was lexed where a command may begin,
// so `((#s)a)` was read as an arithmetic command and came back as the
// expression `#s)a` — and syntax/operandnotacommand_test.go asserts the text
// at that layer. These rows are the same fault from the surface a script
// sees: a refusal with status 1 that stopped the script (#1408).
func TestAPatternOperandMayBeginWithAGroup(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an anchor first in a group",
			`setopt extendedglob; v=aXb; echo "[${v#((#s)a)}]"`,
			"[Xb]",
		},
		{
			"an anchor and an alternation in a group",
			`setopt extendedglob; v=aXb; echo "[${v//((#s)a|b)/}]"`,
			"[X]",
		},
		// **This row could not tell the readings apart in this dialect**,
		// and is recorded as a guard rather than as evidence: the
		// arithmetic reading left the pattern `a`, which trims `aXb` to
		// `Xb` — the same answer the group gives. It passed with the bug
		// fully present. The column where it discriminates is bash, where
		// `((a))` is five ordinary characters and the value is left alone;
		// that is pinned by the corpus row pat/a-group-opening-a-pattern-
		// operand and by the text assertion in syntax/.
		{
			"a group in a group, no flags at all",
			`setopt extendedglob; v=aXb; echo "[${v#((a))}]"`,
			"[Xb]",
		},
		// The rows the issue recorded as already working, so a fix that
		// moved them would be caught: an anchor *not* first, and the
		// condition route, which never went through the operand lexer.
		{
			"an anchor last in a group",
			`setopt extendedglob; v=aXb; echo "[${v//(a|b(#e))/}]"`,
			"[X]",
		},
		{
			"the condition route was never affected",
			`setopt extendedglob; v=aXb; [[ $v == ((#s)a)* ]] && echo hit || echo miss`,
			"hit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want+"\n" || st != 0 {
				t.Errorf("%s\n  = %q (status %d), want %q", tc.src, out, st, tc.want+"\n")
			}
		})
	}
}
