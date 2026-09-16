// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// Semantics.ErrExitEntersACommandSubstitution, both sides, and the latch that
// POSIX mode moves it by.
//
// The source is written with the substitution in a **command word** rather
// than in an assignment, which is the whole of why the probe discriminates:
// `x=$(false; echo no)` ends the script under either answer, because the
// assignment reports the body's status and errexit fires on the assignment.
// `echo "end[$(false; echo no)]"` reaches `after` at 0 whichever way the axis
// is set, and only the word differs — so a shell that got this wrong before
// #3001 looked like a shell that had merely stopped early.

const errExitSubstSrc = `set -e; echo "end[$(false; echo no)]"; echo after`

// TestErrExitEntersACommandSubstitutionYesStopsTheBody is dash, ksh93, zsh and
// bash under the name `sh`: the body's own shell holds the option, so it ends
// at `false` and the word comes back empty.
func TestErrExitEntersACommandSubstitutionYesStopsTheBody(t *testing.T) {
	out, st := run(t, errExitSubstSrc, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.Yes
	})
	if out != "end[]\nafter\n" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, "end[]\nafter\n")
	}
}

// TestErrExitEntersACommandSubstitutionNoRunsTheBodyOn is bash and BusyBox
// ash: the body does not hold the option, so `echo no` runs.
func TestErrExitEntersACommandSubstitutionNoRunsTheBodyOn(t *testing.T) {
	out, st := run(t, errExitSubstSrc, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.No
	})
	if out != "end[no]\nafter\n" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, "end[no]\nafter\n")
	}
}

// TestErrExitEntersACommandSubstitutionNoLeavesTheOptionOffInTheBody is the
// half that makes this a shell option and not a failure being let through.
// Measured: `$-` inside the body carries no `e` in bash and in BusyBox ash,
// and `set -o` there reports `errexit off`. A body that kept the letter and
// merely declined to stop would be wrong about itself.
func TestErrExitEntersACommandSubstitutionNoLeavesTheOptionOffInTheBody(t *testing.T) {
	const src = `set -e; echo "[$(case $- in (*e*) echo E;; (*) echo none;; esac)]"`
	for _, tc := range []struct {
		answer interp.Answer
		want   string
	}{
		{interp.No, "[none]\n"},
		{interp.Yes, "[E]\n"},
	} {
		out, st := run(t, src, func(r *interp.Runner) {
			r.Semantics.ErrExitEntersACommandSubstitution = tc.answer
		})
		if out != tc.want || st != 0 {
			t.Errorf("%v: got %q status %d, want %q at 0", tc.answer, out, st, tc.want)
		}
	}
}

// TestErrExitStillEntersASubshell is the bound: the parentheses are not what
// the axis is about. Every column in the panel stops here, bash and BusyBox
// ash included, so the answer must not reach a subshell that is not a
// substitution's body.
func TestErrExitStillEntersASubshell(t *testing.T) {
	out, st := run(t, `set -e; (false; echo no); echo after`, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.No
	})
	if out != "" || st != 1 {
		t.Errorf("got %q status %d, want nothing at 1", out, st)
	}
}

// TestErrExitEntersACommandSubstitutionIsNotAskedWithoutTheOption keeps the
// axis off the common path. Every column agrees about a substitution in a
// shell that never set `-e`, so a Runner with no answer here has to run
// `echo $(…)` rather than refuse it by name.
func TestErrExitEntersACommandSubstitutionIsNotAskedWithoutTheOption(t *testing.T) {
	out, st := run(t, `echo "end[$(false; echo no)]"`, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.Unspecified
	})
	if out != "end[no]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q at 0", out, st, "end[no]\n")
	}
}

// TestErrExitEntersACommandSubstitutionUnansweredIsRefused is the other half
// of the rule above: where the columns *do* disagree, a shell that was never
// told says so by name rather than picking one.
func TestErrExitEntersACommandSubstitutionUnansweredIsRefused(t *testing.T) {
	out, _ := run(t, errExitSubstSrc, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.Unspecified
	})
	if !strings.Contains(out, "command substitution") {
		t.Errorf("got %q, want the unanswered axis named in it", out)
	}
}

// TestPosixModeMovesErrExitIntoACommandSubstitutionOneWay is the latch, and it
// is the half an implementation that models the mode as save-and-restore gets
// wrong. Measured 2026-09-15 on bash 5.3.20: `set -o posix` turns
// `inherit_errexit` on, `set +o posix` leaves it on, and
// `shopt -u inherit_errexit` is the only way back.
func TestPosixModeMovesErrExitIntoACommandSubstitutionOneWay(t *testing.T) {
	out, st := run(t, errExitSubstSrc, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.No
		r.SetPosixMode(true)
		r.SetPosixMode(false)
	})
	if out != "end[]\nafter\n" || st != 0 {
		t.Errorf("got %q status %d, want the mode to have latched it on", out, st)
	}
	if !r0(t) {
		t.Error("the switch does not report what the mode did to it")
	}
}

// r0 asks the exported switch the same question, because it is the getter
// `shopt inherit_errexit` reads and bash answers `on` there after the mode has
// been left.
func r0(t *testing.T) bool {
	t.Helper()
	on := false
	run(t, `:`, func(r *interp.Runner) {
		r.Semantics.ErrExitEntersACommandSubstitution = interp.No
		r.SetPosixMode(true)
		r.SetPosixMode(false)
		on = r.ErrExitEntersACommandSubstitution()
	})
	return on
}

// TestSetErrExitEntersACommandSubstitutionTravelsBothWays is the switch
// itself, which `shopt -s inherit_errexit` and `shopt -u inherit_errexit`
// drive. Unlike the mode, it moves in both directions: measured, `shopt -u
// inherit_errexit` in a bash invoked as `sh` puts the shell back where plain
// bash starts.
func TestSetErrExitEntersACommandSubstitutionTravelsBothWays(t *testing.T) {
	for _, tc := range []struct {
		start interp.Answer
		set   bool
		want  string
	}{
		{interp.No, true, "end[]\nafter\n"},
		{interp.Yes, false, "end[no]\nafter\n"},
	} {
		out, st := run(t, errExitSubstSrc, func(r *interp.Runner) {
			r.Semantics.ErrExitEntersACommandSubstitution = tc.start
			r.SetErrExitEntersACommandSubstitution(tc.set)
			if got := r.ErrExitEntersACommandSubstitution(); got != tc.set {
				t.Errorf("the getter reports %v after being set %v", got, tc.set)
			}
		})
		if out != tc.want || st != 0 {
			t.Errorf("from %v set %v: got %q status %d, want %q", tc.start, tc.set, out, st, tc.want)
		}
	}
}
