// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// The control for bash's frozen literal refused at its own turn (#7103): in a
// `function`-form body ksh93's `typeset` shadows a frozen global, so the same
// declaration refuses nothing and every literal is the call's own. Measured
// 2026-11-02 on ksh93u+ 2012-08-01 from a script file under `env -i
// PATH=/usr/bin:/bin`.
func TestAFrozenLiteralInAKeywordFunctionIsShadowedNotRefused(t *testing.T) {
	t.Parallel()
	src := "readonly r=1\n" +
		"function f { typeset -a c=(5) r=(b) d=(6); echo \"in st=$? c=${c[*]} d=${d[*]} r=${r[*]}\"; }\n" +
		"f\necho \"out c=${c[*]-unset} d=${d[*]-unset} r=$r\"\n"
	want := "in st=0 c=5 d=6 r=b\nout c=unset d=unset r=1\n"
	if out, _ := answersRun(t, src); out != want {
		t.Errorf("%s\ngot  %q\nwant %q", src, out, want)
	}
}
