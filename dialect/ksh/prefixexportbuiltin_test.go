// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A builtin's prefix takes the export attribute off the name in this column,
// `command` in front of the builtin included, and a name carrying a case
// letter keeps it. Measured 2026-10-16 on ksh93u+ under `-c` (#6859).
func TestABuiltinsPrefixTakesTheExportOffUnlessACaseLetterStands(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -xu v=x; v=abc :; typeset -p v`, "typeset -x -u v=ABC\n"},
		{`typeset -xl v=x; v=abc :; typeset -p v`, "typeset -x -l v=abc\n"},
		{`v=9 command eval '/usr/bin/printenv v || echo none'; echo "[$v]"`, "none\n[9]\n"},
		// The controls: a name with no case letter loses it, and `command`
		// in front of a program hands it the prefix.
		{`typeset -x v=x; v=abc :; typeset -p v`, "v=abc\n"},
		{`v=9 command /usr/bin/printenv v`, "9\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
