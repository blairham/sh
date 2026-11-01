// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestZparseoptsTableOverAFixedSlotIsAKindChange pins #6554: `zparseopts -A`
// over one of the shell's own parameters, or over a private, is refused with
// the sentence a `-A` letter gets, ahead of the freeze, and ends `-c` at 0
// and a script file at 1. A script's tie is taken and untied. Measured
// 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f` under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file).
func TestZparseoptsTableOverAFixedSlotIsAKindChange(t *testing.T) {
	kind := func(loc, name string) string {
		return loc + " " + name + ": can't change type of a special parameter\n"
	}
	const private = "zmodload zsh/param/private; f(){ private "
	for _, tc := range []struct{ src, want string }{
		{"zparseopts -A HOME a", kind("zsh:1:", "HOME")},
		{"zparseopts -A path a", kind("zsh:1:", "path")},
		{"zparseopts -A SECONDS a", kind("zsh:1:", "SECONDS")},
		{"readonly HOME; zparseopts -A HOME a", kind("zsh:1:", "HOME")},
		{"f(){ zparseopts -A HOME a; }; f", kind("f:", "HOME")},
		{"f(){ local HOME; zparseopts -A HOME a; }; f", kind("f:", "HOME")},
		{private + "v=1; zparseopts -A v a; }; f", kind("f:", "v")},
		{private + "-r v=1; zparseopts -A v a; }; f", kind("f:", "v")},
		// A partner the untie cannot remove is the freeze's sentence.
		{"typeset -T FOO=a:b foo; typeset -r foo; zparseopts -A FOO a", "zsh:1: read-only variable: foo\n"},
	} {
		for _, route := range []struct {
			route interp.Route
			want  int
		}{
			{interp.RouteCommandString, 0},
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

// TestZparseoptsTableTakenWhereTheSlotTakesOne is the control for the rows
// above: an association the shell already holds as one, a private declared
// one, a hidden shadow and an ordinary name each take the table, and a
// script's tie gives up its other half.
func TestZparseoptsTableTakenWhereTheSlotTakesOne(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, want string }{
		{"zparseopts -A options a; echo after $?", "after 0\n"},
		{"zparseopts -A ZSH_NAME a; echo after $?", "after 0\n"},
		{"zmodload zsh/param/private; f(){ private -A v; zparseopts -A v a; echo in $?; }; f", "in 0\n"},
		{"f(){ local -h HOME; zparseopts -A HOME a; echo in $?; }; f", "in 0\n"},
		{"typeset -T FOO=a:b foo; zparseopts -A FOO a; echo \"$? [${(t)FOO}] [${(t)foo}]\"", "0 [association] []\n"},
		{"typeset -T FOO=a:b foo; zparseopts -A foo a; echo \"$? [${(t)FOO}] [${(t)foo}]\"", "0 [] [association]\n"},
	} {
		out, _ := runZshOnPath(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
