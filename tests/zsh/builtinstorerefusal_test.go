// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestARefusedStoreThroughPrintOrZparseoptsLeavesZeroFromACommandString pins
// that a frozen name, or a fixed scalar slot, refusing what `print -v`,
// `printf -v`, `zparseopts -a` or `zparseopts -A` would store there is fatal
// and ends `-c` at 0, the number `set -A` and `read -A` already leave, where
// a script file's top level raises it to 1. `zparseopts -A` was not refused
// at all. Every row measured 2026-10-15 against zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f` under `env -i PATH=/usr/bin:/bin`, from `-c`
// and from a script file), the sentence compared byte for byte (#6513).
func TestARefusedStoreThroughPrintOrZparseoptsLeavesZeroFromACommandString(t *testing.T) {
	t.Parallel()
	const ro = "zsh:1: read-only variable: v\n"
	for _, tc := range []struct {
		src, want string
		command   int
	}{
		{"typeset -r v=1; print -v v x", ro, 0},
		{"typeset -r v=1; printf -v v x", ro, 0},
		{"typeset -r v=1; zparseopts -a v a", ro, 0},
		{"typeset -r v=1; zparseopts -A v a", ro, 0},
		{"typeset -r v=1; set -- -a; zparseopts -K -A v a", ro, 0},
		{"typeset -rA v; zparseopts -A v a", ro, 0},
		{"zparseopts -a HOME a", "zsh:1: HOME: attempt to assign array value to non-array\n", 0},
		// The raisers: `||` leaves the 0, `&&` and a loop raise it.
		{"typeset -r v=1; printf -v v x || echo no", ro, 0},
		{"typeset -r v=1; zparseopts -A v a && echo yes", ro, 1},
		{"typeset -r v=1; for i in 1; do print -v v x; done", ro, 1},
		// The control: `read`'s scalar store is 1 by both routes.
		{"typeset -r v=1; read v <<< x", ro, 1},
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
}

// TestAZparseoptsStoreNothingRefusesNothing is the control for the rows
// above: with `-K` and nothing matched there is no store, so a frozen name
// says nothing and the script carries on, and an unfrozen name is filled.
func TestAZparseoptsStoreNothingRefusesNothing(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{"typeset -r v=1; zparseopts -K -A v a; echo after $?", "after 0\n"},
		{"typeset -r v=1; zparseopts -K -a v a; echo after $?", "after 0\n"},
		{"set -- -a; zparseopts -A m a; typeset -p m", "typeset -A m=( [-a]='' )\n"},
	} {
		out, _ := runZshOnPath(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
