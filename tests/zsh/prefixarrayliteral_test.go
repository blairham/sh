// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnArrayLiteralPrefixIsAnArray pins #6547: zsh stores `x=(a b)` in front
// of a command as an array for the length of the command, hands an external
// child no `x` at all, and takes no export attribute for it. Measured
// 2026-10-15 on zsh 5.9.2 from `-c` under `env -i PATH=/usr/bin:/bin`.
func TestAnArrayLiteralPrefixIsAnArray(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"function", `f(){ typeset -p x; }; x=(a b) f; typeset -p x`,
			"typeset -g -a x=( a b )\nzsh:typeset:1: no such variable: x\n",
		},
		{
			"over an export", `export x=old; f(){ typeset -p x; }; x=(a b) f; typeset -p x`,
			"typeset -g -a x=( a b )\nexport x=old\n",
		},
		{"a builtin", `export x=old; x=(a b) eval 'typeset -p x'`, "typeset -a x=( a b )\n"},
		{"external", `x=(a b) /usr/bin/printenv x; echo rc=$?`, "rc=1\n"},
		{"external over an export", `export x=old; x=(a b) /usr/bin/printenv x; echo rc=$?`, "rc=1\n"},
		{"through command", `x=(a b) command /usr/bin/printenv x; echo rc=$?`, "rc=1\n"},
		{"a substitution", `f(){ typeset -p x; }; x=($(echo s >&2) b) f`, "s\ntypeset -g -a x=( b )\n"},
		{"an append", `x=(a b); f(){ typeset -p x; }; x+=(c) f`, "typeset -g -a x=( a b c )\n"},
		{"seen behind it", `x=(a b) y=$x /usr/bin/printenv y`, "a b\n"},
		{
			"the trace", `set -x; x=(a $(echo b)) /usr/bin/true`,
			"+zsh:1> x=+zsh:1> echo b\n+zsh:1> x=( a b ) /usr/bin/true\n",
		},
		// The per-entry trace walk holds each value for the ones behind it,
		// scalar or array — this wrote `y=''` and handed the command an
		// empty `y`.
		{
			"seen behind it under a trace", `set -x; x=1 y=$x /usr/bin/printenv y`,
			"+zsh:1> x=1 y=1 /usr/bin/printenv y\n1\n",
		},
		{
			"an array seen behind it under a trace", `set -x; x=(a b) y=$x /usr/bin/printenv y`,
			"+zsh:1> x=( a b ) y='a b' /usr/bin/printenv y\na b\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// TestAPlacedArrayLiteralPrefixIsSeenBehindIt pins #6636: a prefix literal
// whose elements name their places is seen by the entries behind it as the
// array it places, on the route that stores nothing (an external command) as
// on the ones that store it, and a subscript with a side effect moves the
// variable once — the hold places it ahead of the store and the store reuses
// the index. Measured 2026-10-15 on zsh 5.9.2, `-f`, `env -i
// PATH=/usr/bin:/bin`, `-c` and a script file alike.
func TestAPlacedArrayLiteralPrefixIsSeenBehindIt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{`x=([2]=a) y=${x[2]} /usr/bin/printenv y`, "a\n"},
		{`x=([2]=a b) y=${x[3]} /usr/bin/printenv y`, "b\n"},
		{`x=([1]=a [3]=c) y="${x[*]}" /usr/bin/printenv y`, "a  c\n"},
		{`typeset -A x; x=([k]=v) y=${x[k]} /usr/bin/printenv y`, "v\n"},
		{`i=1; x=([i++]=a) y=$i:${x[1]} /usr/bin/printenv y`, "2:a\n"},
		{`f(){ echo "[$y]"; }; x=([2]=a) y=${x[2]} f`, "[a]\n"},
		{`i=1; f(){ echo "$i $y"; }; x=([i++]=a) y=$i:${x[1]} f; echo $i`, "2 2:a\n2\n"},
		{`i=1; { set -x; x=([i++]=a) y=$i:${x[1]} true; set +x; } 2>/dev/null; echo $i $y`, "2\n"},
		{`x=([2]=a) y=${x[2]} /usr/bin/printenv y; echo "[${x[2]}]"`, "a\n[]\n"},
	} {
		if out, _ := runZshOnPath(t, t.TempDir(), tc.src); out != tc.want {
			t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
		}
	}
}
