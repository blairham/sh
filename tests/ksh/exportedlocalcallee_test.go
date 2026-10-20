// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A caller's exported local is handed to a `function`-form callee** as a
// plain local of its own, where every other local of the caller is sealed
// away from it — see Runner.exportedLocalHandedOn (#6595).
//
// Measured 2026-10-15 on ksh93u+ 2012-08-01 under `-c` and from a script
// file, `env -i PATH=/usr/bin:/bin`, with `v=base` in front of every row.
func TestAnExportedLocalIsHandedToAFunctionFormCallee(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{
			`function f { typeset -x v=4; g; echo "f[$v]"; }; function g { echo "g[$v]"; printenv v || echo noenv; }; f; echo "[$v]"`,
			"g[4]\nnoenv\nf[4]\n[base]\n",
		},
		{
			`function f { typeset -x v=4; g; echo "f[$v]"; }; function g { v=7; echo "g[$v]"; }; f; echo "[$v]"`,
			"g[7]\nf[4]\n[base]\n",
		},
		{
			`function f { typeset -x v=4; g; echo "f[$v]"; }; function g { unset v; echo "g[${v-u}]"; }; f; echo "[$v]"`,
			"g[u]\nf[4]\n[base]\n",
		},
		{
			`function f { typeset -xi v=4; g; }; function g { typeset -p v; v=1+1; echo "g[$v]"; }; f`,
			"v=4\ng[1+1]\n",
		},
		{
			`function f { typeset -xr v=4; g; }; function g { v=7; echo "g[$v]"; }; f`,
			"g[7]\n",
		},
		{
			`function f { typeset -x v=4; g; }; function g { h; }; function h { echo "h[$v]"; }; f`,
			"h[base]\n",
		},
		{
			`function f { typeset -x v=4; g; }; function g { typeset -x v; h; }; function h { echo "h[$v]"; }; f`,
			"h[4]\n",
		},
		{
			`function f { typeset -x v=4; p; }; p() { g; }; function g { echo "g[$v]"; }; f`,
			"g[4]\n",
		},
		{
			`function f { typeset v=4; export v; g; }; function g { echo "g[$v]"; }; f`,
			"g[4]\n",
		},
		{
			`function f { typeset -xa v=(4 5); g; }; function g { echo "g[$v] ${#v[@]}"; }; f`,
			"g[4] 1\n",
		},
		// The controls: a local the caller did not export stays sealed, and
		// so does one that merely shadows an exported global.
		{`function f { typeset v=4; g; }; function g { echo "g[$v]"; }; f`, "g[base]\n"},
		{`export v; function f { typeset v=4; g; }; function g { echo "g[$v]"; }; f`, "g[base]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{
			Name: "ksh", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
		}, "v=base; "+tc.src+"\n")
		if err != nil {
			t.Fatalf("%s: %v", tc.src, err)
		}
		if out != tc.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
