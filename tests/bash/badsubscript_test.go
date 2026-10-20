// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A subscript that will not read is blamed on the expression alone, and a
// substring's range puts the parameter in front of it. Measured against bash
// 5.3.15 (2026-09-05).
func TestABadSubscriptNamesTheExpression(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(x y z); echo "[${a[b c]}]"; echo after`)
	want := "bash: line 1: b c: arithmetic syntax error in expression (error token is \"c\")\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}

func TestABadSubstringRangeNamesTheParameter(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`x=abcdef; echo "[${x:1+:2}]"`, "bash: line 1: x: 1+: arithmetic syntax error: operand expected (error token is \"+\")\n"},
		// The parameter is the name and its subscript, not the name alone.
		{`a=(p q r); echo "[${a[@]:1+}]"`, "bash: line 1: a[@]: 1+: arithmetic syntax error: operand expected (error token is \"+\")\n"},
	} {
		out, st := runBash(t, t.TempDir(), c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s = %q (status %d), want %q at 1", c.src, out, st, c.want)
		}
	}
}

// `unset` gives up on the script here, as a bad expression does anywhere else,
// so nothing after it runs.
func TestABadSubscriptToUnsetEndsTheScript(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(x y z); unset "a[1+]"; echo after`)
	want := "bash: line 1: 1+: arithmetic syntax error: operand expected (error token is \"+\")\n"
	if out != want || st != 1 {
		t.Errorf("got %q (status %d), want %q at 1", out, st, want)
	}
}
