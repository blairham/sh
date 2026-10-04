// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// `zsh -c` with nothing behind it exits 1, not the usage status 2 the other
// columns use; and `zsh -cecho hi` is refused for the option name the welded
// `o` reads (` hi`) rather than for the missing command string, because the
// options are judged first. Both measured 2026-10-03 on zsh 5.9.2.
func TestAMissingCommandStringIsRefusedAtOneAndAfterTheOptions(t *testing.T) {
	for _, tc := range []struct {
		word, want string
	}{
		{"-c", "zsh: string expected after -c\n"},
		{"-cecho hi", "zsh: no such option:  hi\n"},
	} {
		var o, e bytes.Buffer
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &o, &e
		code := driver.MainArgs(sh, []string{"zsh", tc.word})
		if code != 1 || o.Len() != 0 || e.String() != tc.want {
			t.Errorf("%q: got %q/%q status %d, want %q at 1", tc.word, o.String(), e.String(), code, tc.want)
		}
	}
}
