// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// nameInRoster reads Semantics.SpecialBuiltinsBeyondPosix, which is a
// space-separated list of builtin names (#3290).
//
// Written out rather than strings.Fields because it is asked on every command
// with a prefix or a bad option. That is the kind of loop where a `HasPrefix`
// passes every test the dialects have — no builtin we ship is a longer
// spelling of another roster entry, so a prefix match and a whole-name match
// cannot be told apart by any shell case — and then a dialect that later adds
// one silently marks it special. This is the row that can tell them apart.
func TestNameInRosterMatchesWholeNames(t *testing.T) {
	const roster = "alias unalias typeset"
	for _, c := range []struct {
		name string
		want bool
	}{
		{"alias", true},
		{"unalias", true},
		{"typeset", true},
		// Whole names only: a longer name that begins with one of them is
		// not on the roster, and neither is a shorter one it begins with.
		{"aliases", false},
		{"typesetter", false},
		{"typese", false},
		{"ali", false},
		// A name from POSIX's own list, which this function does not answer
		// for — IsSpecialBuiltinHere checks that table first.
		{"export", false},
		{"", false},
	} {
		if got := nameInRoster(roster, c.name); got != c.want {
			t.Errorf("nameInRoster(%q, %q) = %v, want %v", roster, c.name, got, c.want)
		}
	}
	// An empty roster is three of the six presets, and it holds nothing.
	if nameInRoster("", "alias") {
		t.Error("an empty roster held `alias`")
	}
	// One name with no separator at all, which is dash's and BusyBox's shape.
	if !nameInRoster("local", "local") {
		t.Error("a one-name roster did not hold its own name")
	}
	if nameInRoster("local", "locale") {
		t.Error("a one-name roster held a longer name beginning with it")
	}
}
