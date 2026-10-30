// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// The EXIT trap after a `-c` string that ran out is located at line 1, where a
// script file or standard input that ran out locates it past the end (#6953);
// and a `return` that ends the script leaves it on the `return`'s line on
// every route, as an `exit` does.
//
// Measured 2026-10-30 on zsh 5.9.2 under `-f`, with the three-line body the
// neighboring test uses — its second line fails and its third writes
// `$LINENO`; the file and standard-input columns are those routes over the
// same text with its final newline, which the harness's whole-text parse
// stands for when the newline is left off, for the reason
// TestASubshellsExitTrapIsLocatedWhereTheSubshellOpened gives. Standard input
// is held to `$LINENO` alone: this harness does not word that route's
// diagnostics, and zsh names no line there anyway.
//
// Not a row: a string whose last command is an `if`, a `for`, a `case` or a
// brace group is located at that command's own line in zsh, which runs the
// trap inside it — under its redirections as well. That is a question of its
// own and is not modeled.
func TestACommandStringThatRanOutLocatesItsExitTrapAtLineOne(t *testing.T) {
	const body = "trap \":\nnosuch_t\necho L=\\$LINENO\" EXIT\n"
	for _, c := range []struct {
		name, src string
		// c, file and stdin are the line each route counts the trap at.
		c, file, stdin string
	}{
		{"running out", ":\n" + body + ":\n:\n:", "1", "8", "8"},
		{"the trap alone", body[:len(body)-1], "1", "4", "4"},
		{"an exit, the control", ":\n" + body + ":\nexit 0\n:", "6", "6", "6"},
		{"a return at the top", ":\n" + body + ":\nreturn 0\n:", "6", "6", "6"},
		{"a subshell's return", ":\n(\n" + body + ":\nreturn 0\n:\n)\n:", "7", "7", "7"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, r := range []struct {
				route interp.Route
				line  string
				errs  string
			}{
				{interp.RouteCommandString, c.c, "zsh:" + c.c + ": command not found: nosuch_t\n"},
				{interp.RouteScriptFile, c.file, "zsh:" + c.file + ": command not found: nosuch_t\n"},
				{interp.RouteStandardInput, c.stdin, ""},
			} {
				out, _, errs := runZshSplitOnRoute(t, r.route, c.src)
				if r.errs == "" {
					errs = ""
				}
				if want := "L=" + r.line + "\n"; out != want || errs != r.errs {
					t.Errorf("%v: wrote %q and %q, want %q and %q", r.route, out, errs, want, r.errs)
				}
			}
		})
	}
}
