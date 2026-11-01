// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A **written** declaration operand whose subscript holds `=` is split after
// the subscript, so the key is `e=f`; a quoted or expanded one is cut at its
// first `=` and refused as a name. The two arrive at the builtin as one
// string, so the split is carried from the word (#6896).
//
// Measured 2026-10-27 on zsh 5.9.2 from a script file, `typeset -A m` first.
func TestAWrittenOperandsSubscriptMayHoldAnEquals(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"typeset -A m; typeset m[e=f]=y; typeset -p m\n", "typeset -A m=( ['e=f']=y )\n"},
		{"typeset -A m; typeset m[\"e=f\"]=y; typeset -p m\n", "typeset -A m=( ['e=f']=y )\n"},
		{"f() { local -A m; local m[e=f]=y; typeset -p m; }; f\n", "typeset -A m=( ['e=f']=y )\n"},
		{"typeset -A m; k=e=f; typeset m[$k]=y; typeset -p m\n", "typeset -A m=( ['e=f']=y )\n"},
		{"typeset -A m; export m[e=f]=y; typeset -p m\n", "typeset -A m=( ['e=f']=y )\n"},
		{"typeset -A m; typeset m[e=f]=y=z; typeset -p m\n", "typeset -A m=( ['e=f']='y=z' )\n"},
		{"typeset -A m; typeset m[x]=1 m[e=f]=y; typeset -p m\n", "typeset -A m=( ['e=f']=y [x]=1 )\n"},
		{"typeset -A m; set -x; typeset m[e=f]=y\n", "+zsh:1> typeset 'm[e=f]'=y\n"},
		// The quoted and expanded spellings are text, cut at the first `=`.
		{"typeset -A m; typeset 'm[a=b]=w'\n", "zsh:1: not an identifier: m[a\n"},
		{"typeset -A m; v='m[c=d]=x'; typeset \"$v\"\n", "zsh:1: not an identifier: m[c\n"},
		// An append over a subscript is refused with the `+` on the name,
		// after the subscript as the written split puts it.
		{"typeset -A m; typeset m[e=f]+=y\n", "zsh:1: not an identifier: m[e=f]+\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
