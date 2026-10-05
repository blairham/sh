// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A function still waiting to be autoloaded is named with its file once the
// file is fixed (#6155). Measured 2026-10-05 on zsh 5.9.2 under `-f`:
//
//	autoload -Uz myf             myf is an autoload shell function
//	autoload -Uzr myg            myg is an autoload shell function from DIR/fns/myg
//	autoload -Uz DIR/fns//./myg  … from DIR/fns//./myg, kept as written
//	autoload -Uzr nosuch         nosuch is an autoload shell function
//
// and `type` writes the same sentence.
func TestAnAutoloadStubWithAFixedFileIsNamedWithIt(t *testing.T) {
	dir := t.TempDir()
	fns := filepath.Join(dir, "fns")
	if err := os.Mkdir(fns, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"myf", "myg"} {
		if err := os.WriteFile(filepath.Join(fns, n), []byte("print ran\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ src, want string }{
		{`fpath=($PWD/fns); autoload -Uz myf; whence -v myf`, "myf is an autoload shell function\n"},
		{`fpath=($PWD/fns); autoload -Uzr myg; whence -v myg`, "myg is an autoload shell function from DIR/fns/myg\n"},
		{`fpath=($PWD/fns); autoload -UzR myg; type myg`, "myg is an autoload shell function from DIR/fns/myg\n"},
		{`autoload -Uz $PWD/fns//./myg; whence -v myg`, "myg is an autoload shell function from DIR/fns//./myg\n"},
		{`fpath=($PWD/fns); autoload -Uzr nosuch; whence -v nosuch`, "nosuch is an autoload shell function\n"},
		// And once it has run, it is a function from its file as before.
		{`fpath=($PWD/fns); autoload -Uzr myg; myg; whence -v myg`, "ran\nmyg is a shell function from DIR/fns/myg\n"},
	} {
		out, _ := runZsh(t, dir, tc.src)
		if out = strings.ReplaceAll(out, dir, "DIR"); out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
