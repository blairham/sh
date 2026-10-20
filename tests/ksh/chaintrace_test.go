// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A chained element write, `a[1][2]=v`, writes its trace line, with every
// subscript as what it resolved to (#6678). Measured 2026-10-15 on ksh93u+
// 2012-08-01, -c and from a file; every `want` is that shell's own output.
func TestAChainedElementWriteIsTraced(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -x; a[$(echo 1)][$(echo 2)]=$(echo v)", "+ echo 1\n+ echo 2\n+ echo v\n+ a[1][2]=v\n"},
		{"set -x; b[1][2]=w", "+ b[1][2]=w\n"},
		{"set -x; c[1][2]=x; c[1][2]+=y", "+ c[1][2]=x\n+ c[1][2]+=y\n"},
		{"set -x; d[1][2][3]=z", "+ d[1][2][3]=z\n"},
		// Each link is evaluated: an unset name is 0.
		{"set -x; e[a][b]=q", "+ e[0][0]=q\n"},
		// The first link of a table is a key, quoted like one; the rest are
		// numbers.
		{"typeset -A m; set -x; m['a b'][1+1]=v", "+ m['a b'][2]=v\n"},
		// A negative first subscript is written as the position it named.
		{"a=(x y); set -x; a[-1][2]=v", "+ a[1][2]=v\n"},
		// A refused chain writes no line, as a refused single write does not.
		{"set -x; f[1][1/0]=w", "ksh: 1/0: divide by zero\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
