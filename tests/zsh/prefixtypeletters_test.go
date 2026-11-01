// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAPrefixToAProgramIsFoldedThroughTheNamesLetters — the No side of
// interp.Semantics.AssignmentPrefixMakesAFreshCell, at the one route that
// makes no store in this shell: a program. The store is the child's, and it
// keeps the letters of the binding it lands on, so the program is handed the
// folded value — and a field width, which zsh applies on the read and not to
// the environment, is the one letter that does not show (#6698). zsh 5.9.2
// `-fc`, measured 2026-10-15.
func TestAPrefixToAProgramIsFoldedThroughTheNamesLetters(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"integer", `typeset -i v=3; v=2+2 /usr/bin/printenv v; echo "[$v]"`, "4\n[3]\n"},
		{"integer over text", `typeset -i v; v=pre /usr/bin/printenv v`, "0\n"},
		{"a base", `typeset -i 16 v; v=20 /usr/bin/printenv v`, "16#14\n"},
		{"a base the value teaches is the child's", `typeset -i v; v=0x10 /usr/bin/printenv v; v=5; echo "[$v]"`, "16#10\n[5]\n"},
		{"float", `typeset -F 2 v; v=1/3 /usr/bin/printenv v`, "0.00\n"},
		{"upper", `typeset -u v; v=abc /usr/bin/printenv v`, "ABC\n"},
		{"lower", `typeset -l v; v=ABC /usr/bin/printenv v`, "abc\n"},
		{"a width is not padded", `typeset -R5 v; v=ab /usr/bin/printenv v`, "ab\n"},
		{"exported", `typeset -xi v=1; v=2+2 /usr/bin/printenv v; /usr/bin/printenv v`, "4\n1\n"},
		{"the entry behind sees the fold", `typeset -i v; v=2+2 w=$v /usr/bin/printenv w`, "4\n"},
		{"a plain name is its text", `v=2+2 /usr/bin/printenv v`, "2+2\n"},
		// The {v} store retypes a fresh name before the environment is built,
		// and leaves an exported scalar a scalar.
		{"a fresh {v}", `v=pre /usr/bin/printenv v {v}</dev/null`, "0\n"},
		{"an exported scalar {v}", `export v=keep; v=pre /usr/bin/printenv v {v}</dev/null`, "pre\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAPrefixToAProgramThatWillNotEvaluateIsTheCommandsFailure — a value the
// integer letter cannot read is the arithmetic complaint and status 1 for the
// command alone, as a failed expansion in the same place is: the entries behind
// it are never expanded (`SIDE` is never written), the failure is catchable, and the script goes on
// (#6698). zsh 5.9.2 `-fc`, measured 2026-10-15.
func TestAPrefixToAProgramThatWillNotEvaluateIsTheCommandsFailure(t *testing.T) {
	t.Parallel()
	out, st := runZsh(t, t.TempDir(),
		`typeset -i v; v=1+ w=$(echo SIDE >&2) /usr/bin/printenv v || echo "CAUGHT $?"; echo next`)
	want := "zsh:1: bad math expression: operand expected at end of string\nCAUGHT 1\nnext\n"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", out, st, want)
	}
}

// TestAPrefixAppendJoinsWhatWasAssigned — `+=` in a prefix joins the text the
// name was assigned, as the statement form does, and not what a read of it
// answers: zsh folds the case and pads the width on the read (#6698). zsh
// 5.9.2 `-fc`, measured 2026-10-15.
func TestAPrefixAppendJoinsWhatWasAssigned(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"case, listed", `typeset -u v=ab; v+=cd typeset -p v`, "typeset -u v=abcd\n"},
		{"case, handed to a program", `typeset -u v=ab; v+=cd /usr/bin/printenv v`, "ABCD\n"},
		{"a width", `typeset -R5 v=ab; v+=cd /usr/bin/printenv v`, "abcd\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
