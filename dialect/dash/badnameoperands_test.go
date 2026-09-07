// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A fatal bad name takes the operands *behind* it with it and leaves the ones
// in front standing — the other of the panel's two answers, and the one the
// standard's reading gives: the builtin stops where it failed.
//
// Measured 2026-09-07 from a script file with an EXIT trap, at all three
// positions, because only the middle one can tell the two answers apart.
func TestAFatalBadNameKeepsOnlyTheOperandsInFrontOfIt(t *testing.T) {
	dir := t.TempDir()
	trap := `trap 'echo "[${ok1-U}][${ok2-U}]"' EXIT; `
	for _, tc := range []struct{ src, want string }{
		{`export ":" ok1=1 ok2=2`, "[U][U]\n"},
		{`export ok1=1 ":" ok2=2`, "[1][U]\n"},
		{`export ok1=1 ok2=2 ":"`, "[1][2]\n"},
	} {
		out, st := runDash(t, dir, trap+tc.src+`; echo NOT-FATAL`)
		want := "dash: 1: export: :: bad variable name\n" + tc.want
		if out != want || st != 2 {
			t.Errorf("%s = %q (status %d), want %q at 2", tc.src, out, st, want)
		}
	}
}

// The same rule when the operand it refuses is a *subscripted* one, which
// reaches the refusal by a different door and had a discard of its own:
// `export ok1=1 "a[1]=v" ok2=2` leaves ok1 set and ok2 unset here.
func TestASubscriptedOperandRefusedAmongNamesKeepsTheOnesInFront(t *testing.T) {
	out, st := runDash(t, t.TempDir(),
		`trap 'echo "[${ok1-U}][${ok2-U}]"' EXIT; export ok1=1 "a[1]=v" ok2=2; echo NOT-FATAL`)
	want := "dash: 1: export: a[1]: bad variable name\n[1][U]\n"
	if out != want || st != 2 {
		t.Errorf("a subscripted bad operand = %q (status %d), want %q at 2", out, st, want)
	}
}
