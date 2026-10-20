// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// The other side of Semantics.ReadArrayTakesOneNameOnly: this shell reads into
// the first name and **clears** the operands behind it, where the other shell
// with the `-A` letter declines the line. Measured 2026-09-26 on ksh93u+
// 2012-08-01 from a script file (#4622).
//
// Here as well as in dialect/zsh because an axis with one side measured is an
// axis half-pinned: a change that made the refusal unconditional would pass
// every test over there.
func TestReadArrayReadsPastASecondOperandHere(t *testing.T) {
	if got := ksh.Semantics().ReadArrayTakesOneNameOnly; got != interp.No {
		t.Errorf("ReadArrayTakesOneNameOnly = %v, want No", got)
	}
	out, st := runKsh(t, t.TempDir(), `b=keep; c=keep
read -A a b c <<<'1 2 3 4'
print "A st=$? a=(${a[@]}) b=[$b] c=[$c]"`)
	want := "A st=0 a=(1 2 3 4) b=[] c=[]\n"
	if out != want || st != 0 {
		t.Errorf("read -A with a second operand = %q (status %d), want %q", out, st, want)
	}
}
