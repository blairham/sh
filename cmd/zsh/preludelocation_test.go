// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAPreludeFunctionReportsWhereItWasCalled is #5447. A prelude function,
// such as `popd` or `dirs`, is the shell's builtin written as shell. Its
// complaint is located where the script called it, as a builtin's would be.
// It used to be located inside the function's own frame. On `-c` that named
// the binary's path, and inside a caller's function it named the file and
// the prelude's own line.
//
// Invoked through a path, so `$0` and the shell's own name are different
// strings. Every row measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`).
func TestAPreludeFunctionReportsWhereItWasCalled(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"popd", "zsh:popd:1: directory stack empty\n"},
		{"dirs -q", "zsh:dirs:1: bad option: -q\n"},
		{"f() { popd; }; f", "f:popd: directory stack empty\n"},
		{"f() {\ndirs -q\n}\nf", "f:dirs:1: bad option: -q\n"},
		{"g() { f; }; f() { popd; }; g", "f:popd: directory stack empty\n"},
	} {
		var out, errs strings.Builder
		sh := scratchShell(t)
		sh.Stdout, sh.Stderr = &out, &errs
		driver.MainArgs(sh, []string{"/some/where/zsh", "-f", "-c", c.src})
		if errs.String() != c.want {
			t.Errorf("%q: stderr %q, want %q", c.src, errs.String(), c.want)
		}
	}
	// And from a script file, where the top level names the file.
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, "pp.zsh"), []byte("f() { popd; }\nf\npopd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, errs, _ := runZsh(t, "-f", "pp.zsh")
	if want := "f:popd: directory stack empty\npp.zsh:popd:3: directory stack empty\n"; errs != want {
		t.Errorf("a script file: stderr %q, want %q", errs, want)
	}
}
