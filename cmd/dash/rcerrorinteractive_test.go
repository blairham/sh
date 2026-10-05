// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fatal error in an interactive shell's startup file (#6009). See
// interp.Semantics.StartupFileErrorWhenInteractive.
//
// Measured 2026-10-05, the rc holding the failing line and then `echo after`,
// `-i` with `echo X $?` on a pipe and `-i -c 'echo main $?'`: dash 0.5.12 gives up the rest of `$ENV` — `${x?word}` included, which ended the session here — and draws its prompt, `$?` 2.
func TestAnInteractiveRcFileErrorCostsWhatTheShellSays(t *testing.T) {
	for _, line := range []string{
		"echo ${unset?boom}",
		"readonly r=1; r=2; echo same",
		"set -u; echo $NOPE; echo same",
		"echo $((1+))",
	} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		rc := filepath.Join(home, "rc")
		t.Setenv("ENV", rc)
		if err := os.WriteFile(rc, []byte(line+"\necho after\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, route := range []struct {
			typed, want string
			argv        []string
		}{
			{"echo X $?\n", "X 2", []string{"dash", "-i"}},
			{"", "main 2", []string{"dash", "-i", "-c", "echo main $?"}},
		} {
			out, errs, _ := prompt(t, route.typed, route.argv...)
			if !strings.Contains(out, route.want) {
				t.Errorf("%q, %v: stdout %q, want %q (stderr %q)", line, route.argv, out, route.want, errs)
			}
			if strings.Contains(out, "after") {
				t.Errorf("%q, %v: stdout %q ran the file's next line, which this shell gives up", line, route.argv, out)
			}
			if strings.Contains(out, "same") {
				t.Errorf("%q, %v: stdout %q ran the rest of the failing line", line, route.argv, out)
			}
		}
	}
}
