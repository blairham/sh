// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestAnAssignOrErrorOperatorTreatsAMissingKeyAsUnset: `${a[k]=x}` on a key
// the table has not got assigns it and `${a[k]?m}` refuses it, as on any unset
// parameter — the operator makes no key of its own first (#7187). Measured
// 2026-10-31 on zsh 5.9.2, `-c`.
func TestAnAssignOrErrorOperatorTreatsAMissingKeyAsUnset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -A a; echo "<${a[k]=x}>"; typeset -p a`)
	if want := "<x>\ntypeset -A a=( [k]=x )\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
	out, st = runZsh(t, t.TempDir(), "trap 'typeset -p a' EXIT; typeset -A a; : ${a[k]?m}; echo not here")
	if !strings.Contains(out, "a[k]: m\n") || strings.Contains(out, "not here") || st == 0 {
		t.Errorf("got %q status %d, want the refusal ending the shell", out, st)
	}
	if !strings.HasSuffix(out, "typeset -A a=( )\n") {
		t.Errorf("got %q, want the table left as it was", out)
	}
}
