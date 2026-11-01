// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `)` left over after a complete expression closes nothing, and is worded
// as that. See syntax.ErrArithUnmatchedCloseParen.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f -c` (#5145).
func TestAStrayCloseParenInArithmetic(t *testing.T) {
	t.Parallel()
	out, _, errs := runZshUTF8(t, `foo="3)"; (( foo )); print st=$?`)
	if out != "st=2\n" || errs != "zsh:1: bad math expression: unexpected ')'\n" {
		t.Errorf("got %q, %q", out, errs)
	}
}

// A byte the reader refuses outright, met where a group's `)` belongs, is the
// `)` that was expected; an operand there keeps `operator expected`. Outside a
// group the same byte is `illegal character`.
//
// Measured 2026-10-16 on zsh 5.9.2, from a file (#6803).
func TestARefusedByteInAGroupWantsTheCloseParen(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ expr, want string }{
		{`(1;2)`, "bad math expression: ')' expected"},
		{`((1) ;)`, "bad math expression: ')' expected"},
		{`(1 a)`, "bad math expression: operator expected at `a) '"},
		{`(1 ~)`, "bad math expression: operator expected at `~) '"},
		{`1;2`, "bad math expression: illegal character: ;"},
	} {
		_, _, errs := runZshUTF8(t, `print $(( `+tc.expr+` ))`)
		if errs != "zsh:1: "+tc.want+"\n" {
			t.Errorf("%s: got %q, want %q", tc.expr, errs, tc.want)
		}
	}
}
