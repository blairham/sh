// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestAnAssignOrErrorOperatorDeclaresAMissingKey: `${a[k]=x}` and `${a[k]?m}`
// reach a table's element as a place, so a key the table has not got is made,
// declared and holding nothing, and the colon-less test then finds it set —
// nothing is assigned and nothing is refused (#7187). Measured 2026-10-31 on
// ksh93u+ 2012-08-01, `-c`, every row.
func TestAnAssignOrErrorOperatorDeclaresAMissingKey(t *testing.T) {
	const keyed = "typeset -A a=([k]=)\n"
	for _, c := range []struct{ name, src, want string }{
		// The issue's own four rows.
		{"=", `typeset -A a; : ${a[k]=x}; typeset -p a`, keyed},
		{"= substitutes the empty key", `typeset -A a; echo "<${a[k]=x}>"; typeset -p a`, "<>\n" + keyed},
		{
			"= under a freeze", `typeset -A a; readonly a; : ${a[k]=x}; echo st=$?; typeset -p a`,
			"st=0\ntypeset -r -A a=([k]=)\n",
		},
		{"a key that is there", `typeset -A a=([j]=); readonly a; echo "<${a[j]=x}>"`, "<>\n"},

		{"?", `typeset -A a; echo "<${a[k]?m}>"; typeset -p a`, "<>\n" + keyed},
		{
			"? under a freeze", `typeset -A a; readonly a; echo "<${a[k]?m}>"; echo st=$?; typeset -p a`,
			"<>\nst=0\ntypeset -r -A a=([k]=)\n",
		},
		{
			"beside a key that is there", `typeset -A a=([j]=1); : ${a[k]=x}; typeset -p a`,
			"typeset -A a=([j]=1 [k]=)\n",
		},
		{"a computed key", `typeset -A a; k=z; : ${a[$k]=x}; typeset -p a`, "typeset -A a=([z]=)\n"},
		{"quoted", `typeset -A a; : "${a[k]=x}"; typeset -p a`, keyed},
		{"in [[ ]]", `typeset -A a; [[ ${a[k]=x} == x ]] && echo yes || echo no; typeset -p a`, "no\n" + keyed},
		{"in $(( ))", `typeset -A a; echo $(( ${a[k]=3} + 1 )); typeset -p a`, "1\n" + keyed},
		{"in a function body", `function f { typeset -A a; : ${a[k]=x}; typeset -p a; }; f`, keyed},
		{"under set -u", `typeset -A a; set -u; echo "<${a[k]=x}>"; typeset -p a`, "<>\n" + keyed},
		{
			"the key is set afterwards", `typeset -A a; : ${a[k]=x}; echo "<${a[k]-unset}>" ${#a[@]} ${!a[@]}`,
			"<> 1 k\n",
		},
		{"behind the !", `typeset -A a; echo "<${!a[k]=d}>"; typeset -p a`, "<a[k]>\n" + keyed},
		{"behind the !, :=", `typeset -A a; echo "<${!a[k]:=x}>"; typeset -p a`, "<a[k]>\n" + keyed},

		// The controls. The colon form of `=` assigns, `-`, `+` and a plain
		// read leave the table alone, and the same element reached through a
		// reference is assigned.
		{":= assigns", `typeset -A a; : ${a[k]:=x}; typeset -p a`, "typeset -A a=([k]=x)\n"},
		{"- leaves it", `typeset -A a; echo "<${a[k]-d}>"; typeset -p a`, "<d>\ntypeset -A a=()\n"},
		{"+ leaves it", `typeset -A a; echo "<${a[k]+s}>"; typeset -p a`, "<>\ntypeset -A a=()\n"},
		{"a read leaves it", `typeset -A a; : "${a[k]}"; typeset -p a`, "typeset -A a=()\n"},
		{
			"through a reference", `typeset -A a; typeset -n r='a[k]'; echo "<${r=x}>"; typeset -p a`,
			"<x>\ntypeset -A a=([k]=x)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runKsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
				t.Errorf("got %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// TestAColonErrorOperatorLeavesTheKeyItMade: `${a[k]:?m}` makes the key too,
// and the empty value it then finds is refused — so the key is what an EXIT
// trap lists afterwards (#7187). Measured 2026-10-31 on ksh93u+ 2012-08-01:
// `a: m` and `typeset -A a=([k]=)`.
func TestAColonErrorOperatorLeavesTheKeyItMade(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "trap 'typeset -p a' EXIT; typeset -A a; : ${a[k]:?m}; echo not here")
	if !strings.Contains(out, "a: m\n") || strings.Contains(out, "not here") || st == 0 {
		t.Errorf("got %q status %d, want the refusal ending the shell", out, st)
	}
	if !strings.HasSuffix(out, "typeset -A a=([k]=)\n") {
		t.Errorf("got %q, want it to end with the key declared", out)
	}
}

// TestAnEmptyKeyAnOperatorMakesWarnsOnce: under `set -x` the empty key warns
// once for each operator that makes it — the declaration for `=`, the store
// for `:=`, never both (#7187). Measured 2026-10-31 on ksh93u+ 2012-08-01, one
// `adding empty subscript` for each row.
func TestAnEmptyKeyAnOperatorMakesWarnsOnce(t *testing.T) {
	for _, c := range []struct{ name, op, listing string }{
		{"=", "=", "typeset -A a=(['']=)\n"},
		{"?", "?", "typeset -A a=(['']=)\n"},
		{":=", ":=", "typeset -A a=(['']=x)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := "typeset -A a; k=\nset -x\n: ${a[$k]" + c.op + "x}\nset +x\ntypeset -p a\n"
			out, _ := runKsh(t, t.TempDir(), src)
			if n := strings.Count(out, "adding empty subscript"); n != 1 {
				t.Errorf("got %q, want one warning and not %d", out, n)
			}
			if !strings.HasSuffix(out, c.listing) {
				t.Errorf("got %q, want it to end with %q", out, c.listing)
			}
		})
	}
}
