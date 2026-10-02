// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `)` left over after a complete expression closes nothing, and is worded
// as that. See syntax.ErrArithUnmatchedCloseParen.
//
// Measured 2026-10-02 on zsh 5.9.2, `-f -c` (#5145).
func TestAStrayCloseParenInArithmetic(t *testing.T) {
	out, _, errs := runZshUTF8(t, `foo="3)"; (( foo )); print st=$?`)
	if out != "st=2\n" || errs != "zsh:1: bad math expression: unexpected ')'\n" {
		t.Errorf("got %q, %q", out, errs)
	}
}
