// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// whence and which remember what they look up, as type and command -v do,
// and whence -m fills the table the way a read of $commands does (#6111).
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under -f with
// PATH one directory holding `qq` and `r2`: each verb below leaves the
// listing shown, and whence -a — the control — leaves nothing.
func TestAWhenceLookupIsRemembered(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "b2")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"qq", "r2"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ verb, want string }{
		{"whence qq", "qq=" + bin + "/qq\n"},
		{"whence -v qq", "qq=" + bin + "/qq\n"},
		{"which qq", "qq=" + bin + "/qq\n"},
		{"whence -p qq", "qq=" + bin + "/qq\n"},
		{"whence -m qq", "qq=" + bin + "/qq\nr2=" + bin + "/r2\n"},
		{"whence -a qq", ""},
	} {
		t.Run(c.verb, func(t *testing.T) {
			out, _ := runZshOnPath(t, dir, "PATH="+bin+"; "+c.verb+" >/dev/null; hash")
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
