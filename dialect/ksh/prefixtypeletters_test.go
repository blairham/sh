// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAPrefixToAProgramIsFoldedThroughTheNamesLetters — the No side of
// interp.Semantics.AssignmentPrefixMakesAFreshCell, at the one route that
// makes no store in this shell: a program. The store is the child's and keeps
// the letters of the binding it lands on, the field widths included, since
// ksh93 pads on the store (#6698). ksh93u+ 2012-08-01 `-c`, measured
// 2026-10-15.
func TestAPrefixToAProgramIsFoldedThroughTheNamesLetters(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"integer", `typeset -i v=3; v=2+2 /usr/bin/printenv v; echo "[$v]"`, "4\n[3]\n"},
		{"integer over text", `typeset -i v; v=pre /usr/bin/printenv v`, "0\n"},
		{"a base", `typeset -i 16 v; v=20 /usr/bin/printenv v`, "16#14\n"},
		{"a hexadecimal value", `typeset -i v; v=0x10 /usr/bin/printenv v`, "16\n"},
		{"a based value", `typeset -i v; v=8#17 /usr/bin/printenv v`, "15\n"},
		{"float", `typeset -F 2 v; v=1/3 /usr/bin/printenv v`, "0.00\n"},
		{"upper", `typeset -u v; v=abc /usr/bin/printenv v`, "ABC\n"},
		{"right", `typeset -R5 v; v=ab /usr/bin/printenv v`, "   ab\n"},
		{"left", `typeset -L5 v; v=ab /usr/bin/printenv v`, "ab   \n"},
		{"zero", `typeset -Z5 v; v=42 /usr/bin/printenv v`, "00042\n"},
		{"the entry behind sees the fold", `typeset -i v; v=2+2 w=$v /usr/bin/printenv w`, "4\n"},
		{"a plain name is its text", `v=2+2 /usr/bin/printenv v`, "2+2\n"},
		// The {v} store retypes the name before the environment is built,
		// an exported scalar included.
		{"a fresh {v}", `v=pre /usr/bin/printenv v {v}</dev/null`, "0\n"},
		{"an exported scalar {v}", `export v=keep; v=pre /usr/bin/printenv v {v}</dev/null`, "0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAPrefixToAProgramThatWillNotEvaluateIsTheCommandsFailure — the integer
// letter refusing the value is the command's failure, as a failed expansion in
// the same place is: nothing behind it expands, the status is 1 and catchable,
// and the script goes on (#6698). ksh93u+ 2012-08-01 `-c`, measured
// 2026-10-15.
func TestAPrefixToAProgramThatWillNotEvaluateIsTheCommandsFailure(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`typeset -i v; v=1+ w=$(echo SIDE >&2) /usr/bin/printenv v || echo "CAUGHT $?"; echo next`)
	want := "ksh: 1+: more tokens expected\nCAUGHT 1\nnext\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", out, st, want)
	}
}
