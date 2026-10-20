// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A refused letter takes the whole declaration with it here, array literals
// included. Measured 2026-10-03 on zsh 5.9.2: `typeset -Q a=(1)` is `bad
// option: -Q` at 1 and `typeset -p a` then says `no such variable: a`. See
// Semantics.RefusedDeclarationKeepsItsArrayLiterals, which bash answers the
// other way.
func TestARefusedLetterTakesTheArrayLiteralsWithIt(t *testing.T) {
	out, _ := answersRun(t, `typeset -Q a=(1) 2>/dev/null; echo "st=$? [${a-unset}]"`)
	if !strings.HasSuffix(out, "st=1 [unset]\n") {
		t.Errorf("= %q, want it to end %q", out, "st=1 [unset]\n")
	}
}
