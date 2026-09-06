// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	. "github.com/blairham/sh/interp"
)

// testSemantics is the vector the substrate's own tests run on: the standard's
// preset, plus the axes these tests actually reach.
//
// It used to be `bash.Semantics()`, and the seed was the problem rather than
// the shell. The substrate defines the questions and a dialect answers them,
// so a test here that borrows a dialect's whole answer sheet has inverted the
// dependency twice over: `interp`'s tests stop compiling if `dialect/bash`
// does not, and — the part that matters — an edit to bash's preset silently
// changes what the substrate asserts. Two files said so out loud while doing
// it: declareprint_test.go opens "these tests name the axes, never a shell",
// and errexit_test.go records that `set -e` has no axis at all (#491).
//
// PosixSemantics is the base because it is the one complete-ish vector in the
// module that is not a shell — the standard has no successors, which is the
// same reason every dialect preset starts there. It leaves an axis unanswered
// wherever the standard decides nothing, and a Runner *refuses* an unanswered
// axis rather than guessing, so a suite that runs whole scripts needs the gaps
// filled before it can run at all.
//
// The thirty-two below are the ones these tests reach, and each is here
// because removing it turns a test red — the set was reduced field by field
// against the suite until nothing more could come out. That is what makes this
// a statement about the substrate's tests rather than a copy of a shell: 183
// axes separate bash's preset from the standard's, and the substrate's tests
// depend on 32.
//
// Where an answer had to be chosen, it is the one the assertions were written
// against, which came from bash. A test that is *about* an axis still sets it
// itself and asserts both sides — see withSem and TestSemanticsAxesHaveTwoSides
// — so the choice here is a floor, not a claim about any shell. What it buys is
// that the floor stops moving on its own.
func testSemantics() Semantics {
	s := PosixSemantics()

	// Arrays, subscripts and the expansions that read them. The standard has
	// no arrays, so it answers none of this and every array test is refused
	// without it.
	s.ArraysAreSparse = Yes
	s.ArrayScalarIsTheWholeArray = No
	s.ArrayLiteralSubscriptIsAKey = No
	s.NegativeSubscriptPastTheStartInserts = No
	s.IndirectionYieldsName = No
	s.ArithNameValueRecurses = Yes
	s.BraceExpansion = Yes
	s.SetFTurnsOffGlobbing = Yes
	s.RegexQuotingMakesLiteral = Yes

	// Declarations: what `typeset`/`export -p` write, and what a name with
	// no value means. declareprint_test.go is the suite that reaches these.
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	s.DeclaredNameWithoutValueIsEmpty = No
	s.DeclarePrintReportsAMissingName = Yes
	s.TypesetLocalNeedsKeywordFunction = No
	s.ReadonlyReassignmentFatal = No
	s.TrapQuoting = ListingQuoteAlwaysEscaped

	// `read` — the option letters decide which of its axes are even
	// reachable, and the four below are the ones the letters open up.
	s.ReadOptions = "rsa:d:n:N:p:t:u:"
	s.ReadExactCountKeepsPartial = Yes
	s.ReadPartialCountSucceeds = No
	s.ReadTimeoutKeepsWhatArrived = Yes

	// Jobs, pipelines and what a pipeline leaves behind.
	s.PipefailOption = Yes
	s.AssignmentUpdatesPipelineStatus = Yes
	s.CoprocEndsInAnArray = Yes
	s.JobsShowBackgroundCommand = Yes
	s.WaitNWaitsForTheNextJob = Yes

	// `select`, which is a prompt loop and therefore four questions about
	// what happens at end of input.
	s.SelectPromptNeedsTerminal = No
	s.SelectEofIsSuccess = No
	s.SelectEofEndsPromptLine = No
	s.SelectEofPrintsNewline = Yes

	// The rest: one builtin apiece.
	s.ExecTakesOptions = Yes
	s.CommandNotFoundStatusIsNotFound = No
	s.UnknownConditionOptionIsAStatus = No

	return s
}
