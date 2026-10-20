// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A value that will not evaluate is quoted back with its own blanks.
// Measured 2026-10-16 on ksh93u+ under `-c` (#6709).
func TestAFailedValueIsQuotedWithItsBlanks(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"typeset -i n; n=' 1+ '", ":  1+ : more tokens expected\n"},
		{"typeset -i n; n=$'1+\\n'", ": 1+\n: more tokens expected\n"},
		{"x=' 1/0 '; echo $((x))", ":  1/0 : divide by zero\n"},
		{"x='3 4'; echo $((x))", ": 3 4: arithmetic syntax error\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("%s\n got %q\nwant a line ending %q", tc.src, out, tc.want)
		}
	}
}
