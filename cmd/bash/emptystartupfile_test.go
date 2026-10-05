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

// A startup file with no command in it keeps the status the file before it
// left, which is the other half of zsh's rule (#5883): measured 2026-10-05 on
// bash 5.3.20, a `.bash_profile` of `false` and then an empty `$BASH_ENV`
// answer `bash -l -c 'echo st=$?'` with `st=1`, and a `$BASH_ENV` of `true`
// makes it `st=0`. bash 3.2 also says `st=1`.
func TestAnEmptyBashEnvKeepsTheStatusBeforeIt(t *testing.T) {
	for _, c := range []struct{ env, out string }{
		{"", "st=1\n"},
		{"# nothing\n", "st=1\n"},
		{"true\n", "st=0\n"},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, ".bash_profile", "false\n")
		writeHomeFile(t, home, "rc.sh", c.env)
		t.Setenv("BASH_ENV", filepath.Join(home, "rc.sh"))
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		sh.Stdin = null
		driver.MainArgs(sh, []string{"bash", "-l", "-c", "echo st=$?"})
		_ = null.Close()
		if o.String() != c.out {
			t.Errorf("%q: stdout %q, want %q (stderr %q)", c.env, o.String(), c.out, e.String())
		}
	}
}

// `$?` inside `~/.bash_logout` is the status the `exit` found, not the one it
// named: measured 2026-10-05 on bash 5.3.20, `bash -l -c 'false; exit 3'`
// over a `.bash_logout` of `echo st=$?` prints `st=1` and leaves 3, and
// `true; exit 3` prints `st=0`. This shell printed 3 for both (#5996).
func TestALogoutFileSeesTheStatusTheExitFound(t *testing.T) {
	for _, c := range []struct{ cmd, want string }{
		{"false; exit 3", "st=1\n"},
		{"true; exit 3", "st=0\n"},
	} {
		home := scratchHome(t)
		writeHomeFile(t, home, ".bash_profile", "")
		writeHomeFile(t, home, ".bash_logout", "echo st=$?\n")
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		if code := driver.MainArgs(sh, []string{"bash", "-l", "-c", c.cmd}); code != 3 {
			t.Errorf("%s: status %d, want 3", c.cmd, code)
		}
		if o.String() != c.want {
			t.Errorf("%s: stdout %q, want %q (stderr %q)", c.cmd, o.String(), c.want, e.String())
		}
	}
}
