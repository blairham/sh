// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestC04FuncdefFronts holds the C04funcdef fronts this change closes
// (#5148). Every row measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`).
func TestC04FuncdefFronts(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"yes", "no"} {
		if err := os.WriteFile(filepath.Join(dir, f), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct{ src, want string }{
		// interp.Semantics.CommandNotFoundHandler.
		{`command_not_found_handler() { x=set; print "H:$*" }; nosuchcmd a b; echo st=$? x=$x`, "H:nosuchcmd a b\nst=0 x=\n"},
		{`command_not_found_handler() { return 5 }; nosuchcmd; echo st=$?`, "st=5\n"},
		{`command_not_found_handler() { x=set; return 127 }; nosuchcmd a; echo st=$?`, "st=127\n"},
		{`command_not_found_handler() { print "Z=$ZSH_SUBSHELL fs=$funcstack" }; nosuch`, "Z=1 fs=command_not_found_handler\n"},
		{`command_not_found_handler() { print H }; ./nosuchpath; echo st=$?`, "zsh:1: no such file or directory: ./nosuchpath\nst=127\n"},
		{`command_not_found_handler() { print H:$1 }; f() { nosuchcmd x; echo st=$? }; f`, "H:nosuchcmd\nst=0\n"},
		{`command_not_found_handler() { print H:$1 }; eval "nosuchcmd x"; echo st=$?`, "H:nosuchcmd\nst=0\n"},
		// A nameless function lists as `() {` whichever word wrote it.
		{
			`f() { function { : } a; () { : }; function { : } }; functions f`,
			"f () {\n\t() {\n\t\t:\n\t} a\n\t() {\n\t\t:\n\t}\n\t() {\n\t\t:\n\t}\n}\n",
		},
		{`g() { function }; functions g`, "g () {\n\t() {\n\t\t\n\t}\n}\n"},
		// And its call's words are read the way a command's arguments are.
		{`() { echo $1 } (y|z)*`, "yes\n"},
		{`() { echo $* } some (y|z)*`, "some yes\n"},
		{`function { echo $1 } (y|z)*`, "yes\n"},
		{`() { { echo in } } (y|z)*`, "in\n"},
		{`() { echo empty };(echo here)`, "empty\nhere\n"},
	} {
		out, _ := runZsh(t, dir, c.src+"\n")
		if out != c.want {
			t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
		}
	}
}
