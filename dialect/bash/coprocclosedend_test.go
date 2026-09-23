// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAClosedCoprocessEndReadsBackAsMinusOne — closing one of a coprocess's
// near ends through the array that published it replaces the number with -1.
//
// It is the coprocess's own bookkeeping about its ends, and it is visible to a
// script: a caller that reads the array after closing one end was handed a
// descriptor this shell no longer had.
//
// Measured 2026-09-23 against bash 5.3.15 in the pinned debian:sid-slim and
// bash 5.3.20 on macOS, which agree. The two controls are what say it is the
// coprocess's array and not the `{name}>&-` form in general (#4136).
func TestAClosedCoprocessEndReadsBackAsMinusOne(t *testing.T) {
	out, errs := runBashSplitFatal(t, `coproc { read x; echo "[$x]"; }
echo hi >&${COPROC[1]}
exec {COPROC[1]}>&-
echo "after [${COPROC[1]}]"
read -r l <&${COPROC[0]}
echo "got $l"
wait
`)
	if want := "after [-1]\ngot [hi]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing", errs)
	}
	// The controls. A plain variable and an ordinary array element both keep
	// the number they were given, in this shell and the reference alike.
	for _, tc := range []struct{ name, src, want string }{
		{
			"a plain variable",
			"exec {fd}</dev/null\nexec {fd}<&-\necho \"[$fd]\"\n",
			"[10]\n",
		},
		{
			"an ordinary array element",
			"exec {a[1]}</dev/null\nexec {a[1]}<&-\necho \"[${a[1]}]\"\n",
			"[10]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBashSplitFatal(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
