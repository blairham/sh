// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestAPrivateScopeRefusalThroughABuiltinIsLocatedAsTheFunction pins that a
// name an enclosing call declared `private`, written by a builtin in a
// callee, is refused with the function alone in the location — `g: v:` and
// never `g:set: v:` — whichever builtin made the store, the same location
// the bare literal gets. And that the number the refusal leaves from `-c` is
// the store's: 0 for the whole-array stores and `printf -v`/`print -v`, as
// their frozen-name refusals already leave, and 1 for `read v`'s scalar
// store; a script file's top level raises every one of them to 1.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script file), the
// sentence compared byte for byte (#6515).
func TestAPrivateScopeRefusalThroughABuiltinIsLocatedAsTheFunction(t *testing.T) {
	t.Parallel()
	const pre = "zmodload zsh/param/private; f() { private v=1; g; }; g() { "
	const want = "g: v: can't change parameter attribute\n"
	for _, tc := range []struct {
		body    string
		command int
	}{
		{"set -A v x", 0},
		{"set +A v x", 0},
		{"read -A v <<< x", 0},
		{"echo x | read -A v", 0},
		{"printf -v v x", 0},
		{"print -v v x", 0},
		{"zparseopts -a v a", 0},
		{"zparseopts -A v a", 0},
		{"read v <<< x", 1},
		{"export v=2", 1},
		// The control: the bare literal, which never named a builtin.
		{"v=(x)", 1},
	} {
		src := pre + tc.body + "; }; f; echo after"
		for _, route := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, tc.command},
			{interp.RouteScriptFile, 1},
		} {
			out, st := runZshRoute(t, t.TempDir(), src, route.route)
			if out != want {
				t.Errorf("%s from %v = %q, want %q", tc.body, route.route, out, want)
			}
			if st != route.want {
				t.Errorf("%s from %v = status %d, want %d", tc.body, route.route, st, route.want)
			}
		}
	}
}
