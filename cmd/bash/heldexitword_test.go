// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// An `exit` that a running job holds back still writes the word the shell
// says on its way out, before the sentence about the job (#6058). See
// interp.Runner.LeavingWord.
//
// Measured 2026-10-05 on bash 5.3.20 through a pseudo-terminal and on a pipe
// alike, `--norc -i` with a scratch HOME, `shopt -s checkjobs` and `sleep 2 &`
// first: `exit` writes `exit`, then `There are running jobs.` and the job,
// and a login shell writes `logout` there. `logout` writes no word, held or
// not, and the `exit` after it, which leaves, does. This shell wrote the word only when the
// shell left. Typed as `exit 0` so the echo of the typed line cannot stand for
// the word.
func TestAHeldExitStillSaysItsWord(t *testing.T) {
	const held = "There are running jobs.\n"
	for _, c := range []struct {
		name, typed string
		argv        []string
		want        []string
	}{
		{
			"exit", "exit 0\necho after\n",
			[]string{"bash", "--norc", "-i"},
			[]string{"$ exit 0\nexit\n" + held},
		},
		{
			"a login shell", "exit 0\necho after\n",
			[]string{"bash", "--norc", "--noprofile", "-l", "-i"},
			[]string{"$ exit 0\nlogout\n" + held},
		},
		{
			"logout", "logout\nexit 0\n",
			[]string{"bash", "--norc", "--noprofile", "-l", "-i"},
			[]string{"$ logout\n" + held, "$ exit 0\nlogout\n"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			scratchHome(t)
			_, errs, _ := prompt(t, "shopt -s checkjobs\nsleep 2 &\n"+c.typed+"kill %1; wait\n", c.argv...)
			for _, w := range c.want {
				if !strings.Contains(errs, w) {
					t.Errorf("stderr %q, want %q in it", errs, w)
				}
			}
		})
	}
}
