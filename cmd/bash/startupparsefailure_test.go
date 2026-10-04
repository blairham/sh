// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `$BASH_ENV` that will not parse runs up to the line that will not, names
// itself by its path, and leaves the command the shell was started for to run
// (#5869).
//
// Measured 2026-10-04 on bash 5.3.20 (/opt/homebrew/bin/bash) with standard
// input on the null device — bash reads no `$BASH_ENV` at all with a socket
// there — and every line below is that shell's, byte for byte, with the
// scratch directory standing for the path. This shell used to parse the file
// whole first: it ran none of it, skipped the command, and wrote the file's
// whole text where its name goes.
func TestABashEnvThatWillNotParseRunsUpToTheFailure(t *testing.T) {
	for _, c := range []struct {
		name, body, cmd, out, errs string
	}{
		{
			name: "an unclosed brace at the end of the file",
			body: "echo before\nnosuchcmd_q\necho ${x\necho after\n",
			cmd:  "echo main",
			out:  "before\nmain\n",
			errs: "@/rc.sh: line 2: nosuchcmd_q: command not found\n" +
				"@/rc.sh: line 3: unexpected EOF while looking for matching `}'\n",
		},
		{
			name: "a stray token in the middle, which costs the rest of the file",
			body: "echo before\ntrue\necho )\necho after\n",
			cmd:  "echo main $?",
			out:  "before\nmain 2\n",
			errs: "@/rc.sh: line 3: syntax error near unexpected token `)'\n" +
				"@/rc.sh: line 3: `echo )'\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := scratchHome(t)
			writeHomeFile(t, home, "rc.sh", c.body)
			t.Setenv("BASH_ENV", filepath.Join(home, "rc.sh"))
			var o, e bytes.Buffer
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &o, &e
			null, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = null.Close() })
			sh.Stdin = null
			code := driver.MainArgs(sh, []string{"bash", "-c", c.cmd})
			if code != 0 {
				t.Errorf("status %d, want the command's own 0", code)
			}
			if o.String() != c.out {
				t.Errorf("stdout %q, want %q", o.String(), c.out)
			}
			if want := bytes.ReplaceAll([]byte(c.errs), []byte("@"), []byte(home)); e.String() != string(want) {
				t.Errorf("stderr %q, want %q", e.String(), want)
			}
		})
	}
}

// Reading a line at a time is also what lets a line of the file change how the
// next one is read. Measured on the same bash: a `$BASH_ENV` of `shopt -s
// extglob` and then `echo @(a|b)`, run from an empty directory, prints the
// pattern and then the command's output. Read whole, the second line was
// parsed before the option existed and refused.
func TestABashEnvsGrammarOptionReachesItsNextLine(t *testing.T) {
	home := scratchHome(t)
	writeHomeFile(t, home, "rc.sh", "shopt -s extglob\necho @(a|b)\n")
	t.Setenv("BASH_ENV", filepath.Join(home, "rc.sh"))
	t.Chdir(t.TempDir())
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	if code := driver.MainArgs(sh, []string{"bash", "-c", "echo main"}); code != 0 {
		t.Errorf("status %d, want 0", code)
	}
	if want := "@(a|b)\nmain\n"; o.String() != want || e.Len() != 0 {
		t.Errorf("stdout %q and stderr %q, want %q and nothing", o.String(), e.String(), want)
	}
}
