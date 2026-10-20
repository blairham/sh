// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell hashes what a `command -p` search resolved, so a later bare name
// runs a program the script's own PATH cannot reach — with nothing in the
// script saying so. The other four columns do not.
//
// Measured 2026-09-15 on the `-c` route and again 2026-09-18 from a script
// file under `env -i`: with `PATH=/nonexistent_zz`, `command -p ls` runs in
// every column and the plain `ls` after it is 0 here and 127 in zsh 5.9.2,
// dash 0.5.12, ksh93u+ and BusyBox ash 1.37.0 (#2975).
func TestADefaultPathSearchGoesIntoTheHash(t *testing.T) {
	const src = `PATH=/nonexistent_zz
command -p ls /dev/null >/dev/null 2>&1; echo "p-run=$?"
ls /dev/null >/dev/null 2>&1; echo "plain-after=$?"`
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "p-run=0\nplain-after=0\n"; out != want || st != 0 {
		t.Errorf("out %q status %d, want %q at 0", out, st, want)
	}
}
