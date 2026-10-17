// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAnArithmeticSubscriptsQuotesComeOff: a quoted bracket or parenthesis in
// an arithmetic command's subscript is the key's and not the scan's (#6772);
// a `$'…'` quotation's quotes come off without its escapes being decoded and a
// double quotation's backslashes come out with what they escape (#6773); and
// the trace writes the expression with those quotes taken off, a bracket left
// in a key escaped (#6683, #6772, #6773). Measured 2026-10-16 on ksh93u+
// 2012-08-01, `-c`.
func TestAnArithmeticSubscriptsQuotesComeOff(t *testing.T) {
	const tbl = "typeset -A a; "
	for _, c := range []struct{ name, src, want string }{
		{"a quoted open bracket", tbl + `(( a["["]=1 )); typeset -p a`, "typeset -A a=(['[']=1)\n"},
		{"a quoted close bracket", tbl + `(( a["]"]=1 )); typeset -p a`, "typeset -A a=([']']=1)\n"},
		{"a quoted paren", tbl + `(( a[")"]=1 )); typeset -p a`, "typeset -A a=([')']=1)\n"},
		{"quoting outside brackets is still blind", `((echo "a)b"))`, "a)b\n"},
		{"dollar-single", tbl + `(( a[$'x']=1 )); typeset -p a`, "typeset -A a=([x]=1)\n"},
		{"dollar-single undecoded", tbl + `(( a[$'x\ty']=1 )); typeset -p a`, "typeset -A a=(['x\\ty']=1)\n"},
		{"an escaped double quote", tbl + `(( a["\""]=3 )); typeset -p a`, "typeset -A a=(['\"']=3)\n"},
		{"inside a double quotation", tbl + `(( a["a\"b"]=4 )); typeset -p a`, "typeset -A a=(['a\"b']=4)\n"},
		{
			"the trace", tbl + `typeset -A b; set -x; (( a["p q"]=1 + b["x"] )); (( a[$'x']=1 )); (( a["["]=1 )); (( a["]"]=2 )); (( a["a\"b"]=4 )); (( a['x y']=5 ))`,
			"+ (( a[p q]=1 + b[x] ))\n+ (( a[x]=1 ))\n+ (( a[\\[]=1 ))\n+ (( a[\\]]=2 ))\n+ (( a[a\"b]=4 ))\n+ (( a[x y]=5 ))\n",
		},
		{"the empty key's trace", tbl + `set -x; (( a[""]=3 ))`, "+ (( a[]=3 ))\nksh: warning: adding empty subscript\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
