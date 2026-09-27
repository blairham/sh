// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An assignment prefix in front of `exec` reaches the program that takes this
// shell's place.
//
// `exec` is the one builtin whose prefix stands in front of a **command**
// rather than in front of the shell doing something, so the replacement is
// handed the name the way an external command in that position is. Unanimous
// in the panel — measured 2026-09-26 under `env -i PATH=/usr/bin:/bin`, `V=1
// exec /usr/bin/printenv V` writes `1` in bash 5.3.20, ksh93u+ 2012-08-01,
// dash 0.5.12, zsh 5.9.2 and BusyBox ash 1.37.0 — which is why this names no
// shell and is asserted at every answer of the axis below (#4641).

// execPrefixRun runs a probe with one answer to the axis that decides what a
// builtin's *children* are told, which is the question this one is not.
func execPrefixRun(t *testing.T, src string, policy PrefixExportAtABuiltinPolicy) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.PrefixExportAtABuiltin = policy
	})
}

func TestAPrefixToExecReachesTheReplacement(t *testing.T) {
	for _, policy := range []PrefixExportAtABuiltinPolicy{
		PrefixExportAtABuiltinOn,
		PrefixExportAtABuiltinUnchanged,
		PrefixExportAtABuiltinOff,
	} {
		out, st := execPrefixRun(t, "V=1 exec /usr/bin/env\n", policy)
		if st != 0 || !strings.Contains(out, "V=1\n") {
			t.Errorf("%v: got %q status %d, want the replacement handed V=1", policy, out, st)
		}
	}
}

// The control that says the prefix is what put it there rather than the
// shell's own environment: the same replacement with nothing in front of it is
// told nothing about the name.
func TestExecWithNoPrefixHandsTheReplacementNothing(t *testing.T) {
	out, st := execPrefixRun(t, "exec /usr/bin/env\n", PrefixExportAtABuiltinUnchanged)
	if st != 0 || strings.Contains(out, "V=") {
		t.Errorf("got %q status %d, want no V in the replacement's environment", out, st)
	}
}

// And the control that separates this from the axis: a builtin's own *child*
// is told about the prefix only where the axis says the name is exported, and
// that row does not move. `eval` is the builtin with a child, and `exec` is
// not one of these rows — the replacement is the command, not a child of a
// builtin.
func TestAPrefixToAnOrdinaryBuiltinStillAsksTheAxis(t *testing.T) {
	const probe = "V=1 eval '/usr/bin/env'\n"
	for _, row := range []struct {
		policy PrefixExportAtABuiltinPolicy
		told   bool
	}{
		{PrefixExportAtABuiltinOn, true},
		{PrefixExportAtABuiltinUnchanged, false},
		{PrefixExportAtABuiltinOff, false},
	} {
		out, st := execPrefixRun(t, probe, row.policy)
		if got := strings.Contains(out, "V=1\n"); got != row.told || st != 0 {
			t.Errorf("%v: told = %v, want %v (status %d)", row.policy, got, row.told, st)
		}
	}
}
