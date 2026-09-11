// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A feature `zmodload -F` leaves off is taken out of the builtin table, and
// one it puts back is put back.
//
// Measured on zsh 5.9.2, 2026-09-10, and every line below is byte-identical
// in the two shells:
//
//	zmodload zsh/zutil
//	zmodload -F zsh/zutil -b:zparseopts
//	zparseopts -D a:     zsh:1: command not found: zparseopts   127
//	zstyle -s x y z                                             1
//
// Recording the selection was not enough: `-lF` reported it truthfully and
// every builtin stayed callable, so a script that narrowed a module went on
// using what it had just switched off (#1635).
//
// Each case asserts the **status** as well as the sentence, because the two
// refusals here are one digit apart in meaning and identical in shape: 127 is
// the name resolving to nothing, and 1 is the builtin running and complaining
// about its operands. A test matching only "there was a diagnostic" would
// pass for a shell that never withdrew anything.
func TestAFeatureLeftOffIsNotCallable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the one left off",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzparseopts -D a:\nprint \"st=$?\"",
			"st=127",
		},
		{
			"the ones left on still answer",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzstyle -s x y z\nprint \"st=$?\"",
			"st=1",
		},
		{
			"and the sign puts it back",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\n" +
				"zmodload -F zsh/zutil +b:zparseopts\nzparseopts -D a:\nprint \"st=$?\"",
			"st=1",
		},
		{
			"so does a plain load of the module",
			"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\n" +
				"zmodload zsh/zutil\nzparseopts -D a:\nprint \"st=$?\"",
			"st=1",
		},
		{
			"a selection in a subshell does not reach out of it",
			"zmodload zsh/zutil\n( zmodload -F zsh/zutil -b:zparseopts; zparseopts -D a: )\n" +
				"print \"in=$?\"\nzparseopts -D a:\nprint \"out=$?\"",
			"in=127",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runZshSplit(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s\n = %q, want %q in it", tc.src, out, tc.want)
			}
		})
	}
	// The other half of the last one, kept apart so a failure names which
	// direction leaked.
	out, _, _ := runZshSplit(t, t.TempDir(),
		"zmodload zsh/zutil\n( zmodload -F zsh/zutil -b:zparseopts )\nzparseopts -D a:\nprint \"out=$?\"")
	if !strings.Contains(out, "out=1") {
		t.Errorf("got %q, want the parent to still have the builtin", out)
	}
}

// Withdrawing is not `disable`: that shell lists nothing afterwards and
// answers `enable zparseopts` with a hash-table complaint rather than putting
// the builtin back. Measured, and the reason `SetBuiltinEnabled` was the
// wrong seam for this.
func TestAWithdrawnFeatureIsNotADisabledBuiltin(t *testing.T) {
	out, _, _ := runZshSplit(t, t.TempDir(),
		"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\ndisable\nprint \"dis=$?\"")
	if strings.Contains(out, "zparseopts") || !strings.Contains(out, "dis=0") {
		t.Errorf("got %q, want an empty listing at 0", out)
	}
}

// `-lF` still reports the selection it always did: the listing and the table
// are two halves of one answer, and the half that was already right must not
// move.
func TestTheFeatureListingStillReportsTheSelection(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"zmodload zsh/zutil\nzmodload -F zsh/zutil -b:zparseopts\nzmodload -lF zsh/zutil")
	const want = "+b:zformat\n-b:zparseopts\n+b:zregexparse\n+b:zstyle\n"
	if out != want || st != 0 {
		t.Errorf("listing = %q (status %d), want %q at 0", out, st, want)
	}
}
