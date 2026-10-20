// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// Every spelling of whence leaves the name tracked, -a and -p included
// (#6111). Measured 2026-10-05 on ksh93u+ (/bin/ksh): with PATH one
// directory holding `qq`, `whence -a qq`, `whence -p qq` and `whence -pa
// qq` each list `qq=<dir>/qq` from `hash` afterwards, as `whence qq` does.
// The `hash` after nothing is the control.
func TestEveryWhenceTracksTheName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "qq"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ verb, want string }{
		{"whence -a qq", "qq=" + dir + "/qq\n"},
		{"whence -p qq", "qq=" + dir + "/qq\n"},
		{"whence -pa qq", "qq=" + dir + "/qq\n"},
		{":", ""},
	} {
		t.Run(c.verb, func(t *testing.T) {
			out, _ := runKsh(t, dir, c.verb+" >/dev/null; hash")
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
