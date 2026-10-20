// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestFunctionsCCopiesAFunction pins `functions -c`. Measured 2026-10-02 on
// zsh 5.9.2 under `-f` (#5148). See functionsCopying.
func TestFunctionsCCopiesAFunction(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "auto"), []byte("\nprint -P \"%N:%i %1x:%I\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pre := "fpath=(" + dir + "); "
	cases := []struct{ src, want string }{
		{"tbc() { print called $0 }; functions -c tbc nc; nc; unfunction tbc; nc", "called nc\ncalled nc\n"},
		{"autoload -Uz auto; functions -c auto cp; cp", "cp:2 auto:2\n"},
		{"functions -c nosuch x; print st=$?", "zsh:functions:1: no such function: nosuch\nst=1\n"},
		{"f(){ :; }; functions -c f; print st=$?", "zsh:functions:1: -c: requires two arguments\nst=1\n"},
		{"autoload -Uz cant; functions -c cant x; print st=$?", "zsh:1: cant: function definition file not found\nst=1\n"},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, dir, pre+c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
