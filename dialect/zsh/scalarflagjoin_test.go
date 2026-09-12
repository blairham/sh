// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Where the join a scalar context makes sits, in the rows that need this
// preset's own answers to be visible at all (#1705).
//
// The position is below the operator and above everything else the group
// does. `interp` pins the half that a grammar alone can show; these three
// shapes need a bare array name to be the whole array, and a value's
// backslash to survive an unquoted expansion, which are answers rather than
// grammar.
//
// Measured on zsh 5.9.2, 2026-09-12.
func TestAScalarContextJoinsBelowTheOperator(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The discriminating pair: the same characters in the same
		// assignment, parting on where the join sits. Unquoted the trim
		// empties both elements and the separator is all that is left of
		// them; quoted, rule 5 joins first and the trim takes one `ab` off
		// the front of the pair it made.
		{"the trim runs on each element", `y=(ab ab); x=${(j:+:)y#ab}; print -r -- "[$x]"`, "[+]\n"},
		{"where quoting joins ahead of it", `y=(ab ab); x="${(j:+:)y#ab}"; print -r -- "[$x]"`, "[+ab]\n"},
		{"and the control with no trim at all", `y=(ab ab); x=${(j:+:)y}; print -r -- "[$x]"`, "[ab+ab]\n"},

		// From below: the quoting flag takes the joined word, so the spaces
		// the join made are the ones it escapes. A step that ran on the five
		// fields would have found no space in any of them.
		{
			"the quoting flag takes the joined word",
			"f() { printf \"b  b\\na a\\nc\\n\"; }\nx=${(q)$(f)}\nprint -r -- \"[$x]\"\n",
			"[b\\ b\\ a\\ a\\ c]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
