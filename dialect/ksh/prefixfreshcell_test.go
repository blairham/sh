// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A call's assignment prefix writes the binding the name already has, kind
// and letters and all, rather than a cell of its own — which is where this
// shell and zsh part from bash. Measured 2026-09-21 on ksh93u+ 2012 from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME
// (#4087).
//
// The array row is the one bash cannot be confused with: the body reads the
// elements the name held with element zero replaced, where bash's column
// reads the prefix's word alone.
//
// One row of that measurement is deliberately absent and is not this axis.
// `typeset -i foo=7; foo=bar ff` shows the body `bar` here, where every other
// letter is kept — and hands a *child* `foo=0` where bash's column hands it
// `foo=bar`. It is neither answer and wants its own measurement.
//
// This is what pins Semantics.AssignmentPrefixMakesAFreshCell in this column:
// no corpus row puts a prefix in front of a command that *reads* a name
// carrying a kind or a letter, so the graded sweep cannot reach it.
func TestACallsPrefixWritesTheBindingItDisplaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, row := range []struct{ name, src, want string }{
		{
			"the elements are still underneath it",
			`foo=(asdf fdsa); ff() { print "[${foo[*]}] ${#foo[@]}"; }; foo=bar ff`,
			`[bar fdsa] 2`,
		},
		{
			"the case letter is still on it",
			`typeset -u foo=abc; ff() { typeset -p foo; }; foo=bAr ff`,
			`typeset -u foo=BAR`,
		},
		{
			"and it folds what the body reads",
			`typeset -u foo=abc; ff() { print "[$foo]"; }; foo=bAr ff`,
			`[BAR]`,
		},
	} {
		out, st := runKsh(t, dir, row.src)
		if st != 0 {
			t.Errorf("%s: %s answered %d: %q", row.name, row.src, st, out)
			continue
		}
		if got := strings.TrimSpace(out); got != row.want {
			t.Errorf("%s: %s = %q, want %q", row.name, row.src, got, row.want)
		}
	}
}
