// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// POSIX mode swaps several axes on the vector, and all but this one it writes
// the standard's own answer into. This one it takes from the vector, because
// the shells disagree
// about what their own POSIX mode makes of it — see
// Semantics.BadOptionToSpecialBuiltinFatalInPosixMode.
//
// Named for the axis and never for a shell, which is what keeps the substrate
// from knowing its successors: the measurement lives on the field, the dialect
// packages hold the answers, and this asserts only that the knob reads them.
func TestPosixModeTakesTheBadOptionAxisFromTheVector(t *testing.T) {
	for _, tc := range []struct {
		name              string
		own, inMode, want Answer
	}{
		{"a vector whose mode makes it fatal", No, Yes, Yes},
		{"a vector whose mode leaves it alone", No, No, No},
		{"a vector already fatal, whose mode agrees", Yes, Yes, Yes},
		// The shape the whole axis exists for: a shell that is not fatal and
		// whose POSIX mode does not make it so. A knob writing Yes here would
		// be giving one shell's semantics to another.
		{"a vector already fatal, whose mode says otherwise", Yes, No, No},
		// Unanswered in the mode means the question has no answer in the
		// mode, not that the mode leaves the axis where it was: a dialect
		// nobody measured must refuse rather than be handed one.
		{"a vector whose mode was never measured", Yes, Unspecified, Unspecified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := permissive()
			s.BadOptionToSpecialBuiltinFatal = tc.own
			s.BadOptionToSpecialBuiltinFatalInPosixMode = tc.inMode
			r := newTestRunner(t, &Runner{Semantics: &s})

			r.SetPosixMode(true)
			if got := r.Semantics.BadOptionToSpecialBuiltinFatal; got != tc.want {
				t.Errorf("axis in posix mode = %v, want %v", got, tc.want)
			}
			r.SetPosixMode(false)
			if got := r.Semantics.BadOptionToSpecialBuiltinFatal; got != tc.own {
				t.Errorf("axis after leaving = %v, want the vector's own %v", got, tc.own)
			}
		})
	}
}

// The other axes the mode moves are not touched by the value this one carries,
// which is the property that makes it a per-axis answer rather than a per-shell
// one: a vector may decline the bad-option move and still take the redirection
// move, and one shell in the panel does exactly that.
func TestPosixModeStillMovesTheOtherAxesWhenThisOneDeclines(t *testing.T) {
	s := permissive()
	s.RedirectErrorOnSpecialBuiltinFatal = No
	s.BadOptionToSpecialBuiltinFatal = No
	s.BadOptionToSpecialBuiltinFatalInPosixMode = No
	r := newTestRunner(t, &Runner{Semantics: &s})

	r.SetPosixMode(true)
	if got := r.Semantics.RedirectErrorOnSpecialBuiltinFatal; got != Yes {
		t.Errorf("redirection axis in posix mode = %v, want %v", got, Yes)
	}
	if got := r.Semantics.BadOptionToSpecialBuiltinFatal; got != No {
		t.Errorf("bad-option axis in posix mode = %v, want the vector's %v", got, No)
	}
}

// What the swap is for, run rather than inspected: a special builtin's usage
// error ends the script in POSIX mode where the vector says the mode makes it
// fatal, and carries on where it does not.
//
// The status is the *builtin's* and not the shell's generic fatal one, which
// is why FatalErrorStatusIsOne is answered Yes here and the fatal run still
// ends at 2. See setFatalStatus.
func TestAUsageErrorIsFatalInPosixModeOnlyWhereTheVectorSaysSo(t *testing.T) {
	const src = `export -q; echo after`

	for _, tc := range []struct {
		name   string
		inMode Answer
		want   string
		status int
	}{
		{"the mode makes it fatal", Yes, "", 2},
		{"the mode leaves it alone", No, "after\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, src, func(r *Runner) {
				s := permissive()
				s.BadOptionToSpecialBuiltinFatal = No
				s.BadOptionToSpecialBuiltinFatalInPosixMode = tc.inMode
				s.FatalErrorStatusIsOne = Yes
				d := Diagnostics{}
				r.Semantics, r.Diagnostics = &s, &d
				r.SetPosixMode(true)
			})
			// The complaint is the dialect's wording and is not what this
			// test is about; what follows it is.
			_, after, ok := strings.Cut(out, "\n")
			if !ok || after != tc.want {
				t.Errorf("after the complaint the shell wrote %q, want %q (whole output %q)", after, tc.want, out)
			}
			if st != tc.status {
				t.Errorf("status %d, want %d", st, tc.status)
			}
		})
	}
}
