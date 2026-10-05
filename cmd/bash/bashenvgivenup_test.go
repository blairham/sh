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

// A `$BASH_ENV` given up over a fatal expansion error leaves 1, the status a
// file ends with, and not the 127 the same error exits a `-c` string with
// (#6046). See interp.Diagnostics.ExpansionFailureStatusFromCommandString and
// interp.Semantics.StartupFileGivenUpLeavesTheStatusBefore.
//
// Measured 2026-10-05 on bash 5.3.20, `env -i`, `$BASH_ENV` of `<pre>`, the
// failing line and `echo after`, then `-c 'echo main $?'`: `main 1` for every
// row, and `after` never. This shell answered `main 127`.
func TestABashEnvGivenUpLeavesOne(t *testing.T) {
	for _, c := range []struct{ env, cmd, out string }{
		{"true\necho ${unset?boom}\necho after\n", "echo main $?", "main 1\n"},
		{"(exit 7)\necho ${unset?boom}\necho after\n", "echo main $?", "main 1\n"},
		{"set -u\necho $nope\necho after\n", "echo main $?", "main 1\n"},
		// Through a file the startup file sources: still the file's 1.
		{". ./g.sh\necho after\n", "echo main $?", "main 1\n"},
		// The control: a function the startup file defined fails in the
		// string's own run, and the string's 127 stands.
		{"f(){ echo ${unset?boom}; }\n", "f; echo main $?", ""},
	} {
		home := scratchHome(t)
		t.Chdir(home)
		writeHomeFile(t, home, "rc.sh", c.env)
		writeHomeFile(t, home, "g.sh", "(exit 7)\necho ${unset?boom}\n")
		t.Setenv("BASH_ENV", filepath.Join(home, "rc.sh"))
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		sh.Stdin = null
		code := driver.MainArgs(sh, []string{"bash", "-c", c.cmd})
		_ = null.Close()
		if o.String() != c.out {
			t.Errorf("%q: stdout %q, want %q (stderr %q)", c.env, o.String(), c.out, e.String())
		}
		if c.out == "" && code != 127 {
			t.Errorf("%q: status %d, want 127 (stderr %q)", c.env, code, e.String())
		}
	}
}
