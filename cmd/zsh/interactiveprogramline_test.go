// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Under `-i`, a fatal error in a script costs only its line in zsh, except
// `${x?word}`, which still ends it; a `-c` string ends as without `-i` (#6073).
// See interp.Semantics.InteractiveProgramErrorWhenInteractive.
//
// Measured 2026-10-05 on zsh 5.9.2 (`zsh -f`), `env -i` with a scratch HOME
// and stdin /dev/null. This shell ended the script on every row.
func TestAnInteractiveScriptErrorCostsTheLine(t *testing.T) {
	for _, c := range []struct {
		line  string
		after bool
	}{
		{"set -u; echo $nope", true},
		{"readonly r=1; r=2", true},
		{"echo ${u?b}", false},
	} {
		program := c.line + "; echo same\necho after $?\n"
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Chdir(home)
		if err := os.WriteFile(filepath.Join(home, "s.sh"), []byte(program), 0o600); err != nil {
			t.Fatal(err)
		}
		out, errs, _ := prompt(t, "", "zsh", "-f", "-i", "s.sh")
		if got := strings.Contains(out, "after 1\n"); got != c.after || strings.Contains(out, "same") {
			t.Errorf("%q, -i s.sh: stdout %q, want `after 1` %v (stderr %q)", c.line, out, c.after, errs)
		}
		out, errs, _ = prompt(t, "", "zsh", "-f", "-i", "-c", program)
		if strings.Contains(out, "after") {
			t.Errorf("%q, -i -c: stdout %q, want the string ended (stderr %q)", c.line, out, errs)
		}
	}
}
