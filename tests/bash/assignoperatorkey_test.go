// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestAnAssignOrErrorOperatorTreatsAMissingKeyAsUnset: `${a[k]=x}` on a key
// the table has not got assigns it and `${a[k]?m}` refuses it, as on any unset
// parameter — the operator makes no key of its own first (#7187). Measured
// 2026-10-31 on bash 5.3.20, `-c`.
func TestAnAssignOrErrorOperatorTreatsAMissingKeyAsUnset(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `declare -A a; echo "<${a[k]=x}>"; declare -p a`)
	if want := "<x>\ndeclare -A a=([k]=\"x\" )\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
	out, st = runBash(t, t.TempDir(), "trap 'declare -p a' EXIT; declare -A a; : ${a[k]?m}; echo not here")
	if !strings.Contains(out, "a[k]: m\n") || strings.Contains(out, "not here") || st == 0 {
		t.Errorf("got %q status %d, want the refusal ending the shell", out, st)
	}
	if !strings.HasSuffix(out, "declare -A a\n") {
		t.Errorf("got %q, want the table left as it was", out)
	}
}
