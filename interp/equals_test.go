// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// equalsSem answers the axis by name. Which preset answers Yes is the
// dialect packages' claim, not this one's.
func equalsSem(a Answer) Semantics {
	s := permissive()
	s.EqualsExpansion = a
	return s
}

func TestEqualsExpansionIsAnAxis(t *testing.T) {
	// `echo =ls` prints a path where the axis is on and the literal text
	// where it is off, with nothing reported either way — the `&>` failure
	// mode in an expansion.
	if got, _ := run(t, `echo =ls`, withSem(equalsSem(Yes))); !strings.HasSuffix(strings.TrimSpace(got), "/ls") {
		t.Errorf("Yes should expand to a path, got %q", got)
	}
	if got, _ := run(t, `echo =ls`, withSem(equalsSem(No))); got != "=ls\n" {
		t.Errorf("No should take it literally, got %q", got)
	}
	// The core refuses, because both answers are plausible output.
	if _, st := run(t, `echo =ls`, withSem(CoreSemantics())); st != 2 {
		t.Errorf("the core should refuse, status %d", st)
	}
}

func TestEqualsExpansionOnlyAtTheHeadAndUnquoted(t *testing.T) {
	sem := equalsSem(Yes)
	for _, tc := range []struct{ src, want string }{
		// An assignment does not begin with `=`.
		{`echo a=b`, "a=b\n"},
		// Quoting removes it, as it does tilde expansion.
		{`echo "=ls"`, "=ls\n"},
		// A lone `=` names nothing after it and is left alone.
		{`echo =`, "=\n"},
	} {
		if got, _ := run(t, tc.src, withSem(sem)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

func TestEqualsExpansionFailureIsFatal(t *testing.T) {
	// The name is reported through the EqualsNotFound wording and the script
	// is abandoned, like any failed expansion here. The status rides the
	// fatal-status axis, so that is named too.
	sem := equalsSem(Yes)
	sem.FatalErrorStatusIsOne = Yes
	out, st := run(t, `echo =nosuchcommand_xyz; echo after`, withSem(sem))
	if strings.Contains(out, "after") {
		t.Errorf("the script continued: %q", out)
	}
	if !strings.Contains(out, "nosuchcommand_xyz not found") {
		t.Errorf("got %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}
