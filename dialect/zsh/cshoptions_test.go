// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCshJunkieQuotesRefusesAQuotedNewline pins `cshjunkiequotes`: a newline
// inside quotes is the quote that never closed unless a backslash escapes it,
// and then it is the newline without the backslash. Measured 2026-10-02 on
// zsh 5.9.2 from a script file (#5155).
func TestCshJunkieQuotesRefusesAQuotedNewline(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"setopt cshjunkiequotes\neval \"print 'one\ntwo'\"\nprint after", "(eval):1: unmatched '\nafter\n"},
		{"setopt cshjunkiequotes\neval 'print \"one\ntwo\"'\nprint after", "(eval):1: unmatched \"\nafter\n"},
		// Through eval, so the text is read after the option is set: the
		// command string itself is read whole before it runs.
		{"setopt cshjunkiequotes\neval \"print 'three\\\\\nfour'\"", "three\nfour\n"},
		{"setopt cshjunkiequotes\neval 'print \"five\\\nsix\"'", "five\nsix\n"},
		// The control: without the option the double-quoted pair joins.
		{"print \"five\\\nsix\"", "fivesix\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%q\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestCshNullGlobJudgesTheWholeList pins `cshnullglob`: an unmatched pattern
// is deleted, and the word list is `no match` only where none of its patterns
// matched. Measured 2026-10-02 on zsh 5.9.2 under `-fc` (#5155).
func TestCshNullGlobJudgesTheWholeList(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"tmpa", "tmpb"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{"print tmp* nothing* blah", "tmpa tmpb blah\n"},
		{"print nothing* blah; print after", "zsh:1: no match\n"},
		{"for x in nothing* tmp*; do print $x; done", "tmpa\ntmpb\n"},
		{"a=(nothing* tmp*); print $a", "tmpa tmpb\n"},
		{"a=(nothing*); print after", "zsh:1: no match\n"},
		{"(print nothing* blah); print sub $?", "zsh:1: no match\nsub 1\n"},
		{"setopt nullglob; print nothing* blah", "blah\n"},
		{`print "nothing*" blah`, "nothing* blah\n"},
	} {
		got, _ := runZsh(t, dir, "setopt cshnullglob; "+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
