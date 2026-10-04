// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// The older substitution's body is read the way a command string is, whatever
// route the script came by: a quote, or a nested backquote, left open in the
// body closes at the end of the body.
//
// Measured 2026-10-04 on ksh93u+ 2012-08-01 from script files, each row
// followed by `echo B`, which arrives in every row. The newer spelling's body
// is the control and is refused from a file, nothing run (#5717).
func TestABackquotedBodyIsReadAsACommandString(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo \"`echo 'abc`\"; echo B", "abc\nB\n"},
		{"echo `echo \"abc`; echo B", "abc\nB\n"},
		{"echo \"`echo \\`echo n`\"; echo B", "n\nB\n"},
		{"echo `echo \\`echo n`; echo B", "n\nB\n"},
		{"printf \"[%s]\" a \"`echo \\`echo n`\" b; echo", "[a][n][b]\n"},
	} {
		out, st := kshRouteOut(t, tc.src, syntax.RouteFromScriptFile)
		if out != tc.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", tc.src, out, st, tc.want)
		}
	}
	if out, st := kshRouteOut(t, "echo $(echo \"abc); echo B", syntax.RouteFromScriptFile); st != -1 {
		t.Errorf("the newer spelling: got %q at %d, want a refusal", out, st)
	}
}

// And it is ksh's alone; the other columns refuse every row above.
func TestABackquotedBodyIsACommandStringInKshAlone(t *testing.T) {
	if !ksh.Dialect().BackquotedBodyIsACommandString {
		t.Error("ksh: BackquotedBodyIsACommandString is off, want it on")
	}
	for name, d := range map[string]syntax.Dialect{"bash": bash.Dialect(), "zsh": zsh.Dialect(), "dash": dash.Dialect()} {
		if d.BackquotedBodyIsACommandString {
			t.Errorf("%s: BackquotedBodyIsACommandString is on, want it off", name)
		}
	}
}
