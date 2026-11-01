// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestARefusedGetoptsStoreLeavesZeroFromACommandString pins #6588: a refused
// `getopts` store ends zsh and leaves 0 from `-c`, the number the other
// operand stores leave, raised to 1 by a script file's top level, an `&&`
// and a loop, and left at 0 in a subshell. Measured 2026-10-15 against zsh
// 5.9.2 (`/opt/homebrew/bin/zsh -f` under `env -i PATH=/usr/bin:/bin`, from
// `-c` and from a script file), the sentence compared byte for byte.
func TestARefusedGetoptsStoreLeavesZeroFromACommandString(t *testing.T) {
	t.Parallel()
	const (
		ro      = "zsh:1: read-only variable: v\n"
		private = "zmodload zsh/param/private; f(){ private v=1; g; }; g(){ set -- -a; getopts a v; }; f"
	)
	for _, tc := range []struct {
		src, want string
		command   int
	}{
		{"typeset -r v=1; set -- -a; getopts a v", ro, 0},
		{"typeset -r v=1; set -- -z; getopts a v", ro, 0},
		{"typeset -r v=1; set -- -a; getopts a v || echo or", ro, 0},
		{"typeset -r v=1; set -- -a; getopts a v && echo and", ro, 1},
		{"typeset -r v=1; set -- -a; for i in 1; do getopts a v; done", ro, 1},
		{private, "g: v: can't change parameter attribute\n", 0},
	} {
		for _, route := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, tc.command},
			{interp.RouteScriptFile, 1},
		} {
			out, st := runZshRoute(t, t.TempDir(), tc.src+"; echo after", route.route)
			if st != route.want {
				t.Errorf("%s from %v = status %d, want %d", tc.src, route.route, st, route.want)
			}
			if route.route == interp.RouteCommandString && out != tc.want {
				t.Errorf("%s = %q, want %q and nothing after it", tc.src, out, tc.want)
			}
		}
	}
	// A subshell leaves the 0 by both routes.
	for _, route := range []interp.Route{interp.RouteCommandString, interp.RouteScriptFile} {
		out, _ := runZshRoute(t, t.TempDir(), "typeset -r v=1; set -- -a; ( getopts a v ); echo $?", route)
		if want := "0\n"; len(out) < len(want) || out[len(out)-len(want):] != want {
			t.Errorf("a subshell from %v = %q, want it to end %q", route, out, want)
		}
	}
}
