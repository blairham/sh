// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `@` is not a spelling for the whole array here. The brackets hold an
// arithmetic expression as they do everywhere else, `@` is not one, and the
// array is left exactly as it was — the only shell in the panel that does not
// clear it. Measured against ksh93u+ 2012-08-01 (2026-09-05).
//
// The shell reports the operand where this is still quiet, which is #649's
// question and not this one; what is asserted here is that the array survives.
func TestUnsetOfEveryElementIsAnOrdinarySubscript(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		src := `a=(p q r); unset "a[` + sub + `]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		out, st := runKsh(t, t.TempDir(), src)
		if out != "[p][q][r] n=3\n" || st != 0 {
			t.Errorf("[%s] = %q (status %d), want %q", sub, out, st, "[p][q][r] n=3\n")
		}
	}
}

// A scalar keeps its value for the same reason: there is no whole-array
// reading to reach it.
func TestUnsetOfEveryElementLeavesAScalarAlone(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=hello; unset "a[@]"; echo "[$a]"`)
	if out != "[hello]\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[hello]\n")
	}
}
