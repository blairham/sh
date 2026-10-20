// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A redirection's target is **not** brace-expanded here, and this dialect is
// the one that says so on its own: it brace-expands an argument like bash and
// zsh do, and leaves a target as the one name it is written as.
//
// The rows are the third answer in the panel, beside dialect/zsh's three names
// and dialect/bash's refusal, and they are here because #4455 gave a target's
// braces to the shell that fans a stream out to every name it was given. This
// shell has no fan, so nothing about that change may reach it — a test naming
// only the two shells that moved would not have said so.
//
// Measured 2026-09-25 against ksh93u+ 2012-08-01 (`/bin/ksh`), each row in an
// empty directory of its own:
//
//	print -- {a,b}                   `a b` — the argument is expanded
//	: > {a,b}                        one file called `{a,b}`
//	: > d/{a,b,c}                    one file called `d/{a,b,c}`
//	: > {1..3}                       one file called `{1..3}`
//	f() { echo 3; }; : > {1..$(f)}   one file called `{1..3}`, `f` run once
//
// The last row is the one to keep. The endpoint *is* expanded — the name is
// `{1..3}` and not `{1..$(f)}` — and the range still does not form, so the
// substitution runs exactly once and the target is the text it came to. A
// count that built the words before asking whose braces they are would run it
// twice here.
func TestARedirectionTargetIsNotBraceExpandedHere(t *testing.T) {
	// The control first, and it is the half that makes the rest evidence: a
	// shell that had simply lost brace expansion would pass every row below.
	t.Run("an argument is brace-expanded", func(t *testing.T) {
		if out, st := runKsh(t, t.TempDir(), `printf '[%s]' {a,b}`); out != "[a][b]" || st != 0 {
			t.Fatalf("got %q (status %d), want [a][b] at 0", out, st)
		}
	})

	for _, tc := range []struct{ name, src, file string }{
		{"two names is one name", `: > {a,b}`, "{a,b}"},
		{"the line dialect/zsh was made for", `: > d/{a,b,c}`, "d/{a,b,c}"},
		{"text behind the group", `: > {a,b}c`, "{a,b}c"},
		{"a range", `: > {1..3}`, "{1..3}"},
		{
			"a range whose end is an expansion, run once",
			`f() { echo 3; }; : > {1..$(f)}`,
			"{1..3}",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			// The subdirectory row wants somewhere to put the name, and the
			// name holds a `/` only because the braces were not read.
			if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
				t.Fatal(err)
			}
			out, st := runKsh(t, dir, tc.src)
			if out != "" || st != 0 {
				t.Fatalf("%s gave %q (status %d), want nothing at 0", tc.src, out, st)
			}
			if _, err := os.Stat(filepath.Join(dir, tc.file)); err != nil {
				names, _ := os.ReadDir(dir)
				t.Errorf("%s left %v, want one file called %q", tc.src, names, tc.file)
			}
		})
	}
}
