// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a `.` with no operand costs, asked of a shell in POSIX mode. The mode
// takes this from the vector rather than writing the standard's answer in,
// because the shells disagree about what their own POSIX mode makes of it —
// see Semantics.DotWithNoOperandIsFatalInPosixMode.
//
// Named for the axis and never for a shell: the measurement lives on the
// field, the dialect packages hold the answers, and this asserts only that the
// knob reads them and puts the vector's own back.

func TestPosixModeTakesTheDotNoOperandAxisFromTheVector(t *testing.T) {
	for _, tc := range []struct {
		name              string
		own, inMode, want Answer
	}{
		// The bash row: not fatal under its own name, fatal in the mode.
		{"a vector whose mode makes it fatal", No, Yes, Yes},
		// The zsh and BusyBox ash row, and the one that keeps the mode from
		// being a knob with the standard's answer written into it: not fatal,
		// and the mode does not make it so.
		{"a vector whose mode leaves it alone", No, No, No},
		// The ksh93 row: fatal under every name, mode included.
		{"a vector already fatal, whose mode agrees", Yes, Yes, Yes},
		// And the shape that proves the value is read rather than asserted:
		// a mode that takes a fatality away.
		{"a vector already fatal, whose mode says otherwise", Yes, No, No},
		// Unanswered in the mode means the question has no answer in the
		// mode, not that the mode leaves the axis where it was: a dialect
		// nobody measured must refuse rather than be handed one.
		{"a vector whose mode was never measured", Yes, Unspecified, Unspecified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := permissive()
			s.DotWithNoOperandIsFatal = tc.own
			s.DotWithNoOperandIsFatalInPosixMode = tc.inMode
			r := newTestRunner(t, &Runner{Semantics: &s})

			r.SetPosixMode(true)
			if got := r.Semantics.DotWithNoOperandIsFatal; got != tc.want {
				t.Errorf("axis in posix mode = %v, want %v", got, tc.want)
			}
			r.SetPosixMode(false)
			if got := r.Semantics.DotWithNoOperandIsFatal; got != tc.own {
				t.Errorf("axis after leaving = %v, want the vector's own %v", got, tc.own)
			}
		})
	}
}

// It is its own axis and not the misuse axis beside it: a vector whose mode
// makes a builtin's syntax error fatal does not thereby make a missing operand
// fatal, and the other way round. Two columns in the panel separate them, so a
// shell that moved both together would be wrong about one of them in each.
func TestTheDotAxisAndTheSpecialBuiltinOptionAxisMoveApart(t *testing.T) {
	s := permissive()
	s.DotWithNoOperandIsFatal = No
	s.DotWithNoOperandIsFatalInPosixMode = Yes
	s.BadOptionToSpecialBuiltinFatal = No
	s.BadOptionToSpecialBuiltinFatalInPosixMode = No
	r := newTestRunner(t, &Runner{Semantics: &s})

	r.SetPosixMode(true)
	if got := r.Semantics.DotWithNoOperandIsFatal; got != Yes {
		t.Errorf("dot axis in posix mode = %v, want %v", got, Yes)
	}
	if got := r.Semantics.BadOptionToSpecialBuiltinFatal; got != No {
		t.Errorf("bad-option axis in posix mode = %v, want the vector's %v", got, No)
	}
}

// What the swap is for, run rather than inspected.
//
// The control is the row below it: an ordinary failure runs on under the same
// mode and the same vector, so what ends the script is this builtin's failure
// and not the mode being fatal about everything.
func TestABareDotIsFatalInPosixModeOnlyWhereTheVectorSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		src    string
		inMode Answer
		want   string
		status int
	}{
		{"the mode makes it fatal", ".\necho after\n", Yes, "", 2},
		{"the mode leaves it alone", ".\necho after\n", No, "after\n", 0},
		{"an ordinary failure under the same mode", "false\necho after\n", Yes, "after\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				s := permissive()
				s.DotWithNoOperandIsAnError = Yes
				s.DotWithNoOperandIsFatal = No
				s.DotWithNoOperandIsFatalInPosixMode = tc.inMode
				s.FatalErrorStatusIsOne = Yes
				d := Diagnostics{}
				r.Semantics, r.Diagnostics = &s, &d
				r.SetPosixMode(true)
			})
			// The complaint is the dialect's wording and is not what this
			// test is about; what follows it is. An ordinary failure writes
			// none, so the cut is only made where there was one.
			after := out
			if strings.HasPrefix(tc.src, ".") {
				_, rest, ok := strings.Cut(out, "\n")
				if !ok {
					t.Fatalf("output %q, want a complaint first", out)
				}
				after = rest
			}
			if after != tc.want {
				t.Errorf("after the complaint the shell wrote %q, want %q (whole output %q)", after, tc.want, out)
			}
			if st != tc.status {
				t.Errorf("status %d, want %d", st, tc.status)
			}
		})
	}
}
