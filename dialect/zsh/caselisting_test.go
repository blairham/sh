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
