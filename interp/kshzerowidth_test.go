// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// A `~(K)` group makes `\b` a **word boundary**, `\B` its complement and
// `\z` the end — three escapes that consume no text at all.
//
// A third family beside the six classes and the eight control characters,
// and a third shape: a class names a set and takes a unit, a control escape
// names one character and takes one byte, and one of these takes nothing and
// asks about the *position*. The rows are ksh93u+ 2012-08-01's, measured
// 2026-09-28 from a script file under `env -i PATH=/usr/bin:/bin` with a
// scratch `HOME`.
//
// **Each letter is a pair.** An assertion that held everywhere and one that
// held nowhere would each agree with half of these, so every letter has a
// subject it holds at and one it does not.
func TestATildeGlobGroupReadsTheZeroWidthEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The control that makes the `\b` rows evidence: a backspace
		// subject really is one matchable character wide, so a row where
		// `\b` declines it is the escape declining and not the subject
		// being unreachable.
		{"a backspace is one character", `[[ $'za\bb' == z~(K)a?b ]] && echo YES || echo NO`, "YES"},

		// `\b` — a boundary is a word character on exactly one side.
		{"a boundary at the end", `[[ zab == z~(K)ab\b ]] && echo YES || echo NO`, "YES"},
		{"one before a space", `[[ 'za b' == z~(K)a\b?b ]] && echo YES || echo NO`, "YES"},
		{"one before a period", `[[ 'za.b' == z~(K)a\b.b ]] && echo YES || echo NO`, "YES"},
		{"none between two letters", `[[ zab == z~(K)a\bb ]] && echo YES || echo NO`, "NO"},
		{"and it is not the letter b", `[[ zabb == z~(K)a\bb ]] && echo YES || echo NO`, "NO"},
		{"nor a backspace", `[[ $'za\bb' == z~(K)a\bb ]] && echo YES || echo NO`, "NO"},

		// `\B` — the same subject `zab` answered oppositely, which is what
		// says the two are complements rather than two spellings of
		// "matches nothing".
		{"a non-boundary between two letters", `[[ zab == z~(K)a\Bb ]] && echo YES || echo NO`, "YES"},
		{"and it is not the letter B", `[[ zaBb == z~(K)a\Bb ]] && echo YES || echo NO`, "NO"},
		{"nor any other letter", `[[ zaXb == z~(K)a\Bb ]] && echo YES || echo NO`, "NO"},

		// `\z` — the end, and not the letter.
		{"not the letter z", `[[ zazb == z~(K)a\zb ]] && echo YES || echo NO`, "NO"},
		{"the end of the subject", `[[ ab == ~(K)ab\z ]] && echo YES || echo NO`, "YES"},
		{"which a trailing z is not", `[[ abz == ~(K)ab\z ]] && echo YES || echo NO`, "NO"},
		{"nor is the front", `[[ ab == ~(K)\zab ]] && echo YES || echo NO`, "NO"},
		{"and not the middle", `[[ ab == ~(K)a\zb ]] && echo YES || echo NO`, "NO"},
		{"an empty subject is all end", `[[ '' == ~(K)\z ]] && echo YES || echo NO`, "YES"},

		// The control for a letter **neither** family names, which says the
		// backslash is not being swallowed wholesale.
		{"a letter no family names", `[[ zaqb == z~(K)a\qb ]] && echo YES || echo NO`, "YES"},

		// **The group is read where it stands**, so text in front of it is
		// outside it: the boundary here is between `z` and `a`, both word
		// characters, and not at the front of anything.
		{"the front of the group is not the front", `[[ zab == z~(K)\bab ]] && echo YES || echo NO`, "NO"},
		{"and its complement holds there", `[[ zab == z~(K)\Bab ]] && echo YES || echo NO`, "YES"},

		// **The front of the piece is a boundary whatever stands at it**,
		// and the back is not. A period is a non-word character on both
		// sides of the front position, so a rule that counted the absent
		// character as a non-word one would answer the first of these NO —
		// and it is the rule the *back* follows, which the third and fourth
		// rows are the pair for.
		{"the front of the subject", `[[ '.' == ~(K)\b. ]] && echo YES || echo NO`, "YES"},
		{"so the complement declines it", `[[ '.' == ~(K)\B. ]] && echo YES || echo NO`, "NO"},
		{"the back after a period is not", `[[ '.' == ~(K).\b ]] && echo YES || echo NO`, "NO"},
		{"the back after a letter is", `[[ a == ~(K)a\b ]] && echo YES || echo NO`, "YES"},
		{"an empty subject is all front", `[[ '' == ~(K)\b ]] && echo YES || echo NO`, "YES"},
		{"and the complement finds none", `[[ '' == ~(K)\B ]] && echo YES || echo NO`, "NO"},

		// Beside the rest of the glob.
		{"behind a wildcard", `[[ ab == ~(K)*\z ]] && echo YES || echo NO`, "YES"},
		{"two in a row", `[[ ab == ~(K)ab\z\z ]] && echo YES || echo NO`, "YES"},
		{"and two that disagree", `[[ ab == ~(K)a\b\Bb ]] && echo YES || echo NO`, "NO"},

		// With no group the escapes are the letters, which is what says the
		// rows above are the group's doing.
		{"no group, a\\b is the letter", `[[ abb == a\bb ]] && echo YES || echo NO`, "YES"},
		{"no group, so not a boundary", `[[ ab == a\bb ]] && echo YES || echo NO`, "NO"},
		{"no group, \\z is the letter", `[[ abz == ab\z ]] && echo YES || echo NO`, "YES"},

		// And the two families beside them are untouched.
		{"a class still reads", `[[ za1b == z~(K)a\db ]] && echo YES || echo NO`, "YES"},
		{"and not as a letter", `[[ zadb == z~(K)a\db ]] && echo YES || echo NO`, "NO"},
		{"a control escape still reads", `[[ $'za\nb' == z~(K)a\nb ]] && echo YES || echo NO`, "YES"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// A zero-width assertion asks about the **piece** a surface handed over and
// not about the whole value, and a suffix trim is the one surface that can
// tell the two apart: `${v%…}` is the only span-choosing operator that reads
// a `~(K)` group at all — see tildeClassesHere for the surfaces that decline
// it, which the last row here holds in place.
//
// Measured 2026-09-28 on ksh93u+ 2012-08-01. A subject-relative reading gets
// every one of the first six wrong: it would find no boundary at offset 1 of
// `aab`, where the trim finds one because that is where the piece begins.
func TestAZeroWidthEscapeAsksAboutThePiece(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the piece's front is a boundary", `v=aab; echo "[${v%~(K)\bab}]"`, "[a]"},
		{"so the complement declines it", `v=aab; echo "[${v%~(K)\Bab}]"`, "[aab]"},
		{"one character over", `v=ab; echo "[${v%~(K)\bb}]"`, "[a]"},
		{"and its complement there", `v=ab; echo "[${v%~(K)\Bb}]"`, "[ab]"},
		// The sharpest pair: the piece begins at a period *and* has a period
		// in front of it in the subject, so neither a subject-relative
		// reading nor one counting the absent character as a non-word one
		// would trim.
		{"whatever stands at that front", `v=..b; echo "[${v%~(K)\b.b}]"`, "[.]"},
		{"which leaves the complement none", `v=..b; echo "[${v%~(K)\B.b}]"`, "[..b]"},
		// Inside the piece the two readings agree, which is what makes the
		// rows above about the edge rather than about the escape.
		{"inside the piece they agree", `v=ab; echo "[${v%~(K)a\bb}]"`, "[ab]"},
		{"as does the complement", `v=ab; echo "[${v%~(K)a\Bb}]"`, "[]"},
		{"the end anchor at the end", `v=ab; echo "[${v%~(K)b\z}]"`, "[a]"},
		{"and not in the middle", `v=aab; echo "[${v%~(K)a\zb}]"`, "[aab]"},
		// The controls: a trim with no group is the letter, and a prefix
		// trim does not read the group at all.
		{"no group is the letter", `v=aab; echo "[${v%ab}]"`, "[a]"},
		{"and a prefix trim declines the group", `v=abc; echo "[${v#~(K)\bab}]"`, "[abc]"},
		{"where it reads the letters", `v=abc; echo "[${v#ab}]"`, "[c]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// **The two sides of a boundary are read differently**, and the locale is
// what says so. The character *at* the position is one unit, exactly as a
// class escape takes one; the one *behind* it is read as a single **byte**,
// so the trailing byte of a multi-byte character is not a word character
// even in a locale where the character is.
//
// Each row is run under `en_US.UTF-8`, where `é` is one unit, and again
// under the byte-unit locale the rest of the file runs in — which is the
// only way to say this at all. A mutation that reads the *ahead* side one
// byte at a time is invisible where a unit is a byte, so a row without an
// explicit locale is a row that cannot fail.
//
// The last pair is what fixes the *behind* side: reading it as a character
// would make `é` a word character under UTF-8 and answer both of them the
// other way, while leaving the first two rows right — so varying only the
// locale and only looking ahead would confirm the wrong rule.
func TestAWordBoundaryReadsOneUnitAheadAndOneByteBehind(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a two-byte letter ahead is one word character",
			`LC_ALL=en_US.UTF-8; [[ 'aéb' == ~(K)a\béb ]] && echo YES || echo NO`, "NO",
		},
		{
			"so the complement holds there",
			`LC_ALL=en_US.UTF-8; [[ 'aéb' == ~(K)a\Béb ]] && echo YES || echo NO`, "YES",
		},
		{"where a unit is a byte it is not", `[[ 'aéb' == ~(K)a\béb ]] && echo YES || echo NO`, "YES"},
		{"and the complement declines", `[[ 'aéb' == ~(K)a\Béb ]] && echo YES || echo NO`, "NO"},
		{
			"but behind it is a byte in both",
			`LC_ALL=en_US.UTF-8; [[ 'aéb' == ~(K)aé\bb ]] && echo YES || echo NO`, "YES",
		},
		{"here too", `[[ 'aéb' == ~(K)aé\bb ]] && echo YES || echo NO`, "YES"},
		{
			"which the end row says again",
			`LC_ALL=en_US.UTF-8; [[ 'é' == ~(K)é\B ]] && echo YES || echo NO`, "YES",
		},
		{"and again where a unit is a byte", `[[ 'é' == ~(K)é\B ]] && echo YES || echo NO`, "YES"},
		{
			"one letter each side, both sides read",
			`LC_ALL=en_US.UTF-8; [[ 'éé' == ~(K)é\bé ]] && echo YES || echo NO`, "YES",
		},
		{"which a byte ahead answers NO", `[[ 'éé' == ~(K)é\bé ]] && echo YES || echo NO`, "NO"},
		{
			"a period behind a letter ahead",
			`LC_ALL=en_US.UTF-8; [[ '.é' == ~(K).\bé ]] && echo YES || echo NO`, "YES",
		},
		{"and not where the letter is two", `[[ '.é' == ~(K).\bé ]] && echo YES || echo NO`, "NO"},
		{
			"a three-byte letter behaves the same",
			`LC_ALL=en_US.UTF-8; [[ 'a日' == ~(K)a\B日 ]] && echo YES || echo NO`, "YES",
		},
		{
			"and its trailing byte is not a word one",
			`LC_ALL=en_US.UTF-8; [[ '日b' == ~(K)日\bb ]] && echo YES || echo NO`, "YES",
		},
		// The control on the width, which says the locale reached the
		// matcher at all rather than the rows agreeing by accident.
		{"the wildcard as the control", `LC_ALL=en_US.UTF-8; [[ 'aé' == ~(K)a? ]] && echo YES || echo NO`, "YES"},
		{"two bytes without it", `[[ 'aé' == ~(K)a? ]] && echo YES || echo NO`, "NO"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tildeMid(t, tc.src, nil); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}
