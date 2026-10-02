// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// **A shell started as `sh` or `ksh` has none of the parameters only zsh
// creates, and one started as `csh` lacks the module tables** (#5157).
// Measured 2026-10-01 on zsh 5.9.2 under `--emulate MODE -f -c`, byte for byte.
// Through the front end, because the emulation the shell *started* in is the
// invocation's and only the driver holds it; the last line of each row is the
// `emulate zsh` that brings nothing back.
func TestAnInvocationEmulationHidesTheParametersItDoesNotHave(t *testing.T) {
	const src = `print ${+argv} ${+ARGC} ${+path} ${+status} ${+aliases} ${+watch} ${+zsh_eval_context}
print ${(t)PATH} ${(t)KEYTIMEOUT} ${(t)ZSH_EVAL_CONTEXT}; path=(a b); print $PATH; emulate zsh; print ${+argv}`
	for _, c := range []struct{ mode, want string }{
		{"sh", "0 0 0 0 0 0 0\nscalar-export-special scalar scalar-readonly-special\n/usr/bin:/bin\n0\n"},
		{"ksh", "0 0 0 0 0 0 0\nscalar-export-special scalar scalar-readonly-special\n/usr/bin:/bin\n0\n"},
		{"csh", "1 1 1 1 0 0 1\nscalar-tied-export-special integer scalar-readonly-tied-special\na:b\n1\n"},
		{"zsh", "1 1 1 1 1 1 1\nscalar-tied-export-special integer scalar-readonly-tied-special\na:b\n1\n"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			t.Setenv("PATH", "/usr/bin:/bin")
			out, errs, code := runZsh(t, "--emulate", c.mode, "-f", "-c", src)
			if out != c.want || errs != "" || code != 0 {
				t.Errorf("got %q %q %d\nwant %q", out, errs, code, c.want)
			}
		})
	}
}
