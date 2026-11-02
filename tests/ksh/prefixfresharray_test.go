// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// In front of a regular builtin or a `function`-form call, where the prefix is
// the command's own scope rather than a store this shell keeps, a subscripted
// entry is an element of a fresh indexed array: the subscript is read as
// arithmetic and the shell's own name is never touched. In front of a special
// builtin it is the table's element. Measured 2026-11-02 on ksh93u+
// 2012-08-01 (`/bin/ksh`) from script files under `env -i
// PATH=/usr/bin:/bin` with `typeset -A a; a[z]=old` first, where this shell
// wrote the table's element on every route (#6982). See
// interp/prefixfresharray.go.
func TestASubscriptedPrefixTheCommandOwnsIsAFreshArray(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		// The issue's rows, traced.
		{"a regular builtin", `a[k]=1 echo hi`, "+ echo hi\n+ a[0]=1\nhi\n"},
		{
			"a key that is no expression", `a["p q"]=1 echo hi; echo $?`,
			"+ echo hi\nksh[2]: p q: arithmetic syntax error\n+ echo 1\n1\n",
		},
		{"two entries and no warning", `a[k]=1 a[""]=2 echo hi`, "+ echo hi\n+ a[0]=1\n+ a[0]=2\nhi\n"},
		{"a function-form call", `function g { :; }; a[k]=1 g`, "+ g\n+ a[0]=1\n"},
		{
			"the call leaves the table", `function g { :; }; a[k]=1 g; typeset -p a`,
			"+ g\n+ a[0]=1\n+ typeset -p a\ntypeset -A a=([z]=old)\n",
		},
		// The minimal pair: the same entry, a regular builtin and a special
		// one.
		{
			"a regular builtin leaves the table", `a[k]=1 echo hi; typeset -p a`,
			"+ echo hi\n+ a[0]=1\nhi\n+ typeset -p a\ntypeset -A a=([z]=old)\n",
		},
		{
			"a special builtin writes the table", `a[k]=1 :; typeset -p a`,
			"+ a[k]=1\n+ :\n+ typeset -p a\ntypeset -A a=([k]=1 [z]=old)\n",
		},
		// The words and the value are read from the name the shell holds,
		// and the entries behind read the fresh array.
		{
			"the words read the table", `function g { typeset -p a; }; a[${#a[@]}]=${a[z]} g`,
			"+ g\n+ a[1]=old\ntypeset -x -a a=([1]=old)\n",
		},
		{
			"an entry behind sees one element", `a[k]=5 y=${#a[@]} let "x=y"; echo $x`,
			"+ let x=y\n+ a[0]=5\n+ y=1\n+ echo 1\n1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), "typeset -A a; a[z]=old; set -x\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// What the command itself is shown, untraced: the builtin reads index 0 of an
// array holding the prefix's elements and nothing else, and the body of a
// `function`-form call lists that array. Each entry after the first on the
// same name writes into the array the first one made. Measured the same day.
func TestTheCommandSeesItsOwnFreshArray(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"refused untraced", `a["p q"]=1 echo hi; echo $?`, "ksh[2]: p q: arithmetic syntax error\n1\n"},
		{"the builtin reads index 0", `a[k]=7 let "x=a[0]"; echo $x; typeset -p a`, "7\ntypeset -A a=([z]=old)\n"},
		{"one element", `a[k]=5 y=${#a[@]} let "x=y"; echo $x`, "1\n"},
		{
			"over an indexed name", `b=(10 20 30); b[1]=5 let "x=b[0]+b[1]"; echo $x; typeset -p b`,
			"5\ntypeset -a b=(10 20 30)\n",
		},
		{"the words read the table", `a[${#a[@]}]=7 let "x=a[1]"; echo $x`, "7\n"},
		{"an append joins the element", `a[k]=1 a[k]+=2 let "x=a[0]"; echo $x`, "12\n"},
		// The body's view.
		{
			"the body lists the array", `function g { typeset -p a; }; a[k]=1 g; typeset -p a`,
			"typeset -x -a a=(1)\ntypeset -A a=([z]=old)\n",
		},
		{
			"both entries reach the body", `function g { typeset -p a; }; k=5; a[k+1]=1 a[2]=x g`,
			"typeset -x -a a=([2]=x [6]=1)\n",
		},
		{
			"the body's words read the table", `function g { typeset -p a; }; a[${#a[@]}]=${a[z]} g`,
			"typeset -x -a a=([1]=old)\n",
		},
		{"the body's append", `function g { typeset -p a; }; a[1]=x a[1]+=y g`, "typeset -x -a a=([1]=xy)\n"},
		{"a scalar's cell is made once", `function g { typeset -p s; }; s=base; s=1 s+=2 g`, "typeset -x s=12\n"},
		{
			"a call's failed subscript ends the script", `function g { typeset -p a; }; a["p q"]=1 g; echo $?`,
			"ksh: line 2: p q: arithmetic syntax error\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), "typeset -A a; a[z]=old\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
