// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestPrintExitValueReportsAFailedBuiltinOrFunction pins `printexitvalue`.
// Measured 2026-10-02 on zsh 5.9.2, `zsh -f` reading the script from
// standard input (#5155). See interp.Runner.reportExitValue for the panel.
func TestPrintExitValueReportsAFailedBuiltinOrFunction(t *testing.T) {
	const on = "setopt printexitvalue\n"
	cases := []struct{ src, errs string }{
		{"false", "zsh: exit 1\n"},
		{"f(){ false; true; }; f", ""},
		{"f(){ return 4 }; f", "zsh: exit 4\n"},
		{"() { false; }", "zsh: exit 1\n"},
		{"eval false", "zsh: exit 1\nzsh: exit 1\n"},
		{"builtin false", "zsh: exit 1\n"},
		{"command /usr/bin/false", ""},
		{"command -v nosuch", "zsh: exit 1\n"},
		{`eval "command /usr/bin/false"`, "zsh: exit 1\n"},
		{"/usr/bin/false", ""},
		{"( false )", ""},
		{"[[ a = b ]]", ""},
		{"false 2>/dev/null", ""},
		{"exit 5", ""},
		{"TRAPEXIT(){ false }; true", ""},
		{"TRAPUSR1(){ false }; kill -USR1 $$; true", ""},
		{`trap "false" USR1; kill -USR1 $$; true`, "zsh: exit 1\n"},
		{`trap "false" EXIT; true`, "zsh: exit 1\n"},
	}
	for _, c := range cases {
		_, _, errs := runZshSplitOnRoute(t, interp.RouteStandardInput, on+c.src)
		if errs != c.errs {
			t.Errorf("%s\n got %q\nwant %q", c.src, errs, c.errs)
		}
	}
	// And nothing at all from a command string.
	if _, _, errs := runZshSplitOnRoute(t, interp.RouteCommandString, on+"false"); errs != "" {
		t.Errorf("-c: got %q, want nothing", errs)
	}
}
