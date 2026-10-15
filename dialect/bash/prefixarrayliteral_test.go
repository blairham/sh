// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnArrayLiteralPrefixIsItsText pins #6547: bash reads `x=(a b)` in front
// of a command as a scalar holding the expanded elements, joined with a space
// and kept in their parentheses. Measured 2026-10-15 on bash 5.3.20 from `-c`
// under `env -i PATH=/usr/bin:/bin`.
func TestAnArrayLiteralPrefixIsItsText(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"function", `f(){ declare -p x; }; x=(a b) f`, "declare -x x=\"(a b)\"\n"},
		{"external", `x=(a b) /usr/bin/printenv x; echo rc=$?`, "(a b)\nrc=0\n"},
		{"over an export", `export x=old; x=(a b) /usr/bin/printenv x`, "(a b)\n"},
		{"a substitution", `f(){ declare -p x; }; x=($(echo s >&2) b) f`, "s\ndeclare -x x=\"( b)\"\n"},
		{"quoting, globs, tildes and braces", `x=("a  b" *.txt ~ {a,b}) /usr/bin/printenv x`, "(a  b *.txt ~ {a,b})\n"},
		{"a subscript", `i=5; x=([$i]=a [i+1]=b) /usr/bin/printenv x`, "([5]=a [i+1]=b)\n"},
		{"an append", `x=old; x+=(a b) /usr/bin/printenv x`, "old(a b)\n"},
		{"an empty literal", `f(){ declare -p x; }; x=() f`, "declare -x x=\"()\"\n"},
		{"seen behind it", `x=(a b) y=$x /usr/bin/printenv y`, "(a b)\n"},
		{"the trace", `set -x; x=(a $(echo b)) true`, "++ echo b\n+ x='(a b)'\n+ true\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("%s\n= %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
