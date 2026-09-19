// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The scan that decides what a `$((` opens is blind to quoting here too, so a
// `)` written inside quotes ends the count and what is left is a command
// substitution holding a subshell — which runs a command named `0)`.
//
// Measured 2026-09-18, each probe in a script file of its own under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	echo $(( '0)' + 1 ))   a command named `0)` is looked for and not found
//	echo $(( "0)" + 1 ))   the same
//
// The three bash columns call each an arithmetic error at 1. It is a second
// field beside the one the `((` scan reads: the two were measured separately
// and agree today by measurement rather than by construction (#3530).
func TestTheArithSubstScanIsBlindToQuoting(t *testing.T) {
	if !zsh.Dialect().ArithSubstScanIgnoresQuoting {
		t.Fatal("this dialect's `$((` scan does not see quoting")
	}
	for _, src := range []string{`echo $(( '0)' + 1 ))`, `echo $(( "0)" + 1 ))`} {
		out, st, _ := preset.Combined(t, dialecttest.Base{}, src)
		if !strings.Contains(out, "0)") {
			// The status is the `echo`'s and is 0 in both columns, which is
			// half of why the reading matters: the substitution's failure is
			// a sentence on standard error and nothing downstream sees it.
			t.Errorf("%q: out = %q at %d, want the command named `0)` looked for", src, out, st)
		}
		if strings.Contains(out, "math") || strings.Contains(out, "arithmetic") {
			t.Errorf("%q: out = %q, want no arithmetic complaint — the reading was given up", src, out)
		}
	}
	// And an expression with no quoting in it is arithmetic as ever.
	if out, st, _ := preset.Combined(t, dialecttest.Base{}, `echo $(( 1 + 2 ))`); strings.TrimSpace(out) != "3" || st != 0 {
		t.Errorf("an ordinary expression: %q at %d, want 3 at 0", out, st)
	}
}
