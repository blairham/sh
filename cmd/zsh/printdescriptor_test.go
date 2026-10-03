// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `print -u N` looks the number up as one: descriptor 0 opened for writing is
// written to; a number open only for reading is `bad mode on fd M`, where M is
// the lowest free descriptor from 3 up; a closed one is `bad file number`.
// Measured 2026-10-03 on zsh 5.9.2, `zsh -f -c` with stdin on the null device
// and `f` a file holding `hi` (#5551).
func TestPrintLooksUpANamedDescriptor(t *testing.T) {
	for _, c := range []struct{ src, out, errs string }{
		{"print -u0 a 0>f; cat f; echo st=$?", "a\nst=0\n", ""},
		{"exec 0>f; print -u0 b; exec 0</dev/null; cat f", "b\n", ""},
		{"print -u0 a 0<>f; cat f; echo st=$?", "a\n\nst=0\n", ""},
		{"print -u0 a; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 3\n"},
		{"print -u0 a <<<x; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 3\n"},
		{"echo hi | print -u0 a; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 3\n"},
		{"print -u3 a 3<f; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 4\n"},
		{"print -u5 a 5<f; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 3\n"},
		{"exec 3</dev/null 4</dev/null; print -u1 a 1</dev/null; echo st=$?", "st=1\n", "zsh:print:1: bad mode on fd 5\n"},
		{"print -u0 a 0<&-; echo st=$?", "st=1\n", "zsh:print:1: bad file number: 0\n"},
		{"print -u1 foo >&-; echo st=$? >&2", "", "zsh:print:1: bad file number: 1\nst=1\n"},
		{"print foo >&-; echo st=$? >&2", "", "st=0\n"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f"), []byte("hi\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		null, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdin, sh.Stdout, sh.Stderr = null, &out, &errs
		sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
		driver.MainArgs(sh, []string{"zsh", "-fc", c.src})
		_ = null.Close()
		if out.String() != c.out || errs.String() != c.errs {
			t.Errorf("%s\n got %q, %q\nwant %q, %q", c.src, out.String(), errs.String(), c.out, c.errs)
		}
	}
}
