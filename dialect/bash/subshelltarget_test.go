// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// This column reads "already right" on the coarse question — the script is
// alive on the next line either way — and it was not. A `( … )` is a process
// of its own here, so a failed target costs the parentheses: the rest of the
// **same line** runs and `||` catches it, where a group with the same target
// gives up the line and `||` never sees it.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash` — `GNU bash, version
// 5.3.20(1)-release (aarch64-apple-darwin25.6.0)`, `not a Go executable` by
// `go version -m` — over a script file under `env -i PATH=/usr/bin:/bin` with
// a scratch HOME.
//
//	`> $(( 1/0 ))` on …   rest of the line   `||`    next line
//	( echo RAN )          runs               catches st=1
//	{ echo RAN; }         does not run       no      st=1
//
// So `||` is a column of its own and not a restatement of the reach: a probe
// that asked only whether the next line runs reports this shell as agreeing
// when it does not (#4695).

func TestASubshellsFailedTargetCostsOnlyTheParenthesesHere(t *testing.T) {
	t.Parallel()
	t.Run("the rest of its own line runs", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) ; echo SAME\necho NEXT\n")
		if strings.Contains(out, "RAN") {
			t.Errorf("out = %q, want the subshell left unrun", out)
		}
		if !strings.Contains(out, "SAME") || !strings.Contains(out, "NEXT") {
			t.Errorf("out = %q, want the rest of the line and the next line", out)
		}
	})
	t.Run("|| catches it", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "( echo RAN ) > $(( 1/0 )) || echo CAUGHT\n")
		if !strings.Contains(out, "CAUGHT") {
			t.Errorf("out = %q, want the failure caught", out)
		}
	})
	t.Run("the write the target made is lost", func(t *testing.T) {
		t.Parallel()
		out, _ := runBash(t, t.TempDir(), "unset u\n( : ) > \"${u:=made}\"\necho \"u=[${u-unset}]\"\n")
		if !strings.Contains(out, "u=[unset]") {
			t.Errorf("out = %q, want the assignment confined to the subshell", out)
		}
	})
}

// The control on the other side of the same line: a group's failed target
// gives up the line here, so `||` does not see it and the marker after the
// `;` never runs. That is what says the parentheses decide.
func TestAGroupsFailedTargetStillGivesUpItsLineHere(t *testing.T) {
	t.Parallel()
	out, _ := runBash(t, t.TempDir(), "{ echo RAN; } > $(( 1/0 )) ; echo SAME\necho \"after st=$?\"\n")
	if strings.Contains(out, "SAME") {
		t.Errorf("out = %q, want the rest of the line given up", out)
	}
	if !strings.Contains(out, "after st=1") {
		t.Errorf("out = %q, want the next line at 1", out)
	}
}

func TestBashSaysASubshellsTargetExpandsInTheSubshell(t *testing.T) {
	t.Parallel()
	if got, want := bash.Semantics().RedirectTargetOnASubshellExpandsInTheSubshell, interp.Yes; got != want {
		t.Errorf("RedirectTargetOnASubshellExpandsInTheSubshell = %v, want %v", got, want)
	}
}
