// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The two candidate-skipping readers decline any pattern carrying a `~(…)`
// group, which is what lets a trim read the `~(K)` class escapes without
// either of them being taught about a class.
//
// Both take `\d` for two bytes of pattern and one literal `d`. A class takes
// a *unit* and names no character, so a trim that skipped candidates on their
// advice while reading the classes would require a `d` at an edge the pattern
// never asked for, and would miss matches **in silence** — which is why this
// is pinned rather than left to the two bail-out sets happening to hold.
//
// `~` is in `bypassOperators` for the edge literals and in `spanStoppers` for
// the span bound, and both readers are handed the whole pattern with the
// group still on it. Narrow either set and this test is what says so.
func TestTheSpanReadersDeclineATildeGroup(t *testing.T) {
	o := patternOpts{classEscapes: true, tildeGlobRead: true, tildeFold: true}
	for _, p := range []string{
		`~(K)\d`,
		`~(K)a\db`,
		`x~(K)\d`,
		`~(K)[0-9]`,
		`a~(i)b`,
	} {
		if _, _, known := patternEdgeLiterals(p, o); known {
			t.Errorf("patternEdgeLiterals(%q): reported edges, want it to decline", p)
		}
		if _, _, bounded := patternSpanBytes(p, o); bounded {
			t.Errorf("patternSpanBytes(%q): reported a bound, want it to decline", p)
		}
	}
	// And the control: a pattern with no group is read by both, so the rows
	// above are the group declining them rather than the readers being dead.
	if _, _, known := patternEdgeLiterals(`abc`, o); !known {
		t.Error("patternEdgeLiterals(`abc`): declined, want edges")
	}
	if _, _, bounded := patternSpanBytes(`abc`, o); !bounded {
		t.Error("patternSpanBytes(`abc`): declined, want a bound")
	}
}
