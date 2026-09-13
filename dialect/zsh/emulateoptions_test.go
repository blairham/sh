// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"
	"testing"
)

// TestTheEmulationPartitionCoversTheTable — the three measured lists are a
// partition of the option table: every name is in exactly one of them and no
// list names anything the table does not have.
//
// This is the guard the paired option-letter tables taught: a set written down
// as one list and consulted as another drifts silently, because a name left out
// of every list reads as `emulationKeeps` — which is a real answer for nine
// names and a wrong one for the rest. Both directions are checked, since a name
// added to a list and never to the table is the same fault seen from the other
// end.
func TestTheEmulationPartitionCoversTheTable(t *testing.T) {
	seen := map[string]int{}
	for _, list := range []string{emulationAlwaysReset, emulationStrictReset, emulationNeverReset} {
		for _, name := range strings.Fields(list) {
			seen[name]++
		}
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%s is in %d of the three lists, want exactly one", name, n)
		}
		if _, _, ok := exactOptionName(name); !ok {
			t.Errorf("%s is in a list and not in the option table", name)
		}
	}
	for _, o := range zshOptions {
		if seen[o.base] == 0 {
			t.Errorf("%s is in the option table and in none of the three lists", o.base)
		}
	}
	// And the three sizes, which is what says a name has not quietly moved
	// from one list to another. Measured on zsh 5.9.2, 2026-09-12.
	for _, tc := range []struct {
		list string
		name string
		want int
	}{
		{emulationAlwaysReset, "a bare emulation resets", 81},
		{emulationStrictReset, "only `emulate -R` resets", 95},
		{emulationNeverReset, "no emulation resets", 9},
	} {
		if got := len(strings.Fields(tc.list)); got != tc.want {
			t.Errorf("%d names %s, want %d", got, tc.name, tc.want)
		}
	}
}

// TestTheStrictFormIsTheWiderSet — `-R` resets everything a bare emulation
// resets and more. Read off the classification rather than off the lists, so
// that a reordering of the data cannot make it vacuous.
func TestTheStrictFormIsTheWiderSet(t *testing.T) {
	wider := 0
	for _, o := range zshOptions {
		bare, strict := resetByEmulation(o.base, false), resetByEmulation(o.base, true)
		if bare && !strict {
			t.Errorf("%s: a bare emulation resets it and -R does not", o.base)
		}
		if strict && !bare {
			wider++
		}
	}
	if want := 95; wider != want {
		t.Errorf("-R reaches %d names a bare emulation does not, want %d", wider, want)
	}
}

// TestTheNineNobodyResetsAreTheShellsOwnState — spelled out rather than
// counted, because the list is short and each name is there for a stated
// reason: eight describe how the shell was started and `exec` cannot be
// measured from inside a shell at all.
func TestTheNineNobodyResetsAreTheShellsOwnState(t *testing.T) {
	for _, name := range []string{
		"exec", "interactive", "login", "monitor", "privileged",
		"restricted", "shinstdin", "singlecommand", "zle",
	} {
		if resetByEmulation(name, false) || resetByEmulation(name, true) {
			t.Errorf("%s is reset by an emulation, and it is measured as one that is not", name)
		}
	}
}
