// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnUnclosedBraceInAHeredocBodyIsTheCommandsFailure pins how an
// unterminated `${` in a here-document body is refused: at the command's
// line, `bad substitution` where the expansion stopped before an operator and
// `closing brace expected` where it was reading an operand, and the script
// goes on at 1. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5379).
func TestAnUnclosedBraceInAHeredocBodyIsTheCommandsFailure(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"${x", "zsh:2: bad substitution\nst=1\n"},
		{"a ${x b\nc", "zsh:2: bad substitution\nst=1\n"},
		{"${#", "zsh:2: bad substitution\nst=1\n"},
		{"${", "zsh:2: bad substitution\nst=1\n"},
		{"pre\na ${x:-y\nc", "zsh:2: closing brace expected\nst=1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), "echo pre\ncat <<E\n"+tc.body+"\nE\necho st=$?\n")
		if got != "pre\n"+tc.want {
			t.Errorf("body %q\n got %q\nwant %q", tc.body, got, "pre\n"+tc.want)
		}
	}
}

// TestAnExpansionStoppedBeforeAnOperatorIsABadSubstitution pins the split the
// `(e)` flag makes on a `${` that never closed, by what stood after the name.
// Measured 2026-10-02 on zsh 5.9.2: an operator is `closing brace expected`
// and anything else is `bad substitution`.
func TestAnExpansionStoppedBeforeAnOperatorIsABadSubstitution(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"${x b", "bad substitution"},
		{"${x.", "bad substitution"},
		{"${x;", "bad substitution"},
		{"${x:-b", "closing brace expected"},
		{"${x/a", "closing brace expected"},
		{"${x%", "closing brace expected"},
	} {
		got, _ := runZsh(t, t.TempDir(), "v='"+tc.text+"'; print -r -- ${(e)v}")
		if want := "zsh:1: " + tc.want + "\n"; got != want {
			t.Errorf("%s: got %q, want %q", tc.text, got, want)
		}
	}
}
