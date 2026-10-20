// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An external command's assignment prefix is expanded in the child that runs
// it, so a side effect in a value never reaches the shell. Measured
// 2026-10-16 on ksh93u+ under `-c` — see
// Semantics.PrefixExpandsInTheExternalCommandsChild (#6784).
func TestAnExternalCommandsPrefixExpandsInItsChild(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`i=1; y=$((i++)) /usr/bin/true; echo $i`, "1\n"},
		{`unset i; y=${i:=5} /usr/bin/true; echo "[$i]"`, "[]\n"},
		{`i=1; y=$((i++)) true; echo $i`, "2\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
