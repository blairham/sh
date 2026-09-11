// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Every site that reads a `\u` escape refuses a code point the locale's
// encoding cannot hold, not only `echo`.
//
// Measured 2026-09-11 on zsh 5.9.2 under `LC_ALL=C`, bytes read with `od`, the
// subject `a\u00e9Z` throughout. Each of these wrote the UTF-8 here before (#2021),
// which is the plausible-wrong-answer shape: right in a person's terminal, and
// wrong in the C locale a conformance run and a CI job sit in.
//
// The status is what separates the two groups, and it is measured rather than
// reasoned: a builtin's refusal leaves **0** and an expansion's leaves **1**,
// both abandoning the script where they stand.
func TestEverySiteRefusesACodePointOutsideTheLocale(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// `print` writes what came before the escape and the terminator it
		// would have written, exactly as `echo` does.
		{"print", `LC_ALL=C; print -- 'a\u00e9Z'; print AFTER`, refusalLine + "a\n", 0},
		// Once however many escapes the operands hold.
		{"print, two escapes", `LC_ALL=C; print -- 'a\u00e9b\u00e9Z'`, refusalLine + "a\n", 0},
		// A `printf` format, which stops without the newline it ends with.
		{"a printf format", `LC_ALL=C; printf 'a\u00e9Z\n'; print AFTER`, refusalLine + "a", 0},
		{"a printf %b argument", `LC_ALL=C; printf '%b' 'a\u00e9Z'; print AFTER`, refusalLine + "a", 0},
		// The two expansion sites, where nothing is written at all because
		// the word the value was part of never finished.
		{"the (g) flag", `LC_ALL=C; v='a\u00e9Z'; printf '%s' ${(g::)v}; print AFTER`, refusalLine, 1},
		{"the (p) flag", `LC_ALL=C; a=(x y); printf '%s' ${(pj:A\u00e9B:)a}; print AFTER`, refusalLine, 1},
		{"a $'...' word", `LC_ALL=C; x=$'a\u00e9Z'; print AFTER`, refusalLine, 1},
		// `print -r` reads no escapes at all and is unaffected, which is the
		// control: without it every row above would pass for a shell that
		// refused the *word* rather than the escape.
		{"print -r reads no escapes", `LC_ALL=C; print -r -- 'a\u00e9Z'`, "a" + `\u00e9` + "Z\n", 0},
		// And in a UTF-8 locale every site writes the character, which is
		// where a person's terminal is.
		{"a UTF-8 locale writes it", `LC_ALL=en_US.UTF-8; print -- 'a\u00e9Z'`, "a\u00e9Z\n", 0},
		{"and so does the (g) flag", `LC_ALL=en_US.UTF-8; v='a\u00e9Z'; printf '%s' ${(g::)v}`, "a\u00e9Z", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = % x (status %d), want % x at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The complaint is located as the **shell** and not as the builtin, at every
// one of those sites — `zsh:1: character not in range` — which is the tell
// that it is a fact about reading a word.
const refusalLine = "zsh:1: character not in range\n"
