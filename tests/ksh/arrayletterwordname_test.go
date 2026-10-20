// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// An option word with more written behind its `a` is the first operand here,
// refused as a name, and the refusal ends the script. Measured 2026-10-03 on
// ksh93u+ 2012-08-01; see Semantics.ArrayLetterMakesItsWordAName.
func TestAnOptionWordWithMoreBehindItsArrayLetterIsAName(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -aU q=(1 2); echo after", "typeset: -aU: invalid variable name\n"},
		{"typeset -ai q=(1 2); echo after", "typeset: -ai: invalid variable name\n"},
		{"typeset -iaU q; echo after", "typeset: -iaU: invalid variable name\n"},
		{"typeset -aL 3 q=(1 2); echo after", "typeset: -aL: invalid variable name\n"},
		{"typeset -ia q=(1 2); typeset -p q", "typeset -a -i q=(1 2)\n"},
		{"typeset -ra q; echo after", "after\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}
