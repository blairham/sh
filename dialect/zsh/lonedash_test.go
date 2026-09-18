// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A command word that is exactly `-` is thrown away here and the rest of the
// command runs, where the other six columns look it up and report 127.
//
// Measured 2026-09-18 on zsh 5.9.2 with `-f`, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and stdin `/dev/null`.
// Three rows are the ones that made the reading worth writing down: `--` is
// an ordinary command name, the word after the dash is a **command word**
// rather than a prefix, and what is left of `- >f` is not the state `>f`
// alone is — that runs the null command and this is `redirection with no
// command` (#3236).
func TestACommandWordThatIsOnlyADashIsDiscarded(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"the rest of the command runs", "- echo hi; print st=$?", "hi\nst=0\n", 0},
		{"as many dashes as are written", "- - - echo hi; print st=$?", "hi\nst=0\n", 0},
		{"quoting does not protect it", "'-' echo hi; print st=$?", "hi\nst=0\n", 0},
		{"nor does an expansion", "v=-\n$v echo hi; print st=$?", "hi\nst=0\n", 0},
		{"the status is the command's", "- false; print st=$?", "st=1\n", 0},
		{"two dashes is a command name", "-- echo hi; print st=$?", "zsh:1: command not found: --\nst=127\n", 0},
		{
			"the word after it is not a prefix",
			"- v=1 echo hi; print st=$?",
			"zsh:1: command not found: v=1\nst=127\n", 0,
		},
		{"nothing left is nothing run", "-; print st=$?", "st=0\n", 0},
		{"and the assignments still persist", "v=1 -; print \"st=$? v=[$v]\"", "st=0 v=[1]\n", 0},
		{"a function is reached through it", "f(){ print FN; }\n- f; print st=$?", "FN\nst=0\n", 0},
		{"and so is a builtin", "- print B; print st=$?", "B\nst=0\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != c.status {
				t.Errorf("%s =\n%q at %d\nwant\n%q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}

// And with the word gone there is no command for a redirection to belong to,
// which is **not** the state a command written with no word at all is in:
// `>f` alone runs this shell's null command and succeeds, where `- >f` is
// refused and ends the script.
func TestADiscardedDashLeavesARedirectionWithNoCommand(t *testing.T) {
	dir := t.TempDir()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: dir}, "- >out1\nprint after\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "zsh:1: redirection with no command\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}
