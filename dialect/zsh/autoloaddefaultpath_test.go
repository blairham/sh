// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAutoloadDFallsBackToFpath pins `autoload -d` and where a hand-written
// `autoload -X` that finds nothing is reported. Measured 2026-10-02 on zsh
// 5.9.2 under `-f` (#5148).
func TestAutoloadDFallsBackToFpath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "def"), []byte("print loaded by default path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pre := "cd " + dir + "; fpath=(.); "
	cases := []struct{ src, want string }{
		{"autoload -dUz $PWD/extra/def; def", "loaded by default path\n"},
		{"autoload -Uz $PWD/extra/def; def", "zsh:1: def: function definition file not found\n"},
		{"def() { autoload -dXUz $PWD/extra; }; def", "loaded by default path\n"},
		{"def() { autoload -XUz $PWD/extra; }; def", "(eval):1: def: function definition file not found\n"},
		{"cod() { print hi; autoload -X }; cod", "hi\n(eval):1: cod: function definition file not found\n"},
		{"autoload -dUz /x/def; functions def", "def () {\n\t# undefined\n\tbuiltin autoload -XUzc /x\n}\n"},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, dir, pre+c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// TestAutoloadAgainKeepsWhatTheStubHad pins a declaration of a name already
// waiting. Measured 2026-10-02 on zsh 5.9.2 under `-f` (#5148). See
// autoloadMergeStub.
func TestAutoloadAgainKeepsWhatTheStubHad(t *testing.T) {
	cases := []struct{ src, want string }{
		{"autoload -Uz /p/spec; autoload spec; functions spec", "spec () {\n\t# undefined\n\tbuiltin autoload -XUz /p\n}\n"},
		{"autoload -Uz q; autoload -k q; functions q", "q () {\n\t# undefined\n\tbuiltin autoload -XUk\n}\n"},
		{"autoload -k w; autoload -z w; functions w", "w () {\n\t# undefined\n\tbuiltin autoload -Xz\n}\n"},
	}
	for _, c := range cases {
		if got, _ := runZshOnPath(t, t.TempDir(), c.src); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}
