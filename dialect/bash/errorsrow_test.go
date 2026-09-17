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
			"a function line refuses the letters that make a kind of variable",
			"f() { :; }\n" +
				"declare -f -a -x f; echo a=$?; declare -F -p\n" +
				"typeset -F -i f; echo b=$?\n" +
				"declare -f -p -n f >/dev/null; echo c=$?\n" +
				"declare -f +a f; echo d=$?\n" +
				"declare -F -i; echo e=$?\n",
			"sh: line 2: declare: -a: invalid option\na=1\ndeclare -f f\n" +
				"sh: line 3: typeset: -i: invalid option\nb=1\n" +
				"c=0\nd=0\ne=0\n",
		},
		{
			"a function attribute comes off under a plus, and the freeze does not",
			"g() { :; }; declare -fx g; readonly -f g\n" +
				"declare -f +r +x g; echo a=$?; declare -F -p\n" +
				"declare -f +x g; echo b=$?; declare -F -p\n" +
				"declare -F +x\n",
			"sh: line 2: declare: g: readonly function\na=1\ndeclare -frx g\n" +
				"b=0\ndeclare -fr g\n" +
				"declare -fr g\n",
		},
		{
			"read names what a bad number was for, and refuses an empty array name",
			"read -t abc x </dev/null; echo a=$?\n" +
				"read -u ab x </dev/null; echo b=$?\n" +
				"read -n abc x </dev/null; echo c=$?\n" +
				"read -a '' </dev/null; echo d=$?\n" +
				"mapfile '' </dev/null; echo e=$?\n",
			"sh: line 1: read: abc: invalid timeout specification\na=1\n" +
				"sh: line 2: read: ab: invalid file descriptor specification\nb=1\n" +
				"sh: line 3: read: abc: invalid number\nc=1\n" +
				"sh: line 4: read: `': not a valid identifier\nd=1\n" +
				"sh: line 5: mapfile: empty array variable name\ne=2\n",
		},
		{
			"builtin reads options, and source is named as it was invoked",
			"builtin -q; echo a=$?\n" +
				"builtin -- echo hi; echo b=$?\n" +
				"source; echo c=$?\n",
			"sh: line 1: builtin: -q: invalid option\nbuiltin: usage: builtin [shell-builtin [arg ...]]\na=2\n" +
				"hi\nb=0\n" +
				"sh: line 3: source: filename argument required\nsource: usage: source [-p path] filename [arguments]\nc=2\n",
		},
		{
			"trap reads every option word",
			"trap 'echo e' EXIT\n" +
				"trap -p -x name; echo a=$?\n" +
				"trap -p -- EXIT; echo b=$?\n" +
				"trap -pP EXIT; echo c=$?\n",
			"sh: line 2: trap: -x: invalid option\ntrap: usage: trap [-Plp] [[action] signal_spec ...]\na=2\n" +
				"trap -- 'echo e' EXIT\nb=0\n" +
				"sh: line 4: trap: cannot specify both -p and -P\nc=2\n" +
				"e\n",
		},
		{
			"shopt -o refuses a name set does not know, in its own words and at 0",
			"set -e\nshopt -o -s errexit nosuch; echo a=$?\n",
			"sh: line 2: shopt: nosuch: invalid option name\na=0\n",
		},
		{
			"logout, unset -fv and a function line with an assignment are refused",
			"f() { :; }\n" +
				"logout 3; echo a=$?\n" +
				"unset -f -v f; echo b=$?; type -t f\n" +
				"declare -f g='echo hi'; echo c=$?\n",
			"sh: line 2: logout: not login shell: use `exit'\na=1\n" +
				"sh: line 3: unset: cannot simultaneously unset a function and a variable\nb=1\nfunction\n" +
				"sh: line 4: declare: cannot use `-f' to make functions\nc=1\n",
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
