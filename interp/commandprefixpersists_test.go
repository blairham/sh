// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `command` in front of a special builtin, and whether the prefix a script
// wrote there persists — Semantics.CommandKeepsASpecialBuiltinsPrefix. Asked
// only where a bare special builtin's prefix persists at all, which is the
// control every row below carries with it.

func commandPrefixSem(keeps Answer) Semantics {
	s := testSemantics()
	s.AssignmentPrefixPersistsOnSpecialBuiltin = Yes
	s.CommandKeepsASpecialBuiltinsPrefix = keeps
	s.CommandReachesABuiltin = Yes
	return s
}

func TestCommandCanKeepASpecialBuiltinsPrefix(t *testing.T) {
	t.Parallel()
	out, errs := builtinPrefixRun(t,
		"s=base; s=C command :; echo \"[$s]\"\nt=base; t=E command eval :; echo \"[$t]\"\nu=base; u=P :; echo \"[$u]\"",
		commandPrefixSem(Yes))
	if want := "[C]\n[E]\n[P]\n"; out != want {
		t.Errorf("got %q (err %q), want %q", out, errs, want)
	}
}

func TestCommandCanTakeASpecialBuiltinsPrefixAway(t *testing.T) {
	t.Parallel()
	out, errs := builtinPrefixRun(t,
		"s=base; s=C command :; echo \"[$s]\"\nt=base; t=E command eval :; echo \"[$t]\"\nu=base; u=P :; echo \"[$u]\"",
		commandPrefixSem(No))
	// The third row is the control: the bare special builtin's prefix
	// persists either way, so the difference is `command`'s alone.
	if want := "[base]\n[base]\n[P]\n"; out != want {
		t.Errorf("got %q (err %q), want %q", out, errs, want)
	}
}

// A **regular** builtin behind `command` asks nothing: its prefix never
// persisted, so neither answer can reach it.
func TestCommandBeforeARegularBuiltinAsksNothing(t *testing.T) {
	t.Parallel()
	for _, keeps := range []Answer{Yes, No} {
		out, _ := builtinPrefixRun(t,
			"s=base; s=Q command true; echo \"[$s]\"", commandPrefixSem(keeps))
		if want := "[base]\n"; out != want {
			t.Errorf("%v: got %q, want %q", keeps, out, want)
		}
	}
}

// And a dialect that persists a prefix and has not chosen refuses by name.
func TestCommandBeforeASpecialBuiltinIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	_, errs := builtinPrefixRun(t, "s=C command :", commandPrefixSem(Unspecified))
	if !strings.Contains(errs, "`command` in front of a special builtin") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
}
