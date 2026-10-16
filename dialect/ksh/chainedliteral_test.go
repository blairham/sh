// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAChainedElementLiteralNestsAtTheEnd: `f[1][2]=(p q)` puts the literal
// under 2 of the array held in element 1, where it landed one level too
// shallow, under `f[2]`, and wrote no trace line (#6733). Measured 2026-10-16
// on ksh93u+ 2012-08-01, `-c`.
func TestAChainedElementLiteralNestsAtTheEnd(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"two links", `set -x; f[1][2]=(p q); typeset -p f`,
			"+ f[1][2]=( p q )\n+ typeset -p f\ntypeset -a f=([1]=([2]=(p q) ) )\n",
		},
		{
			"subscripts before the value", `i=0; f[i++][i++]=(p $((i*10))); typeset -p f; echo $i`,
			"typeset -a f=(([1]=(p 20) ) )\n2\n",
		},
		{"over a string", `f=(x y); f[1][0]=(p q); typeset -p f`, "typeset -a f=(x ((p q) ) )\n"},
		{"appended", `f[1][2]=(p q); f[1][2]+=(r); typeset -p f`, "typeset -a f=([1]=([2]=(p q r) ) )\n"},
		{"appended over a string", `b=((p) q); b[0][0]+=(z); typeset -p b`, "typeset -a b=(((z) ) q)\n"},
		{
			"from a key", `set -x; typeset -A m; m[k][2]=(p q); typeset -p m`,
			"+ typeset -A m\n+ m[k][2]=( p q )\n+ typeset -p m\ntypeset -A m=([k]=([2]=(p q) ) )\n",
		},
		{"three links", `f[1][2][3]=(p); typeset -p f`, "typeset -a f=([1]=([2]=([3]=(p) ) ) )\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestATracedNegativeSubscriptIsThePosition: the trace writes the position a
// negative subscript counted back to, as the chained write already did
// (#6735). Measured 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestATracedNegativeSubscriptIsThePosition(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), `a=(x y); set -x; a[-1]=v; a[-1]+=v; a[-2]=w; a[1]=q`)
	if want := "+ a[1]=v\n+ a[1]+=v\n+ a[0]=w\n+ a[1]=q\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
