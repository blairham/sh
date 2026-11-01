// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A builtin's complaint about how it was called is an error condition to an
// `always` block, the same as any other error the shell reported and gave up
// over (#6668).
//
// Measured 2026-10-16 on zsh 5.9.2 under `-f`, by `-c` and on standard input:
// a refused `set -o nosuch` or `set -Z` in the try half still runs the always
// half, which reads `$?` as 1 and TRY_BLOCK_ERROR as 1, and the shell ends at
// 1 after it; in the always half the same refusal is discarded like any other
// error and the line after the construct runs at 0. This shell skipped the
// always half at the top level, because the refusal is a usage give-up and
// only an ordinary one was counted as an error there.
//
// `shift 5` is the control: zsh survives it, so it was never in question.
func TestAUsageRefusalIsAnErrorConditionToAnAlwaysBlock(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src, want  string
		wantStatus int
	}{
		{"{ true; set -o nosuch } always { echo in=$? }; echo after\n", "in=1\n", 1},
		{"{ true; set -Z } always { echo in=$? }; echo after\n", "in=1\n", 1},
		{"{ true; set -o nosuch } always { echo TBE=$TRY_BLOCK_ERROR; TRY_BLOCK_ERROR=0 }; echo after $?\n", "TBE=1\nafter 1\n", 0},
		{"{ true } always { set -o nosuch; echo in }; echo after $?\n", "after 0\n", 0},
		{"{ true; shift 5 } always { echo in=$? }; echo after\n", "in=1\nafter\n", 0},
	} {
		out, st, _ := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
		if out != c.want || st != c.wantStatus {
			t.Errorf("%q: wrote %q at %d, want %q at %d", c.src, out, st, c.want, c.wantStatus)
		}
	}
}
