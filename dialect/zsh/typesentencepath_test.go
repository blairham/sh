// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The path inside a `type`-family sentence is written back the way this shell
// writes a word, and the same path with no sentence around it is written
// plain.
//
// Measured 2026-09-19 on zsh 5.9.2 at /opt/homebrew/bin/zsh, each probe a
// script file under `env -i PATH=<dir>:/usr/bin:/bin LC_ALL=C` with a
// directory on PATH holding an executable file literally called `a b`. The
// reference column and this one agree byte for byte over 32 names differing
// by one byte and eight routes each, of which these are the shapes.
//
// The **name** is bare on both sides of the sentence — `a b is …` and not
// `'a b' is …` — which is what keeps this apart from
// Diagnostics.NameReportQuoting, the field ksh93 answers (#3702).
func TestTheSentenceQuotesThePathAndTheBareFormsDoNot(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "a b")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	quoted := "'" + tool + "'"
	for _, c := range []struct{ name, src, want string }{
		// The sentences.
		{"type", `type "a b"`, "a b is " + quoted},
		{"command -V", `command -V "a b"`, "a b is " + quoted},
		{"whence -v", `whence -v "a b"`, "a b is " + quoted},
		{"type -a", `type -a "a b"`, "a b is " + quoted},
		// The bare-path forms, which are the other half of the claim: the
		// same resolved path with nothing around it.
		{"command -v", `command -v "a b"`, tool},
		{"whence", `whence "a b"`, tool},
		{"where", `where "a b"`, tool},
		{"which", `which "a b"`, tool},
		// And the control, where neither route has anything to quote.
		{"a name needing nothing", `type -- :`, ": is a shell builtin"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, c.src+"\n")
			if strings.TrimRight(out, "\n") != c.want {
				t.Errorf("%s = %q, want %q", c.src, out, c.want)
			}
			if st != 0 {
				t.Errorf("%s: status %d, want 0", c.src, st)
			}
		})
	}
}

// A pathname operand is quoted in the sentence too, and the word it was
// written as is what the sentence says back.
func TestAPathnameOperandIsQuotedInTheSentence(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bb"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bb", "a b"), []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `type "./bb/a b"`+"\n")
	if want := "./bb/a b is './bb/a b'"; strings.TrimRight(out, "\n") != want {
		t.Errorf("type = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}
