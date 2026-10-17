// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// A function-local EXIT trap runs once the call is left: in the caller's
// frame, with the caller's parameters and stack, at the line of the call
// (#6720). Measured 2026-10-16 on zsh 5.9.2 under `-f` from a script file:
// the first row is `v=out a=top ft=`, and a failure in the body of a trap set
// by a function called on line 9 is `z2.sh:9:` with `$LINENO` 9, where this
// read the function's `$1` and stack and located the body at the function.
func TestAFunctionExitTrapRunsInTheCallersFrame(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"f(){ local v=in; trap 'echo v=$v a=$1 ft=$funcstack' EXIT; }\nv=out; set -- top; f arg\n", "v=out a=top ft=\n"},
		{"f() {\ntrap \"echo L=\\$LINENO\" EXIT\n:\n:\n}\n:\n:\n:\nf\n:\n", "L=9\n"},
	} {
		out, _, errs := runZshSplitOnRoute(t, interp.RouteScriptFile, c.src)
		if out != c.want {
			t.Errorf("%q: wrote %q (%q), want %q", c.src, out, errs, c.want)
		}
	}
}
