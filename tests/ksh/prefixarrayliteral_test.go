// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAnArrayLiteralPrefixIsAnArray pins #6547: ksh93 stores `x=(a b)` in
// front of a command as an array and hands an external child no `x` at all.
// Measured 2026-10-15 on ksh93u+ from `-c` under `env -i
// PATH=/usr/bin:/bin`.
func TestAnArrayLiteralPrefixIsAnArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"function", `function f { typeset -p x; }; x=(a b) f; typeset -p x`, "typeset -a x=(a b)\n"},
		{"external", `x=(a b) /usr/bin/printenv x; echo rc=$?`, "rc=1\n"},
		{"external over an export", `export x=old; x=(a b) /usr/bin/printenv x; echo rc=$?`, "rc=1\n"},
		{"through command to a builtin", `x=(a b) command eval '/usr/bin/printenv x; echo rc=$?'`, "rc=1\n"},
		{"a substitution", `function f { typeset -p x; }; x=($(echo s >&2) b) f`, "s\ntypeset -a x=(b)\n"},
		{"seen behind it", `x=(a b) y=$x /usr/bin/printenv y`, "a\n"},
		{"the trace", `set -x; x=(a $(echo b)) /usr/bin/true`, "+ /usr/bin/true\n+ echo b\n+ x=( a b )\n"},
		{"seen behind it under a trace", `set -x; x=1 y=$x true`, "+ true\n+ x=1\n+ y=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runKsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// TestAPlacedArrayLiteralPrefixIsSeenBehindIt pins #6636: a prefix literal
// whose elements name their places is seen by the entries behind it — as the
// keyed table ksh93 reads such a literal as, on an external command, a
// function and `eval` alike. Measured 2026-10-15 on ksh93u+ under `env -i
// PATH=/usr/bin:/bin`, `-c` and a script file alike.
func TestAPlacedArrayLiteralPrefixIsSeenBehindIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=([2]=a) y=${x[2]} /usr/bin/printenv y`, "a\n"},
		{`x=([1]=a [3]=c) y="${x[*]}" /usr/bin/printenv y`, "a c\n"},
		{`x=([2]=a [5]=b) y="${!x[*]}" /usr/bin/printenv y`, "2 5\n"},
		{`x=( (1 2) ) y=${x[0][1]} /usr/bin/printenv y`, "2\n"},
		{`function f { echo "[$y]"; }; x=([2]=a) y=${x[2]} f`, "[a]\n"},
		{`x=([2]=a) y=${x[2]} eval 'echo "[$y]"'`, "[a]\n"},
	} {
		if out, _ := runKsh(t, t.TempDir(), tc.src); out != tc.want {
			t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
		}
	}
}
