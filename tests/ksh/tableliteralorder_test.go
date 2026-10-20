// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAKeyedLiteralIsBuiltAPairAtATime is the Yes side of
// interp.Semantics.TableLiteralStoresEachElementAsItIsExpanded: the table is
// emptied, then each pair is expanded, stored and traced before the next is
// expanded (#6770). Measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestAKeyedLiteralIsBuiltAPairAtATime(t *testing.T) {
	const tbl = "typeset -A a=([k]=old [z]=zz); "
	for _, c := range []struct{ name, src, want string }{
		{"a later pair sees an earlier one", tbl + `a=([k]=1 [x]=${a[k]}); typeset -p a`, "typeset -A a=([k]=1 [x]=1)\n"},
		{"an append keeps the rest", tbl + `a+=([k]=1 [x]=${a[k]}${a[z]}); typeset -p a`, "typeset -A a=([k]=1 [x]=1zz [z]=zz)\n"},
		{
			"the table is emptied first", tbl + `a=([k]=1 [x]=$(typeset -p a)); typeset -p a`,
			"typeset -A a=([k]=1 [x]='typeset -A a=([k]=1)')\n",
		},
		{
			"each line after its store", tbl + `set -x; a=([k]=1 [x]=$(echo s >&2) [y]=2)`,
			"+ a[k]=1\n+ echo s\n+ 1>& 2\ns\n+ a[x]=''\n+ a[y]=2\n",
		},
		{"a failure after a store", tbl + `set -x; a=([k]=1 [x]=$((1/0))); echo after`, "+ a[k]=1\nksh: 1/0: divide by zero\n"},
		{"a word list is not built this way", `c=(old); c=(1 ${c[0]}); typeset -p c`, "typeset -a c=(1 old)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
