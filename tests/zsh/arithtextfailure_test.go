// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFailedExpansionInAnArithmeticCommandIsTheConstructs: the construct
// catches it, leaves its own 2 and the line carries on — and what was left of
// the text is not evaluated, so nothing is stored. Measured 2026-10-31 on zsh
// 5.9.2 under `-c`. The expansion `$(( ))` stops the same way and ends the
// shell, as any failed expansion in a command's words does here. See
// interp.Semantics.FailedExpansionEscapesAnArithmeticCommand (#7189).
func TestAFailedExpansionInAnArithmeticCommandIsTheConstructs(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		src, want string
		status    int
	}{
		{"(( $((1/0)) + 1 )); echo \"same=$?\"\necho \"next=$?\"", "zsh:1: division by zero\nsame=2\nnext=0\n", 0},
		{"(( b = $((1/0)) + 1 )); echo \"same=$? b=$b\"", "zsh:1: division by zero\nsame=2 b=\n", 0},
		{"echo $(( b = 15 $((1/0)) )); echo \"b=$b\"", "zsh:1: division by zero\n", 1},
		// And the subscript on a name holding nothing runs once, as it
		// already did here: this column never asked the reach.
		{`i=0; : ${x[i++]}; echo $i`, "1\n", 0},
	} {
		out, st := runZsh(t, t.TempDir(), c.src)
		if out != c.want || st != c.status {
			t.Errorf("%s\n got %q (status %d)\nwant %q (status %d)", c.src, out, st, c.want, c.status)
		}
	}
}
