// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/wild"
)

// Two scripts that fail on the same construct are one finding. The grouping
// is the whole point of the report: a reader looking at a list of paths cannot
// tell forty problems from one, and the sweep is the only thing that can.
func TestCausesGroupsAndRanks(t *testing.T) {
	dir := t.TempDir()
	// Three scripts sharing one gap, and one with a different gap.
	for _, name := range []string{"a", "b", "c"} {
		// A construct this parser does not have and is not about to: the
		// completion-context conditions are recorded in
		// docs/spec/grammar/conditions.md as a decision rather than a gap.
		// It was `[[ $k == (x|y) ]]` until that was implemented (#826),
		// which is the hazard a fixture like this has: it has to be
		// something the parser still refuses.
		write(t, dir, name, "#!/bin/zsh\n[[ -prefix - ]]\n")
	}
	write(t, dir, "d", "#!/bin/zsh\nrepeat 3 { echo x; }\n")

	rep := wild.Sweep(context.Background(), wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope},
		zsh.Dialect(), "/usr/bin/true")
	if len(rep.Failures) != 4 {
		t.Fatalf("failures = %d, want 4: %+v", len(rep.Failures), rep.Failures)
	}

	causes := wild.Causes(rep.Failures)
	if len(causes) != 2 {
		t.Fatalf("causes = %d, want 2: %+v", len(causes), causes)
	}
	if causes[0].Count() != 3 {
		t.Errorf("the largest cause accounts for %d scripts, want 3", causes[0].Count())
	}
	if causes[1].Count() != 1 {
		t.Errorf("the second cause accounts for %d scripts, want 1", causes[1].Count())
	}
	// A cause carries a line a person can look at, not only a category.
	if causes[0].Example.Text == "" {
		t.Error("the example carries no source line")
	}
}

// A reason keeps what came from the grammar's vocabulary and drops what came
// from the script's, so that two scripts refused at different identifiers are
// one cause rather than two.
func TestReasonDropsWhatDiffersPerScript(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "one", "#!/bin/zsh\nif true { install_deps; }\n")
	write(t, dir, "two", "#!/bin/zsh\nif true { build_all; }\n")

	rep := wild.Sweep(context.Background(), wild.Scope{Dirs: []string{dir}, Shells: wild.ZshScope},
		zsh.Dialect(), "/usr/bin/true")
	if len(rep.Failures) != 2 {
		t.Fatalf("failures = %d, want 2: %+v", len(rep.Failures), rep.Failures)
	}
	if got := wild.Causes(rep.Failures); len(got) != 1 {
		t.Errorf("causes = %d, want 1 — the two differ only in a name: %+v", len(got), got)
	}
	// And the position is gone, so the same construct on two different lines
	// is still one cause.
	if a, b := wild.Reason(rep.Failures[0].Err), wild.Reason(rep.Failures[1].Err); a != b {
		t.Errorf("reasons %q and %q differ", a, b)
	}
}

// The line a failure sits on is the closest thing to a minimal reproduction
// the sweep can honestly offer, so it has to survive being asked for out of
// range rather than taking the report down with it.
func TestLineAt(t *testing.T) {
	const src = "first\n  second  \nthird"
	for _, tc := range []struct {
		line int
		want string
	}{
		{1, "first"},
		{2, "second"},
		{3, "third"},
		{0, ""},
		{-1, ""},
		{4, ""},
	} {
		if got := wild.LineAt(src, tc.line); got != tc.want {
			t.Errorf("LineAt(%d) = %q, want %q", tc.line, got, tc.want)
		}
	}
}
