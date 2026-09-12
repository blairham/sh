// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A bare `trap` listing is ordered by signal *number* and not by name, and
// EXIT is in that sequence as signal 0 rather than printed ahead of it.
//
// It listed alphabetically here, which is an order no column in the panel
// produces: ABRT before HUP, and EXIT outside the ordering entirely. The
// listing is what a script parses to save and restore its traps, so the order
// is output rather than presentation.
//
// The signals below are the ones numbered alike everywhere the panel runs —
// 1, 2, 3, 6 and 15. USR1 and USR2 sit either side of TERM depending on the
// kernel, so a test using them would assert a fact about the machine.

const scrambled = "trap 'echo x' TERM\ntrap 'echo x' HUP\ntrap 'echo x' ABRT\n" +
	"trap 'echo x' INT\ntrap 'echo x' EXIT\ntrap 'echo x' QUIT\ntrap\n"

// conditions is the trailing word of each listed line, in the order printed.
func conditions(t *testing.T, order TrapListingSequence) []string {
	t.Helper()
	out, _, _ := trapRun(t, scrambled, func(s *Semantics) {
		s.TrapListingOrder = order
	}, Diagnostics{})
	var got []string
	for _, line := range strings.Split(out, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == "trap" {
			got = append(got, fields[len(fields)-1])
		}
	}
	return got
}

// TestATrapListingIsOrderedBySignalNumber is six of the seven columns.
func TestATrapListingIsOrderedBySignalNumber(t *testing.T) {
	want := []string{"EXIT", "HUP", "INT", "QUIT", "ABRT", "TERM"}
	if got := conditions(t, TrapListingLowestFirst); !equalStrings(got, want) {
		t.Errorf("listed %v, want %v", got, want)
	}
}

// TestATrapListingCanRunTheOtherWay is ksh93's, and it is the same sequence
// reversed rather than a different rule — which is what puts EXIT last with
// nothing said about EXIT.
func TestATrapListingCanRunTheOtherWay(t *testing.T) {
	want := []string{"TERM", "ABRT", "QUIT", "INT", "HUP", "EXIT"}
	if got := conditions(t, TrapListingHighestFirst); !equalStrings(got, want) {
		t.Errorf("listed %v, want %v", got, want)
	}
}

// TestTheListingOrderIsNotAlphabetical, asserted on its own because the two
// orders above are both wrong in the same way if the sort key is the name:
// this is the assertion that fails for the defect rather than for a
// reversal.
func TestTheListingOrderIsNotAlphabetical(t *testing.T) {
	got := conditions(t, TrapListingLowestFirst)
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			return
		}
	}
	t.Errorf("listed %v, which is in name order", got)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
