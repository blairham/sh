// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `@` is not a spelling for the whole array here. The brackets hold an
// arithmetic expression as they do everywhere else, `@` is not one, and the
// array is left exactly as it was — the only shell in the panel that does not
// clear it. Measured against ksh93u+ 2012-08-01 (2026-09-05).
//
// The operand is reported as the bad subscript it is, with the builtin named
// in front of the sentence and the script carrying on — `unset` alone reports
// the failure.
func TestUnsetOfEveryElementIsAnOrdinarySubscript(t *testing.T) {
	for _, sub := range []string{"@", "*"} {
		src := `a=(p q r); unset "a[` + sub + `]"; echo "st=$?"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`
		out, st := runKsh(t, t.TempDir(), src)
		want := "ksh: unset: " + sub + ": arithmetic syntax error\nst=1\n[p][q][r] n=3\n"
		if out != want || st != 0 {
			t.Errorf("[%s] = %q (status %d), want %q", sub, out, st, want)
		}
	}
}

// A scalar keeps its value for the same reason: there is no whole-array
// reading to reach it.
func TestUnsetOfEveryElementLeavesAScalarAlone(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=hello; unset "a[@]"; echo "[$a]"`)
	want := "ksh: unset: @: arithmetic syntax error\n[hello]\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// A subscript that *is* an expression names its element and the element goes
// away, so unsetting the last one shortens the array — the reading this shell
// shares with bash and not with zsh, reached through the same field.
// Measured against ksh93 93u+ 2012-08-01 (2026-09-05).
func TestUnsetOfTheLastElementRemovesIt(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`a=(x y z); unset "a[2]"; printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if out != "[x][y] n=2\n" || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, "[x][y] n=2\n")
	}
}
