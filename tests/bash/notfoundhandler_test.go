// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestACommandNotFoundGoesToTheHandler is bash's
// interp.Semantics.CommandNotFoundHandler: `command_not_found_handle`, run as
// a subshell with the word and its arguments, its status the command's.
// Measured 2026-10-02 on bash 5.3.20.
func TestACommandNotFoundGoesToTheHandler(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		`command_not_found_handle() { x=1; echo "1=$1 #=$#"; return 7; }; nosuch 1 2; echo "st=$? x=$x"`+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "1=nosuch #=3\nst=7 x=\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
