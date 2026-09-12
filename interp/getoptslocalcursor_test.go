// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `getopts` scan position has two halves and only one of them is a
// parameter: OPTIND counts words, and how far into a clustered word the
// letters have been read lives in the engine. So `local OPTIND` is the one
// declaration whose shadow has to reach past Vars, and what happens when it
// does not is not a wrong letter — it is a scan that cannot finish.
//
// See Semantics.GetoptsLocalOptindRestoresTheCursor.

func localCursorSem(restores Answer) Semantics {
	s := getoptsSem()
	s.GetoptsLocalOptindRestoresTheCursor = restores
	// The neighbours these cases walk past, answered so that an unanswered
	// axis cannot be mistaken for the one under test — and answered the way
	// the shell this reproduces answers them, because two of them decide
	// whether these cases test anything.
	//
	// The cursor is not local to every call: that is the other axis, and a
	// preset saying yes would hand the caller its place back without this
	// one doing anything.
	s.GetoptsPositionIsFunctionLocal = No
	// A valueless declaration does *not* give the name a value, which is what
	// makes `local OPTIND` with no `=1` reach the question at all. Said yes,
	// the declaration becomes an assignment, GetoptsAssignmentRestartsWord
	// drops the position inside the word on the strength of it, and the case
	// about entering the call passes with nothing implemented.
	s.DeclaredNameWithoutValueIsEmpty = No
	s.ValuelessDeclarationOfAHeldNameListsIt = No
	// And the declaration hides whatever the caller's name held, which is
	// the other half of "no value": the local starts empty rather than
	// carrying the outer value in.
	s.ValuelessDeclarationHidesTheOuterValue = Yes
	return s
}

// TestALocalOptindDoesNotCarryTheCallersPositionIntoTheCall — entering the
// call is the core's answer and not an axis: every panel shell with a local
// scope hands the callee a cursor at the start of a word.
//
// The valueless spelling on purpose. With `local OPTIND=1` there is an
// assignment, and GetoptsAssignmentRestartsWord already drops the position
// inside a word on the strength of it — so a `=1` case would pass with
// nothing here implemented at all.
func TestALocalOptindDoesNotCarryTheCallersPositionIntoTheCall(t *testing.T) {
	const src = `g() { local OPTIND; set -- -cd; while getopts "cd" x; do printf "[%s]" "$x"; done; }; ` +
		`set -- -ab; getopts "ab" o; g; echo`
	for _, restores := range []Answer{Yes, No} {
		t.Run(restores.String(), func(t *testing.T) {
			sem := localCursorSem(restores)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != "[c][d]\n" {
				t.Errorf("got %q, want %q — the callee reads its own word from the "+
					"start whatever the axis says about the way back", out, "[c][d]\n")
			}
		})
	}
}

// TestALocalOptindHandsBackThePositionInAWord is the axis itself, and the
// callee runs no `getopts` at all — which is what makes it a discriminator
// rather than a test of the builtin. The only thing the call does is declare
// the name.
func TestALocalOptindHandsBackThePositionInAWord(t *testing.T) {
	const src = `g() { local OPTIND=1; :; }; set -- -ab; getopts ab o; echo "1=$o"; ` +
		`g; getopts ab o; echo "2=[$o] st=$?"`
	for _, tc := range []struct {
		restores Answer
		want     string
	}{
		// The caller left off inside `-ab`, so its next read is `b`.
		{Yes, "1=a\n2=[b] st=0\n"},
		// Only the number came back, so the caller is pointed at the start
		// of a word it had already part-read and takes `a` a second time.
		{No, "1=a\n2=[a] st=0\n"},
	} {
		t.Run(tc.restores.String(), func(t *testing.T) {
			sem := localCursorSem(tc.restores)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestASecondLocalOptindInOneCallDoesNotResaveThePosition: the shadow is
// taken by the declaration that makes the name local and by no later one.
// A second `local OPTIND` re-recording the position would save a cursor the
// call had already moved, and the caller would come back to the middle of the
// callee's scan rather than to its own place.
func TestASecondLocalOptindInOneCallDoesNotResaveThePosition(t *testing.T) {
	const src = `g() { local OPTIND=1; set -- -cd; getopts "cd" x; local OPTIND; }; ` +
		`set -- -ab; getopts ab o; g; getopts ab o; echo "[$o]"`
	sem := localCursorSem(Yes)
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	if out != "[b]\n" {
		t.Errorf("got %q, want %q", out, "[b]\n")
	}
}

// TestAGetoptsLoopAroundACallThatDeclaresItsOwnOptind is the axis as a script
// rather than as a letter, and it is the failure this was found as: a loop
// that parses options and calls something which declares `local OPTIND`
// mid-scan. With the position handed back the loop finishes the word; without
// it the loop starts `-pqr` over and never runs out of options.
//
// The `steps` guard is what lets the second half be a test at all. Under the
// answer that loses the position there is no finite output to assert, so the
// case bounds its own iterations and asserts the shape of the repetition
// instead.
func TestAGetoptsLoopAroundACallThatDeclaresItsOwnOptind(t *testing.T) {
	const src = `f() { local OPTIND=1 o; while getopts "pqr" o; do i=$((i+1)); ` +
		`if [ "$i" -gt 8 ]; then break; fi; printf "[%s]" "$o"; ` +
		`if [ "$o" = q ]; then f -p; fi; done; }; i=0; f -pqr; echo " steps=$i"`
	for _, tc := range []struct {
		restores Answer
		want     string
	}{
		// Four letters and then the options run out, which is what the
		// shells this dialect set claims to be do.
		{Yes, "[p][q][p][r] steps=4\n"},
		// `-pqr` from the top every time the inner call returns: the guard
		// is the only reason this ends.
		{No, "[p][q][p][p][q][p][p][q] steps=10\n"},
	} {
		t.Run(tc.restores.String(), func(t *testing.T) {
			sem := localCursorSem(tc.restores)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
