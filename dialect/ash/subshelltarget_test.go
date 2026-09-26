// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// A subshell's redirection target is this shell's word, as a command's is.
// Measured 2026-09-26 in the pinned image
// `alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b`
// — `BusyBox v1.37.0 (2026-01-10 15:38:28 UTC) multi-call binary` — over a
// script file under `env -i PATH=/usr/bin:/bin` with a scratch HOME:
// `( : ) > "${u:=made}"` leaves `u` set, and `( echo RAN ) > $(( 1/0 ))` ends
// the script at 2 exactly as a group does (#4695).
func TestASubshellsTargetIsThisShellsWordHere(t *testing.T) {
	t.Parallel()
	t.Run("the write is kept", func(t *testing.T) {
		t.Parallel()
		out, _ := runIn(t, "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
		if !strings.Contains(out, "u=[made]") {
			t.Errorf("out = %q, want the write kept", out)
		}
	})
	t.Run("a failure ends the script, as a group's does", func(t *testing.T) {
		t.Parallel()
		for _, cmd := range []string{"( echo RAN )", "{ echo RAN; }"} {
			out, st := runIn(t, cmd+" > $(( 1/0 ))\necho NEXT\n")
			if strings.Contains(out, "NEXT") {
				t.Errorf("%s: out = %q, want the script ended", cmd, out)
			}
			if st != 2 {
				t.Errorf("%s: status = %d, want 2", cmd, st)
			}
		}
	})
}

func TestAshSaysASubshellsTargetDoesNotExpandInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := ash.Semantics().RedirectTargetOnASubshellExpandsInTheSubshell, interp.No; got != want {
		t.Errorf("RedirectTargetOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
