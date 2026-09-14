// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// POSIX mode moves what a refused `set` option does, and it takes the answer
// from the vector rather than writing the standard's in — see
// Semantics.BadSetOptionNameFatalInPosixMode for the panel and for the column
// that decides it.
//
// The two spellings are asked separately at every point here, because the one
// shell that splits them splits them *inside* the mode: BusyBox ash carries on
// past a refused name and stops on a refused letter, under its own name and
// under `sh` alike. A test that moved both together would pass on a knob that
// could only move both.
//
// Named for the axes and never for a shell, which is what keeps the substrate
// from knowing its successors: the measurement lives on the fields, the
// dialect packages hold the answers, and this asserts only that the knob reads
// them.
func TestPosixModeTakesBothSetRefusalAxesFromTheVector(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		ownName, ownLetter   Answer
		modeName, modeLetter Answer
	}{
		{"a vector whose mode makes both fatal", No, No, Yes, Yes},
		{"a vector whose mode leaves both alone", No, No, No, No},
		{"a vector already fatal, whose mode agrees", Yes, Yes, Yes, Yes},
		// The shape the pair exists for: a shell that is not fatal on either
		// and whose mode does not make it so. A knob writing Yes here would
		// be giving one shell's semantics to another.
		{"a vector already fatal, whose mode says otherwise", Yes, Yes, No, No},
		// And the shape that makes it two fields rather than one. The mode
		// takes the letter and leaves the name, which is what BusyBox ash
		// does under the only door it has and what bash 3.2 does under both
		// of its own.
		{"a vector whose mode takes the letter and not the name", No, No, No, Yes},
		{"a vector whose mode takes the name and not the letter", No, No, Yes, No},
		// Unanswered in the mode means the question has no answer in the
		// mode, not that the mode leaves the axis where it was: a dialect
		// nobody measured must refuse rather than be handed one.
		{"a vector whose mode was never measured", Yes, Yes, Unspecified, Unspecified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := permissive()
			s.BadSetOptionNameFatal, s.BadSetOptionLetterFatal = tc.ownName, tc.ownLetter
			s.BadSetOptionNameFatalInPosixMode = tc.modeName
			s.BadSetOptionLetterFatalInPosixMode = tc.modeLetter
			r := newTestRunner(t, &Runner{Semantics: &s})

			r.SetPosixMode(true)
			if got := r.Semantics.BadSetOptionNameFatal; got != tc.modeName {
				t.Errorf("name axis in posix mode = %v, want %v", got, tc.modeName)
			}
			if got := r.Semantics.BadSetOptionLetterFatal; got != tc.modeLetter {
				t.Errorf("letter axis in posix mode = %v, want %v", got, tc.modeLetter)
			}
			r.SetPosixMode(false)
			if got := r.Semantics.BadSetOptionNameFatal; got != tc.ownName {
				t.Errorf("name axis after leaving = %v, want the vector's own %v", got, tc.ownName)
			}
			if got := r.Semantics.BadSetOptionLetterFatal; got != tc.ownLetter {
				t.Errorf("letter axis after leaving = %v, want the vector's own %v", got, tc.ownLetter)
			}
		})
	}
}

// What the swap is for, run rather than inspected: a refused `set` option ends
// the script in POSIX mode where the vector says the mode makes it fatal, and
// carries on where it does not.
//
// Both spellings, and each with the other axis held at the answer that would
// hide a knob moving them together: the name's case leaves the letter alone in
// the mode and the letter's case leaves the name alone, so a single field
// standing in for both fails one half of every row.
func TestARefusedSetOptionIsFatalInPosixModeOnlyWhereTheVectorSaysSo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		src    string
		inMode Answer
		other  Answer
		want   string
		status int
	}{
		{"the mode makes the name fatal", `set -o zzznosuch; echo after`, Yes, No, "", 2},
		{"the mode leaves the name alone", `set -o zzznosuch; echo after`, No, Yes, "after\n", 0},
		{"the mode makes the letter fatal", `set -Z; echo after`, Yes, No, "", 2},
		{"the mode leaves the letter alone", `set -Z; echo after`, No, Yes, "after\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			letter := strings.Contains(tc.src, "-Z")
			out, st := run(t, tc.src, func(r *Runner) {
				s := permissive()
				s.BadSetOptionNameFatal, s.BadSetOptionLetterFatal = No, No
				s.BadSetOptionNameFatalInPosixMode = tc.inMode
				s.BadSetOptionLetterFatalInPosixMode = tc.other
				if letter {
					s.BadSetOptionNameFatalInPosixMode = tc.other
					s.BadSetOptionLetterFatalInPosixMode = tc.inMode
				}
				d := Diagnostics{SetInvalidOptionNameStatus: 2, SetInvalidOptionLetterStatus: 2}
				r.Semantics, r.Diagnostics = &s, &d
				r.SetPosixMode(true)
			})
			// The complaint is the dialect's wording and is not what this
			// test is about; what follows it is. The letter's refusal writes
			// a usage line under it in some dialects, so the tail is taken
			// from the last newline rather than the first.
			after := out
			if i := strings.LastIndex(strings.TrimSuffix(out, "\n"), "\n"); i >= 0 {
				after = out[i+1:]
			} else if tc.want == "" {
				after = ""
			}
			if tc.want == "" {
				if strings.Contains(out, "after") {
					t.Errorf("the shell reached the line after the refusal: %q", out)
				}
			} else if after != tc.want {
				t.Errorf("after the complaint the shell wrote %q, want %q (whole output %q)", after, tc.want, out)
			}
			if st != tc.status {
				t.Errorf("status %d, want %d", st, tc.status)
			}
		})
	}
}

// The other axes the mode moves are not touched by what this pair carries,
// which is the property that makes them per-axis answers rather than per-shell
// ones: a vector may decline both `set` moves and still take the redirection
// move, and the shell that most needs that is the one with no POSIX mode at
// all.
func TestPosixModeStillMovesTheOtherAxesWhenTheSetRefusalsDecline(t *testing.T) {
	s := permissive()
	s.RedirectErrorOnSpecialBuiltinFatal = No
	s.BadSetOptionNameFatal, s.BadSetOptionLetterFatal = No, Yes
	s.BadSetOptionNameFatalInPosixMode, s.BadSetOptionLetterFatalInPosixMode = No, Yes
	r := newTestRunner(t, &Runner{Semantics: &s})

	r.SetPosixMode(true)
	if got := r.Semantics.RedirectErrorOnSpecialBuiltinFatal; got != Yes {
		t.Errorf("redirection axis in posix mode = %v, want %v", got, Yes)
	}
	if got := r.Semantics.BadSetOptionNameFatal; got != No {
		t.Errorf("name axis in posix mode = %v, want the vector's %v", got, No)
	}
	if got := r.Semantics.BadSetOptionLetterFatal; got != Yes {
		t.Errorf("letter axis in posix mode = %v, want the vector's %v", got, Yes)
	}
}

// An option changed while the mode is on is not part of the mode, so leaving
// it must not put back an answer the script itself asked for. The two `set`
// axes have no spelling a script can write, but the *saved* half has to be
// per-axis all the same: a single remembered answer would put the name's back
// where the letter's belongs for the one shell whose two differ.
func TestLeavingPosixModeRestoresEachSetAxisSeparately(t *testing.T) {
	s := permissive()
	s.BadSetOptionNameFatal, s.BadSetOptionLetterFatal = No, Yes
	s.BadSetOptionNameFatalInPosixMode, s.BadSetOptionLetterFatalInPosixMode = Yes, Yes
	r := newTestRunner(t, &Runner{Semantics: &s})

	r.SetPosixMode(true)
	r.SetPosixMode(false)
	if got := r.Semantics.BadSetOptionNameFatal; got != No {
		t.Errorf("name axis after leaving = %v, want %v", got, No)
	}
	if got := r.Semantics.BadSetOptionLetterFatal; got != Yes {
		t.Errorf("letter axis after leaving = %v, want %v", got, Yes)
	}
}
