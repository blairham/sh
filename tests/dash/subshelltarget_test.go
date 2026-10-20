// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// A subshell's redirection target is this shell's word, as a command's is:
// what it assigns is still set afterwards, and a failure in it ends the
// script. Measured 2026-09-26 on `/bin/dash` 0.5.12 — `not a Go executable`
// by `go version -m` — over a script file under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME (#4695).
func TestASubshellsTargetIsThisShellsWordHere(t *testing.T) {
	t.Parallel()
	t.Run("the write is kept", func(t *testing.T) {
		t.Parallel()
		out, _ := runDash(t, t.TempDir(), "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
		if !strings.Contains(out, "u=[made]") {
			t.Errorf("out = %q, want the write kept", out)
		}
	})
	t.Run("a failure ends the script, as a group's does", func(t *testing.T) {
		t.Parallel()
		for _, cmd := range []string{"( echo RAN )", "{ echo RAN; }"} {
			out, st := runDash(t, t.TempDir(), cmd+" > $(( 1/0 ))\necho NEXT\n")
			if strings.Contains(out, "NEXT") {
				t.Errorf("%s: out = %q, want the script ended", cmd, out)
			}
			if st != 2 {
				t.Errorf("%s: status = %d, want 2", cmd, st)
			}
		}
	})
}

func TestDashSaysASubshellsTargetDoesNotExpandInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := dash.Semantics().RedirectTargetOnASubshellExpandsInTheSubshell, interp.No; got != want {
		t.Errorf("RedirectTargetOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
