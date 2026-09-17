// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Refusals bash makes and this dialect made in silence, each measured on bash
// 5.3.20 from a script file on 2026-09-16. The snippets are one construct per
// line, because what a refusal costs — the line, the loop, the script — is
// half of each answer and a one-line probe cannot see the difference.
func TestRefusalsBashMakes(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// The readonly sentence without its reason, for four of the five
			// arrays the call stack produces. FUNCNAME is the control.
			"the call-stack arrays refuse unset",
			"unset BASH_SOURCE; echo a=$?\n" +
				"x=1; unset BASH_LINENO x; echo \"b=$? x=${x-gone}\"\n" +
				"unset 'BASH_ARGV[0]'; echo c=$?\n" +
				"f() { unset -v BASH_ARGC; echo d=$?; }; f\n" +
				"unset FUNCNAME; echo e=$?\n",
			"sh: line 1: unset: BASH_SOURCE: cannot unset\na=1\n" +
				"sh: line 2: unset: BASH_LINENO: cannot unset\nb=1 x=gone\n" +
				"sh: line 3: unset: BASH_ARGV: cannot unset\nc=1\n" +
				"sh: line 4: unset: BASH_ARGC: cannot unset\nd=1\n" +
				"e=0\n",
		},
		{
			"an indirection through a name nothing declared",
			"unset u; echo \"[${!u-D}]\"; echo same-line\n" +
				"echo \"[${!nosuch[3]}]\"\n" +
				"declare x; echo \"[${!x}]\"\n" +
				"declare -a arr; echo \"[${!arr[9]}]\"\n" +
				"echo A=$?\n",
			"sh: line 1: u: invalid indirect expansion\n" +
				"sh: line 2: nosuch[3]: invalid indirect expansion\n" +
				"[]\n[]\nA=0\n",
		},
		{
			"an indirection through text that names no parameter",
			"v=-3; echo one; echo \"[${!v:-D}]\"; echo two\n" +
				"echo A=$?\n" +
				"v=''; echo \"[${!v}]\"\n" +
				"v='a[1]b'; echo \"[${!v}]\"\n" +
				"v=' a'; echo \"[${!v}]\"\n" +
				"set -- p; a=(x y); v=1; echo \"[${!v}]\"; v='a[1]'; echo \"[${!v}]\"; v=@; echo \"[${!v}]\"\n",
			"one\nsh: line 1: -3: invalid variable name\nA=1\n" +
				"sh: line 3: : invalid variable name\n" +
				"sh: line 4: a[1]b: invalid variable name\n" +
				"sh: line 5:  a: invalid variable name\n" +
				"[p]\n[y]\n[p]\n",
		},
		{
			"the refusal is ahead of set -u, and still costs only the line",
			"set -u\nunset u; echo \"[${!u}]\"\necho gone\n",
			"sh: line 2: u: invalid indirect expansion\ngone\n",
		},
		{
			"a loop count out of range ends every loop",
			"for i in 1 2; do for j in a b; do echo $i$j; continue 0; echo tail; done; echo mid; done; echo \"st=$?\"\n",
			"1a\nsh: line 1: continue: 0: loop count out of range\nst=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}
