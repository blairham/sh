// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `case` arm's parenthesized pattern list is one alternation word in this
// dialect, so a newline in it is a character of the pattern.
//
// The substrate's tests name the grammar flag; these name the shell, because
// it is the only one that takes the line at all — every other shell in the
// panel calls it a syntax error. Measured 2026-09-07 on zsh 5.9.2 with a
// scratch HOME and ZDOTDIR.
//
// The rows are subjects rather than "it parsed", because what the newline
// joined is the part parsing cannot say.
func TestACasePatternListSpansANewlineInThisDialect(t *testing.T) {
	dir := t.TempDir()
	arm := func(subject string) string {
		return "case " + subject + " in (a|\nb) printf m;; *) printf .;; esac"
	}
	for _, tc := range []struct{ name, src, want string }{
		{"the first alternative", arm(`a`), "m"},
		{"and not the second on its own", arm(`b`), "."},
		{"nor the empty string", arm(`""`), "."},
		{"but a value that begins with a newline", arm(`"$(printf '\nb')"`), "m"},
		// The newline before the separator joins the *first* alternative,
		// which is what says the rule is not about the `|`.
		{
			"a newline before the separator",
			"case a in (a\n|b) printf m;; *) printf .;; esac",
			".",
		},
		{
			"and with no separator in the list at all",
			"case a in (a\n) printf m;; *) printf .;; esac",
			".",
		},
		{
			// Through `$'…'` rather than a command substitution, which
			// strips the trailing newline and would measure the stripping
			// instead — zsh answers `.` for that spelling, and it is the
			// wrong probe rather than a different answer.
			"which the subject holding it does match",
			"x=$'a\\n'; case \"$x\" in (a\n) printf m;; *) printf .;; esac",
			"m",
		},
		// The arm's body gets its newlines back at the closing paren.
		{
			"the body is statements again",
			"case a in (a|\nb)\nprintf o\nprintf k\n;; esac",
			"ok",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
