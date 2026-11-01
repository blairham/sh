// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An external command's assignment prefix is expanded in the child that runs
// it: a side effect in a value stays there, and a subscript the literal
// refuses costs the child. Measured 2026-10-16 on zsh 5.9.2, `-f`, under `env
// -i PATH=/usr/bin:/bin` — see Semantics.PrefixExpandsInTheExternalCommandsChild
// (#6784, #6783).
func TestAnExternalCommandsPrefixExpandsInItsChild(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`i=1; y=$((i++)) /usr/bin/true; echo $i`, "1\n"},
		{`unset i; y=${i::=5} /usr/bin/true; echo "[$i]"`, "[]\n"},
		{`i=1; x=(${$((i++))}) /usr/bin/true; echo $i`, "1\n"},
		{`i=1; x=([i++]=a) /usr/bin/true; echo $i`, "1\n"},
		{`x=([0]=a) /usr/bin/true; echo after $?`, "zsh:1: bad subscript for direct array assignment: 0\nafter 1\n"},
		{`x=([1/0]=a) /usr/bin/true; echo after $?`, "zsh:1: division by zero\nafter 1\n"},
		// The entries behind still see it, in the child.
		{`i=1; x=([i++]=a) y=$i:${x[1]} /usr/bin/printenv y; echo $i`, "2:a\n1\n"},
		// The control: a builtin's prefix is expanded in this shell.
		{`i=1; y=$((i++)) true; echo $i`, "2\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
