// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestDirsDrawsTheStackByThePromptsRule is #6025: `dirs` writes the home as
// `~` only for the home itself or a path under it, a value ending in `/`
// abbreviates nothing, and a named directory is drawn as `~name` — the rule
// `%~`, `print -D` and `(D)` already follow. Measured 2026-10-05 on zsh 5.9.2
// (/opt/homebrew/bin/zsh, -f), from /tmp with /usr pushed.
func TestDirsDrawsTheStackByThePromptsRule(t *testing.T) {
	out, st := runZshPrelude(t, t.TempDir(), `cd /tmp; pushd -q /usr
HOME=/; dirs
HOME=/usr/; dirs
HOME=/us; dirs
HOME=; dirs
HOME=/usr; dirs
dirs -l
HOME=/nowhere; hash -d xx=/usr; dirs
dirs -v
abbreviatedir v /usr; echo st=$?`)
	want := strings.Join([]string{
		"/usr /tmp",
		"/usr /tmp",
		"/usr /tmp",
		"/usr /tmp",
		"~ /tmp",
		"/usr /tmp",
		"~xx /tmp",
		"0\t~xx",
		"1\t/tmp",
		"st=127",
	}, "\n") + "\n"
	got := out
	// The seam is the prelude's own: a script that names it is told the
	// command is not there, as zsh tells it. Only the status is pinned
	// here; the sentence is the dialect's ordinary not-found line.
	if i := strings.Index(got, "command not found"); i >= 0 {
		j := strings.LastIndex(got[:i], "\n") + 1
		k := strings.Index(got[i:], "\n") + i + 1
		got = got[:j] + got[k:]
	}
	if st != 0 || got != want {
		t.Errorf("status %d, output:\n%s\nwant:\n%s", st, out, want)
	}
}
