// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strings"
	"testing"
)

// The two synopsis tables are disjoint, which is the guard the paired
// option-letter tables taught: one shell with two tables of synopses for the
// same builtin is two places for one string to live, and the second copy is
// the one that goes stale. See helpOtherSynopses, where the split is a
// measurement about `--help` and not a filing convention.
func TestTheTwoSynopsisTablesAreDisjoint(t *testing.T) {
	other := helpOtherSynopses()
	for name := range builtinHelp() {
		if _, ok := other[name]; ok {
			t.Errorf("%s has a synopsis in builtinHelp and in helpOtherSynopses", name)
		}
	}
	// And a topic's line never repeats its own name, because the builtin
	// half arrives as `name: synopsis` and this half as the synopsis alone.
	// Getting that wrong writes `cd: cd: cd [-L…]`, which is the kind of
	// mistake a differential test catches and an eye does not.
	for name, synopsis := range other {
		if strings.HasPrefix(synopsis, name+": ") {
			t.Errorf("%s: the synopsis repeats the name: %q", name, synopsis)
		}
	}
	for name, line := range builtinHelp() {
		if !strings.HasPrefix(line, name+": ") {
			t.Errorf("%s: builtinHelp's line does not open with the name: %q", name, line)
		}
	}
}
