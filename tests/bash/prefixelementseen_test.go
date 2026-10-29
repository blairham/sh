// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// bash refuses a subscripted prefix entry, so nothing behind it can see an
// element it never wrote. The controls for #6985 and #7012, where ksh93 and zsh
// show the entries behind it the element. Measured 2026-10-29 against GNU bash
// 5.3.20 under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script file
// alike. bash 3.2.57 takes the word without a complaint and reads the element
// the command found all the same (`f y=y`, `y=q p q r`). A scalar entry is seen,
// which is the last row. See interp/prefixsubscriptinturn.go.
func TestAPrefixElementIsNotSeenWhereTheWordIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"to a function", `b[1]=Q y=${b[1]} f`, "bash: line 2: `b[1]': not a valid identifier\nf y=y\n"},
		{"to a program", `b[1]=Q y=${b[1]} /usr/bin/printenv y`, "bash: line 2: `b[1]': not a valid identifier\ny\n"},
		{
			"to typeset", `c=(p q r); c[1]=B typeset y=${c[1]}; echo "y=$y ${c[*]}"`,
			"bash: line 2: `c[1]': not a valid identifier\ny=q p q r\n",
		},
		{
			"traced", `set -x; b[1]=Q y=${b[1]} f`,
			"bash: line 2: `b[1]': not a valid identifier\n+ y=y\n+ f\n+ echo 'f y=y'\nf y=y\n",
		},
		{"a scalar entry", `v=Q y=$v f`, "f y=Q\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), "b=(x y z); f() { echo \"f y=$y\"; }\n"+c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
