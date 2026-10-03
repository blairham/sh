// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `<>` onto a process substitution the command made takes the substitution's
// own end: the write reaches a `>(cmd)` and the read comes from a `<(cmd)`.
// Measured 2026-10-02 on zsh 5.9.2, `zsh -f -c` with stdin on the null device
// (#5514). The last row is the control: a substitution the command did not
// make is a descriptor that is gone. See
// interp.Semantics.ReadWriteRedirectionTakesTheSubstitutionsEnd.
func TestAReadWriteRedirectionTakesTheSubstitutionsEnd(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{"print a <> >(tr a b); echo st=$?", "a\nst=0\n", ""},
		{"print a 3<> >(tr a c) >&3; echo st=$?", "c\nst=0\n", ""},
		{"{ print x >&4 } 4<> >(tr x y); echo st=$?", "y\nst=0\n", ""},
		{"cat <> <(echo in); echo st=$?", "in\nst=0\n", ""},
		// The wording is the platform's: `bad file descriptor` for the
		// `/dev/fd` path on macOS, `no such file or directory` for the
		// `/proc/self/fd` one on Linux. What is asserted is the refusal.
		{"x=>(cat); print a <> $x; echo st=$?", "st=1\n", "/fd/"},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
		driver.MainArgs(sh, []string{"zsh", "-fc", c.src})
		if out.String() != c.out || !strings.Contains(errs.String(), c.errs) || (c.errs == "" && errs.Len() > 0) {
			t.Errorf("%s\n got %q, %q\nwant %q, %q", c.src, out.String(), errs.String(), c.out, c.errs)
		}
	}
}
