// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether an assignment written in front of a special builtin is still set on
// the next line is one of the axes POSIX mode moves, and it is one of the ones
// the mode writes the standard's own answer into rather than reading a twin
// field for.
//
// Measured 2026-09-13 over `FOO=1 : ; echo "[$FOO]"`: bash 5.3, bash 3.2 and
// zsh 5.9 print `[]` by default and `[1]` under their own POSIX mode and under
// the name `sh`; dash, ksh93u+ and BusyBox ash print `[1]` under every name and
// have no such mode at all. The second half is what licenses the standard's
// answer here — the core's mode is entered by every dialect invoked as `sh`, so
// a written-in `Yes` is only safe where the dialects with no mode of their own
// already say `Yes`.
//
// Named for the axis and never for a shell: the measurement lives on the field
// and the dialect packages hold the defaults.
func TestPosixModeMakesAPrefixOnASpecialBuiltinPersist(t *testing.T) {
	for _, tc := range []struct {
		name string
		own  Answer
	}{
		// The two shells with a mode: transient by default, and the mode
		// moves them.
		{"a vector that takes the prefix back", No},
		// The three without one: already the standard's answer, so the mode
		// has nothing to move and leaving it must not write the opposite.
		{"a vector that already keeps it", Yes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := permissive()
			s.AssignmentPrefixPersistsOnSpecialBuiltin = tc.own
			r := newTestRunner(t, &Runner{Semantics: &s})

			r.SetPosixMode(true)
			if got := r.Semantics.AssignmentPrefixPersistsOnSpecialBuiltin; got != Yes {
				t.Errorf("axis in posix mode = %v, want %v", got, Yes)
			}
			r.SetPosixMode(false)
			if got := r.Semantics.AssignmentPrefixPersistsOnSpecialBuiltin; got != tc.own {
				t.Errorf("axis after leaving = %v, want the vector's own %v", got, tc.own)
			}
		})
	}
}

// What the swap is for, run rather than inspected — and with the control that
// keeps it the special-builtin rule and not the prefix rule as a whole. A shell
// that made every prefix persist in POSIX mode would pass the first half and
// fail the second, and so would one that reached the site in execBuiltin
// instead of the field.
func TestAPrefixPersistsInPosixModeAndOnlyOnASpecialBuiltin(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"a special builtin", `x=1; x=2 export y=3; echo "[$x]"`, "[2]\n"},
		{"an ordinary one", `x=1; x=2 true; echo "[$x]"`, "[1]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, func(r *Runner) {
				s := permissive()
				s.AssignmentPrefixPersistsOnSpecialBuiltin = No
				r.Semantics = &s
				r.SetPosixMode(true)
			})
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
		})
	}
}

// And the round trip through the script's own words, which is the half a knob
// written at startup could not do: leaving the mode reaches the dialect's own
// answer, so the name is a starting position a script can leave rather than a
// reading fixed for the run.
func TestLeavingPosixModeMakesAPrefixTransientAgain(t *testing.T) {
	out, _ := run(t, "x=1; x=2 export y=3; echo \"[$x]\"\nset +o posix\na=1; a=2 export b=3; echo \"[$a]\"\n",
		func(r *Runner) {
			s := permissive()
			s.AssignmentPrefixPersistsOnSpecialBuiltin = No
			r.Semantics = &s
			r.AddSetOptions("posix")
			r.SetPosixMode(true)
		})
	if want := "[2]\n[1]\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}
