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

// A `$ENV` given up over a fatal error leaves `$?` as it was before the
// command that failed (#6046). See
// interp.Semantics.StartupFileGivenUpLeavesTheStatusBefore.
//
// Measured 2026-10-05 on ksh93u+ 2012-08-01 (/bin/ksh), `env -i`, `$ENV` of
// each row and then `echo after`, `ksh -E -c 'echo main $?'`. This shell
// answered `main 1` for every row.
func TestAGivenUpEnvLeavesTheStatusBefore(t *testing.T) {
	for _, c := range []struct{ env, out string }{
		{"true\necho ${unset?boom}\n", "main 0\n"},
		{"false\necho ${unset?boom}\n", "main 1\n"},
		{"(exit 7)\necho ${unset?boom}\n", "main 7\n"},
		{"(exit 7)\necho $((1/0))\n", "main 7\n"},
		// The command and not the line, and the set -u before the failure
		// is a command too.
		{"(exit 7)\nset -u; echo $nope\n", "main 0\n"},
		{"(exit 7)\nfalse; echo ${unset?boom}\n", "main 1\n"},
		{"f(){ (exit 4); echo ${unset?boom}; }; (exit 7); f\n", "main 4\n"},
		// A `.` the error gives up is a boundary of its own, which leaves 1,
		// and $ENV carries on from it.
		{"(exit 7)\n. ./g.sh\necho dot $?\n", "dot 1\nafter\nmain 0\n"},
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(home)
		env := filepath.Join(home, "env.sh")
		if err := os.WriteFile(env, []byte(c.env+"echo after\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "g.sh"), []byte("echo ${unset?boom}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ENV", env)
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		var o, e bytes.Buffer
		sh := shell()
		sh.SystemStartupDirectory = t.TempDir()
		sh.Stdout, sh.Stderr, sh.Stdin = &o, &e, null
		driver.MainArgs(sh, []string{"ksh", "-E", "-c", "echo main $?"})
		_ = null.Close()
		if o.String() != c.out {
			t.Errorf("%q: stdout %q, want %q (stderr %q)", c.env, o.String(), c.out, e.String())
		}
	}
}
