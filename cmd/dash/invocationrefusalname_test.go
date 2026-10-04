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

// An option refused at the invocation is the shell's to name even when a
// script operand follows it. Measured 2026-10-04 on dash 0.5.12 with
// `dash -o nosuch x.sh`: `dash: 0: Illegal option -o nosuch`, where this
// wrote the script's name.
func TestAnInvocationRefusalNamesTheShellAndNotTheScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.sh"), []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-o", "nosuch", "x.sh"}, "dash: 0: Illegal option -o nosuch\n"},
		{[]string{"-Z", "x.sh"}, "dash: 0: Illegal option -Z\n"},
	} {
		var o, e bytes.Buffer
		sh := shell()
		sh.SystemStartupDirectory = t.TempDir()
		sh.Stdout, sh.Stderr = &o, &e
		code := driver.MainArgs(sh, append([]string{"dash"}, c.args...))
		if o.Len() > 0 || e.String() != c.want || code != 2 {
			t.Errorf("%q: %q, %q at %d; want %q at 2", c.args, o.String(), e.String(), code, c.want)
		}
	}
}
