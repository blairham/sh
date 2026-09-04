// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// complete keeps specs it never acts on: a completion file run without a
// terminal registers, lists, and removes them, and observes nothing else.

func TestCompleteRegistersAndReadsBack(t *testing.T) {
	out, st := run(t, `complete -W "a b" foo; complete -F _f bar; complete -p foo; complete`, nil)
	if st != 0 {
		t.Fatalf("status %d (out %q)", st, out)
	}
	if !strings.Contains(out, `complete -W 'a b' foo`) {
		t.Errorf("out = %q, want the spec printed back the way it was written", out)
	}
	if !strings.Contains(out, "complete -F _f bar") {
		t.Errorf("out = %q, want the bare listing to carry every spec", out)
	}
}

func TestCompleteRemovesAndReportsAMiss(t *testing.T) {
	out, st := run(t, `complete -W x foo; complete -r foo; echo "r=$?"; complete -p foo; echo "p=$?"`, nil)
	if !strings.Contains(out, "r=0") || !strings.Contains(out, "p=1") {
		t.Errorf("out = %q (st %d), want removal to succeed and the lookup to miss", out, st)
	}
	if !strings.Contains(out, "no completion specification") {
		t.Errorf("out = %q, want the miss named", out)
	}
}

func TestCompleteIsRemovable(t *testing.T) {
	out, st := run(t, `complete -W x foo`, func(r *Runner) { r.Unregister("complete") })
	if st == 0 {
		t.Errorf("status 0 (out %q), want the dialect without the builtin to refuse it", out)
	}
}

func TestCompleteSurvivesASubshellCopy(t *testing.T) {
	out, _ := run(t, `complete -W x foo; (complete -W y bar); complete`, nil)
	if !strings.Contains(out, "foo") || strings.Contains(out, "bar") {
		t.Errorf("out = %q, want the parent's spec kept and the subshell's its own", out)
	}
}
