// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A refusal that quotes an assignment's subscript as written lays out each
// command substitution in it the way `declare -f` and the trace do — `>&2`
// is `1>&2`, a subshell gets its blanks — while a parameter, an arithmetic
// expansion and a backquoted body are quoted as written (#6762).
//
// Measured 2026-10-16 against GNU bash 5.3.20 as one `-c` string, byte for
// byte.
func TestARefusedSubscriptIsQuotedLaidOut(t *testing.T) {
	for _, c := range []struct{ probe, want string }{
		{
			"a=(1 2); a[$(echo SUB >&2; echo 1)]=(p q)",
			"bash: line 1: a[$(echo SUB 1>&2; echo 1)]: cannot assign list to array member\n",
		},
		{
			"a=(1); a[$(echo -9 >&2;echo -9)]=x",
			"-9\nbash: line 1: a[$(echo -9 1>&2; echo -9)]: bad array subscript\n",
		},
		{"a=(1); a[$( (echo -9) )]=x", "bash: line 1: a[$( ( echo -9 ))]: bad array subscript\n"},
		{"a=(1); a[$( (echo -9) )]+=x", "bash: line 1: a[$( ( echo -9 ))]: bad array subscript\n"},
		{"a=(1); i=-9; a[$i]=x", "bash: line 1: a[$i]: bad array subscript\n"},
		{"a=(1); i=-9; a[${i}$(echo)]=x", "bash: line 1: a[${i}$(echo)]: bad array subscript\n"},
		{`a=(1); a["$(echo -9;)"]=x`, "bash: line 1: a[\"$(echo -9)\"]: bad array subscript\n"},
		{`a=(1); a[${x:-"-9"}]=x`, "bash: line 1: a[${x:-\"-9\"}]: bad array subscript\n"},
		{"a=(1); a[$((-9))]=x", "bash: line 1: a[$((-9))]: bad array subscript\n"},
		{"a=(1); a[`echo -9`]=x", "bash: line 1: a[`echo -9`]: bad array subscript\n"},
	} {
		if got, _ := posixRouteStatus(t, c.probe, interp.RouteCommandString); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.probe, got, c.want)
		}
	}
}
