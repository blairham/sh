// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `ulimit -N <number>` is this shell's own option and nothing else in the
// panel has a two-token one here. It exists because this shell prints a row
// its letters do not cover — `-N 15: rt cpu time (microseconds)` — and the
// number is the **kernel's**, which is what `ulimit -N 7` says: 10666
// processes on macOS and 1024 open files on Linux, out of the same shell.
//
// Measured 2026-09-18 on zsh 5.9.2 (macOS arm64) and zsh 5.9 in the pinned
// Alpine image (linux/arm64), each probe a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`. Ours now agrees on every row of both columns
// except two the kernel makes up — `ulimit -N 99` prints 8176 there and 8192
// on Linux, and `ulimit -N -1` prints `unlimited` on both, which is a read
// past the end of the table rather than an answer (#3667).
func TestTheNumberedUlimitOptionIsThisShellsOwn(t *testing.T) {
	got := zsh.Diagnostics().UlimitNumberedOption
	if got.Letter != 'N' {
		t.Errorf("letter is %q, want N", got.Letter)
	}
	for _, c := range []struct{ name, got, want string }{
		{"NeedsNumber", got.NeedsNumber, "number required after -%[1]s"},
		{"BadNumber", got.BadNumber, "invalid number: %[1]s"},
		{"OutOfRange", got.OutOfRange, "can't read limit: invalid argument"},
	} {
		if c.got != c.want {
			t.Errorf("%s is %q, want %q", c.name, c.got, c.want)
		}
	}
	// The row the option exists for has no letter of its own, which is what
	// made it unreachable: `ulimit -a` prints it and `ulimit -N 15` is the
	// only way to read it.
	var found bool
	for _, row := range zsh.Diagnostics().UlimitListing {
		if row.Letter == 0 {
			found = true
			if row.Res == 0 {
				t.Errorf("the letterless row names no resource: %+v", row)
			}
		}
	}
	if !found {
		t.Error("no letterless row in the table, so the option has nothing to reach")
	}
	// And no other dialect grows one by inheritance: the zero value is no
	// option at all, which is what the other four columns have.
	if zsh.Diagnostics().UlimitBadOptionStatus != 1 {
		t.Errorf("the refusals ride on the builtin's own status, which is %d",
			zsh.Diagnostics().UlimitBadOptionStatus)
	}
}
