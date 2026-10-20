// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// The other side of the same reading, and the one that says `let` is not
// taking the arithmetic command's answer either: this shell ends a script for
// `readonly x=1; (( x = 2 ))` and does not for the same write through `let`.
//
// Measured 2026-09-18 against ksh93u+ 2012-08-01 over a script file, `env -i`
// with LC_ALL=C (#3470).
func TestALetRefusedByAFreezeIsNotFatalWhereTheArithmeticCommandIs(t *testing.T) {
	out, st := answersRun(t, "readonly x=1\nlet x=2\necho after $?\n")
	if !strings.Contains(out, "after 1") {
		t.Errorf("let: got %q at %d, want the next line to run and report 1", out, st)
	}
	out, _ = answersRun(t, "readonly x=1\n(( x = 2 ))\necho after $?\n")
	if strings.Contains(out, "after") {
		t.Errorf("(( )): got %q, want the script over", out)
	}
}
