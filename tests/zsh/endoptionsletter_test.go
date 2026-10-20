// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// This shell spends `b` on ending the invocation's option reading, where
// every other column on the panel spends it on the option that reports a
// finished background job at once.
//
// Measured 2026-09-19 with `env -i PATH=/usr/bin:/bin LC_ALL=C` and standard
// input on the null device, `<shell> -b -c 'echo ran'` then
// `<shell> -c 'set -b; echo ran'`:
//
//	bash 5.3.20   `ran` at 0     `ran` at 0
//	bash 3.2.57   `ran` at 0     `ran` at 0
//	ksh93u+       `ran` at 0     `ran` at 0
//	dash 0.5.12   `ran` at 0     `ran` at 0
//	BusyBox ash   `ran` at 0     `ran` at 0
//	zsh 5.9.2     can't open input file: -c, 127    set: bad option: -b
//
// The value is pinned here rather than only where the front end reads it,
// because a letter this preset stopped naming would take the option reading
// back to the shape six other columns have and nothing in driver would
// notice: those tests set the field themselves (#3754).
func TestTheEndOfOptionsLetterIsB(t *testing.T) {
	if got := zsh.Semantics().EndOfOptionsInvocationLetter; got != "b" {
		t.Errorf("EndOfOptionsInvocationLetter = %q, want %q", got, "b")
	}
	// And it is not also an option in this shell's table, which is what the
	// second measured column says: a letter in both places would be read by
	// the front end and then found at `set` too, and the panel's other five
	// columns are exactly that shape.
	out, st := runZshOnPath(t, t.TempDir(), "set -b; echo ran\n")
	if st == 0 || !strings.Contains(out, "-b") {
		t.Errorf("`set -b` gave %q at %d, want it refused by name", out, st)
	}
}
