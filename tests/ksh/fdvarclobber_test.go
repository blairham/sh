// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// `set -C` protects files here too and not the name a `{var}` redirection
// writes into — the second No column for
// interp.Semantics.NoclobberProtectsAnFdVariable.
//
// Measured 2026-09-29 on ksh93u+ (`/bin/ksh`), a script file in a fresh
// directory:
//
//	set -C; exec {m}>f1; exec {m}>f2; echo "st=$? m=$m"
//	    st=0 m=11, nothing on standard error
//
// Two No columns rather than one because the axis is asked on a path every
// `{name}` redirection reaches, and a second column is what catches a rule
// that was written to the one shell it was measured on.
func TestNoclobberDoesNotProtectAnFdVariableHereEither(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "set -C\nexec {m}>f1\nexec {m}>f2\necho \"st=$? m=$m\"\n")
	if !strings.Contains(out, "st=0") {
		t.Errorf("said %q, want the redirection to succeed", out)
	}
	if strings.Contains(out, "clobber") {
		t.Errorf("said %q, want no refusal about clobbering a parameter", out)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}
