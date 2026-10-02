// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A substitution whose body holds a `case` read a line at a time — the
// standard input route — is read on to its real closer rather than closed at
// a pattern's parenthesis. Measured 2026-10-02 on zsh 5.9.2 with this program
// on standard input to `zsh -f` (#5145).
func TestASubstitutionReadALineAtATimeKeepsItsCase(t *testing.T) {
	src := "print $((case foo in\nbar)\necho not this no, no\n;;\nfoo)\necho yes, this one\n;;\nesac)\nprint after case in subshell)\n"
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Stdin = strings.NewReader(src)
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	code := driver.MainArgs(sh, []string{"zsh", "-f"})
	if got, want := out.String(), "yes, this one after case in subshell\n"; got != want || errs.Len() != 0 || code != 0 {
		t.Errorf("got %q, %q at %d, want %q at 0", got, errs.String(), code, want)
	}
}
