// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A definition's name holding a bare `*`, `?` or `[` is a **pattern**: this
// shell matches it against the filesystem when the definition runs, and
// defines one function per match.
//
// It was a parse error here, for both spellings, on the reasoning that a
// function literally called `a*b` would be a plausible wrong answer where a
// refusal is a visible one. That reasoning is about *defining* the name and
// not about reading the line: the cost of the refusal was the rest of the
// file, in a shell whose own `-n` reads it — which is how this was found, as
// one of the two files the zsh column's static read refused and the reference
// read (#4437).
//
// Measured on zsh 5.9.2, 2026-09-25, from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, in a directory holding `ax` and `bx`.
func TestAFunctionNameIsMatchedAgainstTheFilesystem(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		// One definition per match, and the name that was written is not one
		// of them. Both spellings, which is what says the rule is the name's
		// and not the `()`'s.
		{
			"the name() spelling", "?x() { :; }\nprint -l ${(ok)functions}\n",
			"ax\nbx\n", 0,
		},
		{
			"the keyword spelling", "function ?x { :; }\nprint -l ${(ok)functions}\n",
			"ax\nbx\n", 0,
		},
		// A pattern matching nothing is the ordinary unmatched-pattern
		// refusal — not a complaint about a definition — and it is fatal.
		{
			"a pattern matching nothing", "nomatch?zz() { :; }\necho after\n",
			"zsh:1: no matches found: nomatch?zz\n", 1,
		},
		{
			"the keyword spelling matching nothing", "function a*b { :; }\necho after\n",
			"zsh:1: no matches found: a*b\n", 1,
		},
		// **The row that says this happens when the definition runs**: a
		// branch nothing takes says nothing at all, which a parse-time
		// refusal cannot do however it is worded.
		{
			"inside a branch nothing takes", "if false; then nomatch?zz() { :; }; fi\necho after\n",
			"after\n", 0,
		},
		// And the quoting decides it, per span: quoted or escaped, the
		// characters are ordinary text and name the function they spell.
		{
			"a quoted pattern", "'a*b'() { :; }\nprint -l ${(ok)functions}\n",
			"a*b\n", 0,
		},
		{
			"an escaped pattern", "a\\*b() { :; }\nprint -l ${(ok)functions}\n",
			"a*b\n", 0,
		},
		{
			"an escaped pattern after the keyword", "function a\\*b { :; }\nprint -l ${(ok)functions}\n",
			"a*b\n", 0,
		},
		// A name list stops at the first name that matches nothing, at the
		// name that carried it — the same place the script would have stopped
		// had the names been written out.
		{
			"a name list stopping at a pattern", "nomatch?zz second() { :; }\necho after\n",
			"zsh:1: no matches found: nomatch?zz\n", 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range []string{"ax", "bx"} {
				if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
		})
	}
}
