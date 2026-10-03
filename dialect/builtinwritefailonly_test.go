// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **Which builtins a failed write into a closed descriptor fails** —
// Semantics.BuiltinWriteErrorFailsOnlyThese. ksh93 fails `echo`, `print` and
// `printf` and nobody else; bash fails every one of them and says so.
//
// Measured 2026-10-03 with `( cmd >&- ) 2>e; echo "st=$?"; cat e` on ksh93u+
// and bash 5.3.20. `print` is in the ksh row because it used to drop its own
// write error and answer 0 there whatever the axis said.
func TestWhichBuiltinsAFailedWriteFails(t *testing.T) {
	probe := func(cmd string) string {
		return "readonly RO=1; ( " + cmd + " >&- ) 2>e; echo \"" + cmd + " st=$?\"; while IFS= read -r l; do echo \"$l\"; done <e\n"
	}
	for _, c := range []struct {
		dialect, cmd, want string
	}{
		{"ksh", "echo hi", "echo hi st=1\n"},
		{"ksh", "print hi", "print hi st=1\n"},
		{"ksh", "printf hi", "printf hi st=1\n"},
		{"ksh", "pwd", "pwd st=0\n"},
		{"ksh", "export", "export st=0\n"},
		{"ksh", "readonly", "readonly st=0\n"},
		{"bash", "pwd", "pwd st=1\nbash: line 1: pwd: write error: Bad file descriptor\n"},
	} {
		out, _, err := presets[c.dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, probe(c.cmd))
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s %q: got %q, want %q", c.dialect, c.cmd, out, c.want)
		}
	}
}
