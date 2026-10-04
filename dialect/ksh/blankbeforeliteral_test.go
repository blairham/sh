// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `a= (x y)` is the array literal here, blanks and all, wherever an
// assignment stands; a newline is not a blank. Measured 2026-10-03 on
// ksh93u+; see syntax.Dialect.ArrayLiteralAfterABlank.
func TestABlankMayStandBeforeAnArrayLiteral(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a= (echo x); echo "n=${#a[@]} [${a[*]}]"`, "n=2 [echo x]\n"},
		{`a+= (x); echo "[${a[*]}]"`, "[x]\n"},
		{`typeset a= (x); echo "[${a[*]}]"`, "[x]\n"},
		{`b=1 a= (x); echo "[${a[*]}][$b]"`, "[x][1]\n"},
		{"a=\n(echo sub); echo \"[${a[*]}]\"", "sub\n[]\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}
