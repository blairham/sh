// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A stored value that is not a number is named with its own blanks.
// Measured 2026-10-16 on dash 0.5.12 under `-c` (#6709).
func TestANonNumericValueIsNamedWithItsBlanks(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"x=' 1+ '; echo $((x))", "Illegal number:  1+ \n"},
		{"x='3 4'; echo $((x))", "Illegal number: 3 4\n"},
	} {
		out, _ := runDash(t, t.TempDir(), tc.src)
		if !strings.HasSuffix(out, tc.want) {
			t.Errorf("%s\n got %q\nwant a line ending %q", tc.src, out, tc.want)
		}
	}
}
