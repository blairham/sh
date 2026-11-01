// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// The pathname stage of a word list runs once every word of the list has had
// its other expansions, word by word, and stops at the first word it refuses
// (#6768). Code in a glob qualifier is the one place that stage runs
// commands, so it is the one place the order can be seen. Measured
// 2026-10-27 on zsh 5.9.2 under -fc, in a directory holding one file `q`;
// every `want` is that shell's output with both streams on one pipe.
func TestThePathnameStageRunsAfterEveryWordIsExpanded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a builtin's words",
			"echo q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\nq\n",
		},
		{
			"two words with code",
			"echo q*(e:'print -u2 Q1':) $(echo A >&2) q*(e:'print -u2 Q2':) $(echo B >&2)",
			"A\nB\nQ1\nQ2\nq q\n",
		},
		{
			"a word the braces made",
			"echo q*(e:'print -u2 Q':) $(echo A >&2){1,2}",
			"A\nQ\nq 1 2\n",
		},
		{
			"an arithmetic expansion in a later word",
			"echo q*(e:'print -u2 Q':) $(($(echo A >&2; echo 1)))",
			"A\nQ\nq 1\n",
		},
		{
			"the command's own name",
			// `q` is on the PATH the harness gives the shell, and is not
			// executable; zsh under the same PATH writes the same line.
			"q*(e:'print -u2 Q':) $(echo A >&2)",
			"A\nQ\nzsh:1: permission denied: q\n",
		},
		{
			"a for list",
			"for w in q*(e:'print -u2 Q':) $(echo A >&2); do echo $w; done",
			"A\nQ\nq\n",
		},
		{
			"an array literal",
			"a=( q*(e:'print -u2 Q':) $(echo A >&2) ); echo $a",
			"A\nQ\nq\n",
		},
		{
			"a declaration's literal operand",
			"typeset -a a=( q*(e:'print -u2 Q':) $(echo A >&2) ); echo $a",
			"A\nQ\nq\n",
		},
		{
			"a parameter the code sets is not seen by a later word",
			"echo q*(e:'X=set':) $X",
			"q\n",
		},
		{
			"a parameter a later word sets is seen by the code",
			"echo ${X::=set} q*(e:'print -u2 X=$X':)",
			"X=set\nset q\n",
		},
		{
			"the redirections come after the words",
			"echo q*(e:'print -u2 Q':) > $(echo A >&2; echo out); echo \"$(<out)\"",
			"Q\nA\nq\n",
		},
		{
			"a refusal from inside the code waits for the later word",
			"echo q*(e:\"echo zz* >&2\":) $(echo A >&2)",
			"A\nzsh:1: no matches found: zz*\nzsh:1: no matches found: q*(e:echo zz* >&2:)\n",
		},
		{
			"a refusal stops the stage: a builtin's words",
			"echo zz* q*(e:'print -u2 Q':); echo after",
			"zsh:1: no matches found: zz*\n",
		},
		{
			"a refusal stops the stage: a qualifier that will not read",
			"echo *(Z) q*(e:'print -u2 Q':); echo after",
			"zsh:1: unknown file attribute: Z\n",
		},
		{
			"a refusal stops the stage: a for list",
			"for w in zz* q*(e:'print -u2 Q':); do :; done; echo after",
			"zsh:1: no matches found: zz*\n",
		},
		{
			"a refusal stops the stage: an array literal",
			"a=( zz* q*(e:'print -u2 Q':) ); echo after",
			"zsh:1: no matches found: zz*\n",
		},
		{
			"a refusal stops the stage: a program's words",
			"/bin/echo zz* q*(e:'print -u2 Q':); echo after $?",
			"zsh:1: no matches found: zz*\nafter 1\n",
		},
		{
			"a refusal stops the stage: a second word's own code",
			"echo q*(e:'print -u2 Q1; false':) q*(e:'print -u2 Q2':); echo after",
			"Q1\nzsh:1: no matches found: q*(e:print -u2 Q1; false:)\n",
		},
		{
			"the words before a refusal are matched first",
			"/bin/echo q*(e:'print -u2 Q':) zz*; echo after $?",
			"Q\nzsh:1: no matches found: zz*\nafter 1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "q"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			out, _ := runZsh(t, dir, tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
