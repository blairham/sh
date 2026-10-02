// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// **`-v` writes back a `-c` string the way it was read: all of it, then one
// newline, before any of it runs** (#5428). It was echoed a line at a time as
// it ran, and the last line came out with no newline after it, so it ran into
// the first byte the command wrote.
//
// Measured 2026-10-02 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `-f`, both streams on one pipe through
// `sed -n l`. Each row is the whole of what the run wrote.
func TestVerboseEchoesACommandStringWholeBeforeRunningIt(t *testing.T) {
	for _, c := range []struct{ name, flags, src, want string }{
		{"two lines", "-fv", "echo a\necho b", "echo a\necho b\na\nb\n"},
		{"two commands on one line", "-fv", "echo a; echo b", "echo a; echo b\na\nb\n"},
		{"one line", "-fv", "echo a", "echo a\na\n"},
		// The newline is added even where the text already ends in one, so
		// the echo is one line longer than the text.
		{"a trailing newline", "-fv", "echo a\n", "echo a\n\na\n"},
		{"an empty string", "-fv", "", "\n"},
		// Turning it off on the first line still echoes that line: the read
		// happened before the line ran.
		{"turned off by its own first line", "-fv", "set +v\necho a", "set +v\necho a\na\n"},
		// And turning it on there echoes nothing of this text, for the same
		// reason — nor the text `eval` reads after it.
		{"turned on by its own first line", "-f", "set -v\neval 'echo e'\necho a", "e\na\n"},
		// Text that will not parse is echoed in full before the refusal.
		{"before a refusal", "-fv", "echo a\nif; then", "echo a\nif; then\nzsh:2: parse error near `then'\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runZshMerged(t, c.flags, "-c", c.src)
			if out != c.want {
				t.Errorf("%q\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}

// The control: a script file is read as it runs in zsh too, so it echoes a
// line at a time — and a frozen whole-text echo on that route would put both
// lines out before the first one ran.
func TestVerboseStillEchoesAScriptALineAtATime(t *testing.T) {
	script := filepath.Join(t.TempDir(), "s.zsh")
	if err := os.WriteFile(script, []byte("echo a\necho b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runZshMerged(t, "-fv", script)
	if want := "echo a\na\necho b\nb\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// runZshMerged is runZsh with both streams on one writer, the way a person
// reading `-v` sees them: the echo is on descriptor 2 and the commands write to
// 1, so only one writer can say which came first.
func runZshMerged(t *testing.T, argv ...string) string {
	t.Helper()
	var both strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &both, &both
	driver.MainArgs(sh, append([]string{"zsh"}, argv...))
	return both.String()
}
