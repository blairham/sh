// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A frozen array written through a reference: an element or a list names the
// reference, a scalar names the target (#6763).
//
// Measured 2026-10-16 against GNU bash 5.3.20 as one `-c` string, after
// `a=(1 2); readonly a; typeset -n r=a`, byte for byte.
func TestAFrozenContainerWriteThroughAReferenceNamesTheReference(t *testing.T) {
	for _, c := range []struct{ probe, want string }{
		{"r[1]=x", "bash: line 1: r: readonly variable\n"},
		{"r[1]+=x", "bash: line 1: r: readonly variable\n"},
		{"r=(p)", "bash: line 1: r: readonly variable\n"},
		{"r+=(p)", "bash: line 1: r: readonly variable\n"},
		{"declare r=(p)", "bash: line 1: r: readonly variable\n"},
		{`read "r[1]" <<< x`, "bash: line 1: r: readonly variable\n"},
		{`printf -v "r[1]" x`, "bash: line 1: r: readonly variable\n"},
		{"(( r[1]=3 ))", "bash: line 1: r: readonly variable\n"},
		{"r=x", "bash: line 1: a: readonly variable\n"},
		{"r+=x", "bash: line 1: a: readonly variable\n"},
		{"read r <<< x", "bash: line 1: a: readonly variable\n"},
	} {
		src := "a=(1 2); readonly a; typeset -n r=a; " + c.probe
		if got, _ := posixRouteStatus(t, src, interp.RouteCommandString); got != c.want {
			t.Errorf("%q\n got %q\nwant %q", c.probe, got, c.want)
		}
	}
}
