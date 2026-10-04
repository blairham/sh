// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `ulimit` refuses a limit it cannot read with the operand quoted and the
// shell's name alone in front, from a script as from anywhere. Measured
// 2026-10-04 on BusyBox ash 1.37.0 in the pinned Alpine image: `ulimit -n abc`
// in a script is `ash: invalid number 'abc'` at 1 (#5723).
func TestUlimitNamesTheOperandItCannotRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("ulimit -t abc\necho \"st=$?\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	driver.MainArgs(sh, []string{"ash", path})
	if got, want := e.String(), "ash: invalid number 'abc'\n"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	if !strings.Contains(o.String(), "st=1") {
		t.Errorf("stdout = %q, want st=1", o.String())
	}
}
