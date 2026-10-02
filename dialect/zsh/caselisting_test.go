// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// TestACaseArmIsListedAsItWasRead: the alternatives of an arm are listed with
// a blank either side of each `|`, except a list written inside the arm's
// parenthesis, which this shell reads as one pattern and lists as it read it
// — blanks dropped. Under `emulate sh` the parenthesis is only the arm's and
// the list is spaced like any other. Measured 2026-10-02 on zsh 5.9.2 through
// `which`, the A01grammar chunk (#5138).
func TestACaseArmIsListedAsItWasRead(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `f() { case $1 in (one|two) :;; ( a | b ) :;; c|d) :;; (h|(i|j)) :;; esac }; which f
emulate sh -c 'g() { case $1 in ( one | two ) :;; esac }'; which g`)
	for _, want := range []string{
		"\t\t(one|two) : ;;\n",
		"\t\t(a|b) : ;;\n",
		"\t\t(c | d) : ;;\n",
		"\t\t(h|(i|j)) : ;;\n",
		"\t\t(one | two) : ;;\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("listing %q lacks %q", out, want)
		}
	}
}

// TestAReservedDeclarationHoldingAnAssignmentIsListedWithABlank: a
// reserved-word declaration whose operands hold an assignment comes back with
// one blank at its end, wherever the assignment stood. Measured 2026-10-02 on
// zsh 5.9.2 through `which` (#5138), the A01grammar `typeset ac_file=…` row.
func TestAReservedDeclarationHoldingAnAssignmentIsListedWithABlank(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset a="x y"`, "\ttypeset a=\"x y\" \n"},
		{`typeset b=1 c`, "\ttypeset b=1 c \n"},
		{`typeset c b=1`, "\ttypeset c b=1 \n"},
		{`integer k=6`, "\tinteger k=6 \n"},
		{`typeset a[1]=2`, "\ttypeset a[1]=2 \n"},
		{`typeset a+=3`, "\ttypeset a+=3 \n"},
		{`typeset i=(1) c`, "\ttypeset i=(1) c \n"},
		{`typeset o=1 >/dev/null`, "\ttypeset o=1  > /dev/null\n"},
		// None: no assignment, one the grammar does not read as one, a
		// declaration that is not the reserved word, and `private`, which
		// takes an array but is a builtin.
		{`typeset c d`, "\ttypeset c d\n"},
		{`typeset "a=1"`, "\ttypeset \"a=1\"\n"},
		{`typeset $x=1`, "\ttypeset $x=1\n"},
		{`builtin typeset a=1`, "\tbuiltin typeset a=1\n"},
		{`private q=(1)`, "\tprivate q=(1)\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), "f() { "+tc.src+" }; which f")
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s listed %q, want %q in it", tc.src, out, tc.want)
		}
	}
}
