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

// A trap body that will not parse is placed at the line the trap fired on,
// as every other line of a body is here, and not at the body line it gave out
// on. Measured 2026-10-04 on BusyBox ash 1.37.0 in the pinned Alpine image
// over script files (#5723). See Semantics.TrapBodyLine.
func TestATrapBodyThatWillNotParseIsPlacedWhereItFired(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a signal", "echo one\ntrap 'echo a\nif' USR1\necho two\nkill -USR1 $$\necho three\n",
			`s.sh: line 5: syntax error: unexpected end of file (expecting "then")`,
		},
		{
			"exit after the last line", "echo one\ntrap 'echo a\nif' EXIT\necho end",
			`s.sh: line 4: syntax error: unexpected end of file (expecting "then")`,
		},
		{
			"exit from one line down", "trap 'if' EXIT\necho after",
			`s.sh: line 2: syntax error: unexpected end of file (expecting "then")`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "s.sh")
			if err := os.WriteFile(path, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			driver.MainArgs(sh, []string{"ash", path})
			got := strings.ReplaceAll(e.String(), dir+"/", "")
			if !strings.Contains(got, tc.want) {
				t.Errorf("stderr = %q, want %q in it", got, tc.want)
			}
		})
	}
}
