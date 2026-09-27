// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The other side of Semantics.EmptyBracketExpressionCompiles: a bracket the
// member reading cannot close stays **unterminated** here, so `[]` and `[!]`
// are the words themselves rather than an empty set and its negation (#4644).
//
// Measured 2026-09-26 with `--norc --noprofile -c`, in a directory holding
// `a`, `z`, `~`, `]` and `a]`, against bash 5.3.20 (`/opt/homebrew/bin/bash`)
// and bash 3.2.57 (`/bin/bash`) — both builds answer every row alike, which
// is worth having because 5.3 is the shell that moved on the neighboring
// bracket question.
//
// **The first three rows are the ones that would break if the rule were read
// as "a `]` written first closes the bracket"**: they are ordinary sets here,
// and in every other column including the one that parts from us below.
func TestAnEmptyBracketStaysUnterminatedHere(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "z", "~", "]", "a]"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ name, src, want string }{
		{"a one-member set", `echo []]`, "]\n"},
		{"a two-member set", `echo []a]`, "] a\n"},
		{"a negated set holding it", `echo [!]]`, "a z ~\n"},

		// And the rows that part: unterminated, so the word is itself.
		{"the empty bracket", `echo []`, "[]\n"},
		{"with a literal after it", `echo []a`, "[]a\n"},
		{"the negated empty bracket", `echo [!]`, "[!]\n"},
		{
			// A prefix trim shows the same thing from the other side: the
			// unterminated bracket is a literal `[`, so the two characters
			// come off.
			"a prefix trim takes the characters", `w="[]abc"; echo "${w#[]}"`, "abc\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, dir, c.src+"\n")
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}
