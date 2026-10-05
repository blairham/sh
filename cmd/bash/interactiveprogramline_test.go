// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// Under `-i`, a fatal error in the `-c` string or in a script costs only its
// line, and the next line runs (#6073). See
// interp.Semantics.InteractiveProgramErrorWhenInteractive.
//
// Measured 2026-10-05 on bash 5.3.20, `env -i` with a scratch HOME and
// stdin /dev/null: `after 1` on every row, by both routes, and never `same`.
// This shell ended the program.
func TestAnInteractiveProgramErrorCostsTheLine(t *testing.T) {
	for _, line := range []string{"echo ${u?b}", "set -u; echo $nope", "readonly r=1; r=2", "f(){ echo ${u?b}; }; f"} {
		program := line + "; echo same\necho after $?\n"
		for _, argv := range [][]string{
			{"bash", "--norc", "-i", "-c", program},
			{"bash", "--norc", "-i", "s.sh"},
		} {
			home := scratchHome(t)
			t.Chdir(home)
			writeHomeFile(t, home, "s.sh", program)
			out, errs, code := prompt(t, "", argv...)
			if !strings.Contains(out, "after 1\n") || strings.Contains(out, "same") || code != 0 {
				t.Errorf("%q, %s: stdout %q status %d, want `after 1` and 0 (stderr %q)", line, argv[3], out, code, errs)
			}
		}
	}
}
