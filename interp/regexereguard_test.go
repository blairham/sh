// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math/rand"
	"strings"
	"testing"
)

// The fast path in asERE is a guard on a set of characters, and a guard that
// is wrong there is **silent**: the rewrite it gates does not run, the pattern
// reaches the engine as the script wrote it, and the answer is a match that
// should not have happened or a refusal that should not have been needed.
//
// It has been wrong four times. `[` was left out when the bracket constructs
// were added, `\` when the escape rule was, the repetition operators when the
// repeated repetition was, and the quoting mark when that arrived — the last
// sent NUL bytes to the engine and turned every quoted operand into a pattern
// that matched nothing, which `cond.tests` caught as seventeen differing lines
// where it had had three.
//
// So the guard is held to what it gates rather than to a restatement of
// itself: rewriteERE is the same rewrite with the guard taken off, and the
// property is that the fast path may only be taken where running the rewrite
// would have changed nothing.
func TestTheFastPathIsOnlyTakenWhenTheRewriteWouldDoNothing(t *testing.T) {
	check := func(t *testing.T, pat string) {
		t.Helper()
		if strings.ContainsAny(pat, ereRewriteTriggers) {
			// The slow path is taken; nothing is being skipped.
			return
		}
		got, err := rewriteERE(pat)
		if err != nil {
			t.Errorf("%q: the guard skipped a pattern the rewrite refuses: %v", pat, err)
			return
		}
		if got != pat {
			t.Errorf("%q: the guard skipped a rewrite that would have written %q —\n"+
				"a character that reaches the rewrite is missing from ereRewriteTriggers", pat, got)
		}
	}

	// One row per construct the rewrite has to *reach*, and whether reaching
	// it also changes the text. The ones that do not are not decoration: a
	// `[` has to be entered so that a `*` written inside it is a member
	// rather than a repetition, and an interval has to be read so that the
	// operator behind it is seen as one — a guard that skipped them would
	// hand the engine a pattern read the wrong way round.
	for _, tc := range []struct {
		name, pat string
		rewrites  bool
	}{
		{"a bracket expression", "[ab]", false},
		{"a negated set", "[^a]", false},
		{"a character class", "[[:alpha:]]", false},
		{"an escape", `a\.b`, false},
		{"an interval", "a{2}", false},
		{"a collating element", "[[.a.]]", true},
		{"an equivalence class", "[[=a=]]", true},
		{"a repeated repetition", "a**", true},
		{"a repetition behind a group", "(a)*?", true},
		{"an open-ended interval", "a{,2}", true},
		{"a quoted character", "a" + string(rune(condRegexMark)) + ".b", true},
		{"a quoted character inside a set", "[" + string(rune(condRegexMark)) + "]a]", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.ContainsAny(tc.pat, ereRewriteTriggers) {
				t.Fatalf("%q: the guard's set does not admit this construct at all", tc.pat)
			}
			got, err := rewriteERE(tc.pat)
			switch {
			case tc.rewrites && err == nil && got == tc.pat:
				t.Errorf("%q: the rewrite left it alone, so this row proves nothing", tc.pat)
			case !tc.rewrites && err == nil && got != tc.pat:
				t.Errorf("%q: the rewrite wrote %q — the row says it only reads it", tc.pat, got)
			}
		})
	}

	// Every byte on its own, and in the three positions a rewrite reads:
	// nothing outside the guard's set may reach the rewrite and come back
	// changed.
	t.Run("every byte", func(t *testing.T) {
		for b := 0; b < 256; b++ {
			c := string(rune(b))
			for _, pat := range []string{c, "a" + c, c + "a", "a" + c + "b"} {
				check(t, pat)
			}
		}
	})

	// And generated patterns, because a character that only matters beside
	// another one would pass every row above.
	t.Run("generated", func(t *testing.T) {
		alphabet := []string{
			"a", "b", "0", ".", "^", "$", "|", "(", ")", "-", ":", "=", "!", "]",
			"[", `\`, "*", "+", "?", "{", "}", ",", "2", string(rune(condRegexMark)),
		}
		rng := rand.New(rand.NewSource(20260923))
		for i := 0; i < 4000; i++ {
			var b strings.Builder
			for n := rng.Intn(6) + 1; n > 0; n-- {
				b.WriteString(alphabet[rng.Intn(len(alphabet))])
			}
			check(t, b.String())
		}
	})
}
