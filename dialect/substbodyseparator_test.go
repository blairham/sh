// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"
)

// A `;` standing where a command belongs, inside a substitution's body
// (#3333).
//
// Two of the six presets step over one at the top level, and only one of them
// keeps doing it inside the body of a `$( … )` or a `${ …;}`. The other keeps
// it for the older spelling and takes it away for the newer two, which is the
// same seam the two spellings part company on for where a refusal is located
// and for whether a line continuation at the front is removed.
//
// Measured 2026-09-17, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> s.sh` with
// stdin from `/dev/null`, in a fresh directory, on ksh93u+ 2012-08-01 and zsh
// 5.9.2.
//
// The silence is what made this worth a row rather than a note: before this,
// `v=$(echo hi; ;)` left an empty value and a status of 0 in the ksh preset
// with nothing said, so a script whose body held a stray `;` was told its
// substitution had succeeded.
func TestASeparatorWhereACommandBelongsInsideASubstitutionBody(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		// refused says the body is a syntax error in the preset that
		// narrows; the other preset takes every row.
		refused bool
	}{
		// The two spellings of one construct, and the seam between them.
		{"a bare separator in a `$( … )` body", "v=$(echo a; ;)\n", true},
		{"the same body in backquotes", "v=`echo a; ;`; printf '[%s]' \"$v\"\n", false},
		{"a body that begins with one", "v=$(; echo a)\n", true},
		{"one between two statements", "v=$(echo a; ; echo b)\n", true},
		{"one after a `&`", "v=$(echo a & ; echo b)\n", true},
		{"one after a bar", "v=$(echo a | ; cat)\n", true},
		{"one where a condition begins", "v=$(if ; then echo a; fi)\n", true},
		{"a body that is nothing else", "v=$( ; )\n", true},
		{"the current-shell spelling", "v=${ echo a; ;}\n", true},
		// The two controls. An and-or's missing right-hand side is a
		// position of its own and keeps the separator; a separator
		// *terminating* a statement was never this question.
		{"an and-or's missing operand", "v=$(false || ; echo b); printf '[%s]' \"$v\"\n", false},
		{"a separator that terminates", "v=$(echo a; ); printf '[%s]' \"$v\"\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The preset that narrows inside the body.
			_, errs, st := splitRun(t, presets["ksh"], tc.src)
			if refused := errs != ""; refused != tc.refused {
				t.Errorf("the narrowing preset wrote %q at %d, want refused = %v", errs, st, tc.refused)
			}
			if tc.refused && !strings.Contains(errs, "`;'") {
				t.Errorf("the refusal was %q, want it to name the separator", errs)
			}
			// And the preset that takes a separator anywhere, which this
			// must not move: it is the column the panel says is already
			// right, and it has no `${ …;}` spelling to ask about.
			if tc.src != "v=${ echo a; ;}\n" {
				// What it says about a command it could not find is not the
				// question; that the `;` was taken is.
				if _, errs, _ := splitRun(t, presets["zsh"], tc.src); strings.Contains(errs, "parse error") {
					t.Errorf("the wider preset wrote %q, want it to take every row", errs)
				}
			}
		})
	}
}

// And the whole panel on the row the issue was filed from, which is what says
// the four presets that step over nothing were not reached by this.
func TestTheBodyWithAStraySeparatorAcrossThePanel(t *testing.T) {
	const src = "printf 'start\\n'\nv=$(echo hi; ;)\n"
	for _, c := range []struct {
		preset string
		errs   string
		status int
	}{
		{"bash", "syntax error near unexpected token `;'", 2},
		{"ksh", "syntax error at line 2: `;' unexpected", 3},
		{"dash", "Syntax error: \";\" unexpected", 2},
		{"ash", "syntax error: unexpected \";\"", 2},
		{"posix", "\";\" unexpected", 2},
		// The one column that is right to say nothing.
		{"zsh", "", 0},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, errs, st := splitRun(t, presets[c.preset], src)
			if out != "start\n" {
				t.Errorf("wrote %q, want the line before the substitution to have run", out)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
			if c.errs == "" {
				if errs != "" {
					t.Errorf("wrote %q, want nothing", errs)
				}
				return
			}
			if !strings.Contains(errs, c.errs) {
				t.Errorf("wrote %q, want it to hold %q", errs, c.errs)
			}
		})
	}
}
