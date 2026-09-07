// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two of the tables a subshell owns are only observable through this dialect,
// because it is the one that says where a function came from and the one that
// carries a function in a command's environment (#1125).
//
// Here rather than in interp for that reason and no other: the property is the
// core's — the panel is unanimous — but the *view* of it is bash's, and a
// change to `Runner.clone` that nothing could see was a change nothing could
// hold. Both of these survived the first mutation run.

// TestASubshellDoesNotLoseWhereTheParentsFunctionCameFrom is the record of a
// function's defining file.
//
// It moves with the function table and had to, in the direction that is easy
// to miss: with the definitions copied and the *files* still shared, a subshell
// that does `unset -f f` leaves the parent holding `f` and takes away the note
// of where `f` came from. The function then still runs and reports its source
// as nothing — a plausible answer, at status 0, on the surface a trace reads.
func TestASubshellDoesNotLoseWhereTheParentsFunctionCameFrom(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib.sh")
	if err := os.WriteFile(lib, []byte("f(){ echo \"src=${BASH_SOURCE[0]}\"; }\n"), 0o600); err != nil {
		t.Fatalf("writing the library: %v", err)
	}
	out, st := runBash(t, dir, ". "+lib+"\n(unset -f f)\nf")
	want := "src=" + lib + "\n"
	if out != want || st != 0 {
		t.Errorf("after a subshell unset the name = %q (status %d), want %q", out, st, want)
	}
}

// TestASubshellsFunctionExportDoesNotEscape is the other one: which functions
// are written into a *command's* environment.
//
// A child's environment is the only place it is observable, and that is not a
// detour — it is the whole of what exporting a function means. It is not a
// shell variable (`${!BASH_FUNC@}` is empty in bash 5.3.15 with one exported,
// checked), and this shell's `export -p -f` prints nothing yet.
//
// The command is `env` itself, by absolute path, and **not** a script that
// runs it. That is the whole reason this test is shaped as it is: the entry
// is called `BASH_FUNC_g%%`, which is not a valid variable name, and a POSIX
// shell in between drops it on import. A `#!/bin/sh` probe passed on macOS,
// where /bin/sh is bash and keeps its own entry, and failed on Linux, where
// it is dash and does not — a platform split that says nothing about the
// shell under test.
//
// Measured against bash 5.3.15: `g(){ :; }; export -f g` puts exactly one
// such entry in a command's environment and `(export -f g)` puts none. The
// second row is the control — without it this would pass in a shell that had
// stopped exporting functions at all.
func TestASubshellsFunctionExportDoesNotEscape(t *testing.T) {
	const env = "/usr/bin/env"
	if _, err := os.Stat(env); err != nil {
		t.Skipf("no %s here: %v", env, err)
	}
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"exported in a subshell", "g(){ :; }\n(export -f g)\n" + env, 0},
		{"exported at the top level", "g(){ :; }\nexport -f g\n" + env, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), tc.src)
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			got := 0
			for _, line := range strings.Split(out, "\n") {
				if strings.HasPrefix(line, "BASH_FUNC") {
					got++
				}
			}
			if got != tc.want {
				t.Errorf("a command's environment carried %d exported function(s), want %d", got, tc.want)
			}
		})
	}
}
