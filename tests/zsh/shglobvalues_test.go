// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShGlobLeavesValuesAndRangesAlone pins two things `shglob` does that
// were missing. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5155): a value
// that is not a pattern stays text when the option has taken the group syntax
// away, and a numeric range stops matching a number while its word is still a
// pattern.
func TestShGlobLeavesValuesAndRangesAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a9"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ src, want string }{
		{"setopt shglob; s='a(b)'; print $s", "a(b)\n"},
		{"setopt shglob; s='a|b'; print $s", "a|b\n"},
		{"setopt shglob; for s in '(x)'; do print $s; done", "(x)\n"},
		{"setopt shglob; [[ a9 = a<1-10> ]] && print y || print n", "n\n"},
		{"setopt shglob; x='a<1-10>'; [[ 'a<1-10>' = ${~x} ]] && print y || print n", "y\n"},
		{"setopt shglob; print a<1-10>(N)", "\n"},
		{"print a<1-10>(N)", "a9\n"},
		{"setopt shglob; x='a(b|c)'; [[ ab = ${~x} ]] && print y || print n", "n\n"},
		{"x='a(b|c)'; [[ ab = ${~x} ]] && print y || print n", "y\n"},
	}
	for _, c := range cases {
		if got, _ := runZsh(t, dir, c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
