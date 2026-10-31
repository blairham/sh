// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The caller's EXIT trap a function call holds is never run when `set -e`,
// `${x?word}` or an untrapped HUP ends the shell from inside the call, or an
// `exec` that could not run does (#6952, #7007, #7065). The call's own trap
// still fires on the way out, and so does `zshexit`.
//
// Measured 2026-10-30 on zsh 5.9.2 under `-f`, by `-c`, a script file and
// standard input alike, stdout and status only — the complaint on stderr is
// located differently per route, which is not this question. The controls
// are the same stops at the top level, an error the shell reported inside a
// call (which unwinds through it and gets the trap back), `exit` inside a
// call, `posixtraps` (under which nothing is held), and errexit firing on a
// command substitution's status rather than inside the call. See
// interp.Semantics.StopInsideACallLosesTheHeldExitTrap.
func TestAStopInsideACallLosesTheExitTrapTheCallHeld(t *testing.T) {
	routes := []interp.Route{interp.RouteCommandString, interp.RouteScriptFile, interp.RouteStandardInput}
	for _, c := range []struct {
		src    string
		want   string
		status int
		// wantC is what `-c` writes where it parts from the other two
		// routes, which only the reported-error control does: there zsh
		// takes the handler away for a reason of its own (#6666).
		wantC *string
	}{
		{src: "trap 'echo T' EXIT; set -e; f() { false; }; f; echo after", status: 1},
		{src: "TRAPEXIT() { echo T }; set -e; f() { false; }; f; echo after", status: 1},
		{src: "trap 'echo T' EXIT; set -e; f() { g; }; g() { false; }; f; echo after", status: 1},
		{src: "trap 'echo T' EXIT; set -e; f() { eval false; }; f; echo after", status: 1},
		{src: "trap 'echo T' EXIT; f() { : ${q?boom}; }; f; echo after", status: 1},
		{src: `trap 'echo "trap v=$v"' EXIT; f() { v=new typeset y=${q?boom}; }; v=old; f; echo after`, status: 1},
		{src: "trap 'echo T' EXIT; f() { exec /nonexistent/x; }; f; echo after", status: 127},
		{src: "trap 'echo T' EXIT; set -e; f() { trap 'echo FT' EXIT; false; }; f; echo after", want: "FT\n", status: 1},
		{src: "trap 'echo T' EXIT; set -e; f() { setopt posixtraps; trap 'echo F' EXIT; false; }; f; echo after", want: "F\n", status: 1},
		{src: "set -e; ( trap 'echo ST' EXIT; f() { false; }; f ); echo after", status: 1},
		{src: "trap 'echo T' EXIT; f() { kill -HUP $$; }; f; echo after", status: 1},
		{src: "trap 'echo T' EXIT; f() { trap 'echo FT' EXIT; kill -HUP $$; }; f; echo after", want: "FT\n", status: 1},
		{src: "trap 'echo T' EXIT; g() { trap 'echo G' EXIT; kill -HUP $$; }; f() { trap 'echo F' EXIT; g; }; f; echo after", want: "G\n", status: 1},
		{src: "trap 'echo T' EXIT; f() { eval 'kill -HUP $$'; }; f; echo after", status: 1},
		{src: "trap 'echo T' EXIT; zshexit() { echo hook; }; f() { kill -HUP $$; }; f; echo after", want: "hook\n", status: 1},
		// Controls.
		{src: "trap 'echo T' EXIT; set -e; false; echo after", want: "T\n", status: 1},
		{src: "trap 'echo T' EXIT; : ${q?boom}; echo after", want: "T\n", status: 1},
		{src: "trap 'echo T' EXIT; f() { echo $((1/0)); }; f; echo after", want: "T\n", status: 1, wantC: new(string)},
		{src: "trap 'echo T' EXIT; f() { exit 4; }; f; echo after", want: "T\n", status: 4},
		{src: "trap 'echo T' EXIT; kill -HUP $$; echo after", want: "T\n", status: 1},
		{src: "setopt posixtraps; trap 'echo T' EXIT; f() { kill -HUP $$; }; f; echo after", want: "T\n", status: 1},
		{src: "setopt posixtraps; trap 'echo T' EXIT; set -e; f() { false; }; f; echo after", want: "T\n", status: 1},
		{src: "trap 'echo T' EXIT; set -e; f() { false; }; x=$(f); echo after", want: "T\n", status: 1},
	} {
		for _, route := range routes {
			want := c.want
			if route == interp.RouteCommandString && c.wantC != nil {
				want = *c.wantC
			}
			out, st, _ := runZshSplitOnRoute(t, route, c.src)
			if out != want || st != c.status {
				t.Errorf("%v %q: wrote %q at %d, want %q at %d", route, c.src, out, st, want, c.status)
			}
		}
	}
}
