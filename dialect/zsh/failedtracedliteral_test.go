// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A table literal that fails partway under `set -x` stores nothing here and
// the table keeps what it held, as it does untraced. Measured 2026-10-15 on
// zsh 5.9.2 from a script file: `typeset -A a; a=([o]=0); set -x;
// a=([k]=1 [x]=$((1/0)))` is `division by zero` and the table is still
// `( [o]=0 )`. The trace expands the elements ahead of the store, and the
// part it read before the failure must not reach the store as a list that
// succeeded (#6683).
func TestAFailedTableLiteralStoresNothingUnderTracing(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"typeset -A a; a=([o]=0)\ntrap 'set +x; typeset -p a' EXIT\nset -x\na=([k]=1 [x]=$((1/0)))")
	if !strings.HasSuffix(out, "typeset -A a=( [o]=0 )\n") {
		t.Errorf("got %q, want the table left `( [o]=0 )`", out)
	}
}

// `[*]=` written as a table literal's key is an ordinary key here, where the
// same spelling on the left of an assignment is the slice refusal. Measured
// 2026-10-15 on zsh 5.9.2 from a script file. See
// interp.Semantics.WholeArraySubscriptKeyInATableLiteral (#6686).
func TestAWholeArrayKeyInATableLiteralIsAnOrdinaryKey(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "typeset -A a; a=([k]=1 [*]=s [@]=t); typeset -p a")
	if want := "typeset -A a=( ['*']=s [@]=t [k]=1 )\n"; out != want || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, want)
	}
}
