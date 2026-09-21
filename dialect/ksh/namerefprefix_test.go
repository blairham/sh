// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A call's assignment prefix over a **name reference** writes the reference's
// target here too — and here the write *stays*, because this is the one column
// in the panel whose prefix in front of a POSIX-form function persists at all.
//
// Measured 2026-09-21 on ksh93u+ 2012-08-01 from a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME (#4110).
//
// This is the control the bash column needs, and the reason #4110 wanted no
// axis of its own. The two shells part on whether the write survives, and that
// is Semantics.AssignmentPrefixPersistsAfterAFunction, which this column
// already answers Yes and which a prefix with no reference in sight reaches
// just as squarely — `plain=P; ff() { :; }; plain=bar ff` leaves `plain` at
// `bar`. So the reference row is the existing axis reaching its target, and a
// second question would have been a fabricated one.
func TestAPrefixOverANameReferencePersistsOnTheTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const decl = `target=T; typeset -n foo=target; `
	for _, row := range []struct{ name, src, want string }{
		{
			"the reference is left alone",
			decl + `ff() { typeset -p foo; }; foo=bar ff`,
			`typeset -n foo=target`,
		},
		{
			"the body reads the prefix through it",
			decl + `ff() { print "[$foo]"; }; foo=bar ff`,
			`[bar]`,
		},
		{
			"and the target keeps the write after the call",
			decl + `ff() { :; }; foo=bar ff; print "[$target]"`,
			`[bar]`,
		},
		// The plain row beside it, which is what says the one above is this
		// column's persistence answer rather than anything about references.
		{
			"a prefix with no reference persists the same way",
			`plain=P; ff() { :; }; plain=bar ff; print "[$plain]"`,
			`[bar]`,
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

// The entry a **child** is handed is named for the target, exactly as it is in
// bash — so this row is unanimous across the two columns that have references
// at all, and is the core's rather than an axis. This shell handed the child
// `foo=bar`: the reference's own name, carrying a value that is the target's
// *name* everywhere else in the script (#4110).
//
// Measured 2026-09-21 on ksh93u+ 2012-08-01. Filtered in the shell rather than
// with `grep`, because the run's PATH is the scratch directory alone.
func TestAPrefixOverANameReferenceReachesAChildAsTheTarget(t *testing.T) {
	t.Parallel()
	const src = `target=T; typeset -n foo=target; ` +
		`foo=bar /usr/bin/env | while IFS= read -r l; do ` +
		`case $l in foo=*|target=*) print "$l" ;; esac; done`
	out, st := runKsh(t, t.TempDir(), src)
	const want = `target=bar`
	if got := strings.TrimSpace(out); got != want || st != 0 {
		t.Errorf("%s = %q status %d, want %q", src, got, st, want)
	}
}
