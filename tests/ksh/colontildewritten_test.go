// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// A tilde behind an assignment's colon ends its name only where the word
// writes the end, as a leading tilde does. Measured 2026-10-03 on ksh93u+.
func TestAColonTildeNameEndsWhereTheWordWritesIt(t *testing.T) {
	t.Setenv("HOME", "/H")
	for _, c := range []struct{ src, want string }{
		{`u=/x; v=a:~$u; echo "$v"`, "a:~/x\n"},
		{`v=a:~\/x; echo "$v"`, "a:~/x\n"},
		{`v=a:~"/x"; echo "$v"`, "a:/H/x\n"},
		{`v=a:~"":b; echo "$v"`, "a:/H:b\n"},
	} {
		var out, errs bytes.Buffer
		driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", c.src})
		if got := out.String() + errs.String(); got != c.want {
			t.Errorf("%s: %q, want %q", c.src, got, c.want)
		}
	}
}
