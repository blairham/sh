// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/driver"
)

// ksh93 has the one-command option under its letter alone: `set -t` stops the
// shell after the line it was set on, and `set -o onecmd` is `bad option(s)`
// here because this shell has no long name for it. Measured 2026-09-10.
//
// So the letter writes the state directly rather than through the `set -o`
// table, and that is what `Semantics.SetHasTheTLetter` turns on — it was in
// this dialect's unimplemented-letter list while the option behind it was
// wired for bash, which is a table saying the opposite of what the shell does.

// kshShell is the value cmd/ksh assembles, minus the prompt wiring.
func kshShell(out, errs *bytes.Buffer) driver.Shell {
	return driver.Shell{
		Name:        "ksh",
		Dialect:     ksh.Dialect(),
		Semantics:   ksh.Semantics(),
		Diagnostics: ksh.Diagnostics(),
		Prelude:     ksh.Prelude(),
		Register:    ksh.Apply,
		Stdout:      out,
		Stderr:      errs,
	}
}

func TestKshTakesTheOneCommandLetter(t *testing.T) {
	script := func(src string) string {
		path := filepath.Join(t.TempDir(), "s.sh")
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, c := range []struct {
		name string
		argv []string
		want string
	}{
		// From a file: the line that set it finishes and nothing more is read.
		{"a script stops", []string{"ksh", script("echo A\nset -t\necho B\n")}, "A\n"},
		{"the line finishes first", []string{"ksh", script("set -t; echo B\necho C\n")}, "B\n"},
		{"the invocation", []string{"ksh", "-t", script("echo A\necho B\n")}, "A\n"},
		// And the route that separates this shell from bash: ksh93 stops a
		// command string too, where bash reads on through it.
		{"a command string stops here", []string{"ksh", "-c", "set -t\necho B\n"}, ""},
		{"the line still finishes", []string{"ksh", "-c", "set -t; echo B"}, "B\n"},
		// The state is visible, under the same letter bash shows.
		{"the letter is in $-", []string{"ksh", "-c", "set -t; case $- in *t*) echo has-t;; *) echo no-t;; esac"}, "has-t\n"},
		// And it is not one-way: the line that set it can take it back.
		{"taken back on the same line", []string{"ksh", script("set -t; set +t\necho C\n")}, "C\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(kshShell(&out, &errs), c.argv)
			if out.String() != c.want || errs.String() != "" || code != 0 {
				t.Errorf("%v = %q / %q status %d, want %q and nothing said",
					c.argv, out.String(), errs.String(), code, c.want)
			}
		})
	}
}

// TestKshHasNoLongNameForTheOneCommandOption is the other half of the pairing,
// and the reason the letter is not routed through the `set -o` table: the name
// bash lists is not a name here at all.
func TestKshHasNoLongNameForTheOneCommandOption(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(kshShell(&out, &errs), []string{"ksh", "-c", "set -o onecmd; echo NO"})
	if code == 0 || out.String() != "" || errs.String() == "" {
		t.Errorf("set -o onecmd = %q / %q status %d, want it refused before anything runs",
			out.String(), errs.String(), code)
	}
}
