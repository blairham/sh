// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runKshFile runs src as a script file named p.sh, the route that locates a
// complaint as `p.sh: line N:`, and returns both streams with the scratch
// directory taken out of the script's path. One buffer takes both, so the
// order they were written in is kept.
func runKshFile(t *testing.T, src string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	var both bytes.Buffer
	status := driver.MainArgs(kshShell(&both, &both), []string{"ksh", path})
	return strings.ReplaceAll(both.String(), dir+string(filepath.Separator), ""), status
}

// A write that would aim a name reference at a word that is no name is this
// shell's ordinary bad-name sentence, with no backquotes, and it ends the
// shell whichever route made the write. Measured 2026-11-01 against ksh93u+
// 2012-08-01 (`/bin/ksh`) under `env -i PATH=/usr/bin:/bin` with a scratch
// HOME, each row from a script file and from `-c`, with `echo next` on the
// line after. This wrote bash's sentence, `not a valid identifier` with the
// word in backquotes, gave up only
// the rest of the line on the bare assignment and carried on to `st=1` after
// the loop (#7085).
//
// The bare assignment ending the shell is the pair that says the key is the
// refusal and not the loop: the two routes agree here. The last two rows are
// the controls, where a boundary any fatal error stops at stops this one too.
func TestARefusedReferenceAimIsAnInvalidVariableNameAndEndsTheShell(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, file, cmd string
		status               int
	}{
		{
			"a digit", `typeset -n m; m=1; echo st=$?`,
			"p.sh: line 1: 1: invalid variable name\n", "ksh: 1: invalid variable name\n", 1,
		},
		{
			"a word holding an equals sign", `typeset -n m; m=a=b; echo st=$?`,
			"p.sh: line 1: a=b: invalid variable name\n", "ksh: a=b: invalid variable name\n", 1,
		},
		{
			"a word holding a blank", `typeset -n m; m="a b"; echo st=$?`,
			"p.sh: line 1: a b: invalid variable name\n", "ksh: a b: invalid variable name\n", 1,
		},
		{
			"a loop over an aimed reference", `x=0; typeset -n m=x; for m in 1; do :; done; echo st=$?`,
			"p.sh: line 1: 1: invalid variable name\n", "ksh: 1: invalid variable name\n", 1,
		},
		{
			"a loop runs the words in front of it", `typeset -n m; for m in a 1; do echo body=$m; done; echo f=$?`,
			"body=\np.sh: line 1: 1: invalid variable name\n", "body=\nksh: 1: invalid variable name\n", 1,
		},
		{
			"a subshell ends alone", `typeset -n m; (m=1); echo sub=$?`,
			"p.sh: line 1: 1: invalid variable name\nsub=1\nnext\n", "ksh: 1: invalid variable name\nsub=1\nnext\n", 0,
		},
		{
			"a keyword function's call ends alone", `function f { typeset -n m; m=1; echo in=$?; }; f; echo out=$?`,
			"p.sh[1]: f: line 1: 1: invalid variable name\nout=1\nnext\n",
			"ksh: f: line 1: 1: invalid variable name\nout=1\nnext\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			src := c.src + "\necho next\n"
			if got, st := runKshFile(t, src); got != c.file || st != c.status {
				t.Errorf("from a file: %q at %d, want %q at %d", got, st, c.file, c.status)
			}
			var both bytes.Buffer
			st := driver.MainArgs(kshShell(&both, &both), []string{"ksh", "-c", src})
			if got := both.String(); got != c.cmd || st != c.status {
				t.Errorf("from -c: %q at %d, want %q at %d", got, st, c.cmd, c.status)
			}
		})
	}
}

// And the word that is a name is still taken, which keeps the refusal above a
// statement about the word rather than about aiming.
func TestAReferenceAimedAtANameIsStillTaken(t *testing.T) {
	t.Parallel()
	got, st := runKshFile(t, "typeset -n m; m=x; x=5; echo \"$m\"\n"+
		"x=0; typeset -n r=x; for r in y; do :; done; typeset -p r\n")
	if want := "5\ntypeset -n r=y\n"; got != want || st != 0 {
		t.Errorf("%q at %d, want %q at 0", got, st, want)
	}
}
