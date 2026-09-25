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
	// A stream redirected twice, which is bash's answer here: the last target
	// alone. The suite that is *about* it sets both — see redirect_test.go
	// and multiosdup_test.go — and the tests that merely write `cat <<A <<B`
	// on the way to something else need an answer rather than a refusal.
	s.RedirectsUseEveryTarget = No

	// The mark in front of a value written in an output base, spelled as
	// `base#` — the majority reading, and the one every test not about the
	// C spellings needs answered rather than refused. See
	// Semantics.IntegerBaseMarkIsCSpelled.
	s.IntegerBaseMarkIsCSpelled = No

	// The reach a negative subscript makes past an array's first element, at
	// the silent reading — no complaint, and no end to count back from where
	// the name holds nothing. That is what every suite here that counts off
	// the end of an array on the way to something else was written against,
	// and the one that is *about* the reach answers all three itself; see
	// interp/subscriptbeforestart_test.go (#3406).
	s.SubscriptBeforeTheFirstElementRead = SubscriptBeforeStartIsNothing
	s.SubscriptBeforeTheFirstElementNeedsAnElement = No

	// What an assignment prefix does to the export attribute of the name it
	// stands in front of at a *builtin*, and whether a declaration keeps the
	// value its own prefix set. The leave-alone reading and No: four of the
	// six columns and five of the six, and the pair that changes nothing
	// about a suite writing `x=1 true` on the way to something else. The
	// suite that is about them sets them itself (#3437).
	s.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
	// A whole-table listing sees the prefix as an ordinary entry, which is
	// what this engine did before the axis existed — so a test that is not
	// about it reads the listing it always read. See interp/prefixlisting.go.
	s.PrefixInAWholeTableListing = PrefixInAWholeTableListingIsAnOrdinaryEntry
	s.DeclarationPromotesThePrefixEntry = No

	// What a traced assignment shows of its own elements and of its
	// subscript. The words as written, which is the reading a suite tracing
	// an array on its way to something else was written against; the suite
	// that is *about* them answers both sides (#1959).
	s.TraceArrayLiteralShowsTheExpandedElements = No
	s.TraceElementSubscriptIsEvaluated = No

	// What stops a name resolved through its own value: a frame count, which
	// is three of the four columns that recurse at all. A suite that writes
	// `a=b; b=a` on its way to something else needs an answer rather than a
	// refusal; the suite that is *about* the bound sets both values and
	// asserts on each — see interp/arithrecursionbound_test.go (#3416).
	s.ArithRecursionBound = ArithRecursionBoundedByDepth

	// The hexadecimal escape's two readings. `$'a\x1bb'` has three digits
	// after the `\x`, so a snippet writing a control character that way
	// reaches the first of them on the way to something else — the
	// `${x@Q}` rows do. The suites that are *about* them set both sides.
	s.DollarSingleHexReadsEveryDigit = No
	s.DollarSingleDigitlessEscapeIsAZeroByte = No

	// And the three that used to be a shared table. `\e` is how a snippet
	// writes a control sequence and `\u` how it writes a code point, both
	// on the way to something else; the suite that is *about* them is
	// dialect/ash's, where BusyBox answers No to all three (#3270).
	s.DollarSingleEscEscape = Yes
	s.DollarSingleQuestionEscape = Yes
	s.DollarSingleUnicodeEscapes = Yes

	// The span replacement's anchors, and an empty pattern behind one. Every
	// suite that writes `${v/#a/X}` on its way to something else reaches the
	// first, and it is an axis since #3272 — BusyBox ash has the replacement
	// and no anchors. The suite that is *about* them is
	// emptyreplacementpattern_test.go, which sets both sides.
	s.ReplacementAnchors = Yes
	s.AnchoredEmptyReplacementPattern = Yes
	// And whether the anchor counts after the *global* `//`, which zsh alone
	// reads. bash's answer is the floor here as everywhere else in this
	// function; the suite that is *about* it is
	// globalreplacementanchor_test.go, which sets both sides (#3307).
	s.GlobalReplacementAnchors = No

	s.ArraysAreSparse = Yes
	s.ArrayScalarIsTheWholeArray = No
	// And which element the one-element answer means on a keyed table: the
	// one keyed `0`, which is bash's and ksh93's answer and the floor here.
	// The suite that is *about* it sets both — see assoc_test.go.
	s.KeyedTableScalarIsTheFirstValue = No
	s.ArrayLiteralSubscriptIsAKey = No
	// And a keyed literal's bare elements are each one value, which is bash's
	// answer and the floor here as everywhere else in this function. The
	// suite that is *about* it sets both sides — see tableliteralvalue_test.go.
	s.BareElementsInATableLiteralAreEachOneValue = Yes
	s.NegativeSubscriptPastTheStartInserts = No
	// `a+=x` over a name holding an array, and the two array letters given to
	// a name holding a scalar. bash's answers, which is the floor these
	// suites assert against; the tests that are *about* them set them
	// themselves and assert every side — see arrayscalarappend_test.go and
	// declaredcompound_test.go.
	s.ScalarAppendedToAnArrayBecomesANewElement = No
	// And what a plain `a=x` does to a name already holding one: bash
	// writes the first element and keeps the array. The suite that is
	// *about* it sets both answers — see scalarovercompound_test.go.
	s.ScalarAssignedOverACompoundReplacesTheName = No
	s.ScalarUnderAnArrayDeclaration = ScalarUnderACompoundBecomesTheFirstElement
	s.ScalarUnderATableDeclaration = ScalarUnderACompoundBecomesTheFirstElement
	// The two questions a *subscripted* declaration and an exported compound
	// raise, at bash's answers, which is the floor these suites assert
	// against. The tests that are *about* them set both sides themselves —
	// see exportedcompound_test.go (#1380).
	s.ExportedCompoundReachesAChildAsItsFirstValue = No
	s.SubscriptedOperandCarriesTheAttributes = Yes
	s.IndirectionYieldsName = No
	s.ArithNameValueRecurses = Yes
	// And what an unset name found that way is: a zero, which is what three
	// of the four do. The suite that is *about* it sets both — see
	// arithrecurse_test.go.
	s.ArithRecursedNameMustBeSet = No
	s.BraceExpansion = Yes
	s.SetFTurnsOffGlobbing = Yes
	// What a value's backslash does to the character behind it once the
	// field is a pattern. The standard's preset globs an expansion result and
	// says nothing about this, so a suite that never answered it would refuse
	// every `$v*` over a value with a backslash in it. The floor is the
	// reading four of the six columns have; the suite that is *about* it sets
	// all three -- see valuebackslash_test.go.
	s.ValueBackslashInAPattern = ValueBackslashQuotesWhatFollows
	s.RegexQuotingMakesLiteral = Yes

	// Declarations: what `typeset`/`export -p` write, and what a name with
	// no value means. declareprint_test.go is the suite that reaches these.
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	s.DeclaredNameWithoutValueIsEmpty = No
	// The declaration questions whose answer the suite needs but is not
	// about. Each is a dialect's divergence and the majority of the panel
	// answers it the quiet way, which is what a test not asking the question
	// should meet: the suites that *are* about them set both sides
	// themselves — see declarationlisting_test.go, exportglobal_test.go,
	// inconsistenttype_test.go, typeletterfamily_test.go and
	// readonlycompound_test.go.
	s.ExportLetterDeclaresAGlobal = No
	s.ValuelessDeclarationOfAHeldNameListsIt = No
	s.ScalarOverACompoundIsAnInconsistentType = No
	s.ReadonlyRecordsTheCompoundAttribute = No
	s.TypeLetterAndAnArrayLiteralIsAnInconsistentType = No
	s.NumericAttributeReplacesTheCaseAttribute = No
	s.CaseAttributeReplacesTheNumericAttribute = No
	s.NumericAttributeReplacesTheArrayAttribute = No
	s.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = No
	s.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = No
	s.DeclarePrintReportsAMissingName = Yes
	s.TypesetLocalNeedsKeywordFunction = No
	// And who else can see a declaration that *is* local, at the answer five
	// of the six panel columns give: whatever the function calls sees it. A
	// suite about a nested local should get the dynamic reading rather than a
	// refusal, and the suite that is about the axis sets both sides — see
	// staticscope_test.go (#2865).
	s.CallerLocalsReachTheCallee = Yes
	s.ReadonlyReassignmentFatal = No
	// A here-document body fed to a program is expanded in that program's
	// process, so what it writes does not come back. bash's answer, and
	// ksh93's and zsh's; dash alone says otherwise.
	s.HeredocExpandsInTheCommandsProcess = Yes
	// And the same for a redirection's target. bash's answer again, with
	// dash alone on the other side; the suite that is *about* it sets both
	// — see redirtarget_test.go.
	s.RedirectTargetExpandsInTheCommandsProcess = Yes
	s.TrapQuoting = ListingQuoteAlwaysEscaped

	// `read` — the option letters decide which of its axes are even
	// reachable, and the four below are the ones the letters open up.
	s.ReadOptions = "rsa:d:n:N:p:t:u:"
	s.ReadExactCountKeepsPartial = Yes
	s.ReadPartialCountSucceeds = No
	s.ReadTimeoutKeepsWhatArrived = Yes
	// And what the deadline is a deadline *for*: the whole read, which is
	// what two of the three answer. The suite that is *about* it sets both —
	// see readtimeoutscope_test.go.
	s.ReadTimeoutBoundsReadability = No

	// Jobs, pipelines and what a pipeline leaves behind.
	s.PipefailOption = Yes
	s.AssignmentUpdatesPipelineStatus = Yes
	s.TestAndArithmeticUpdatePipelineStatus = Yes
	s.CoprocEndsInAnArray = Yes
	s.JobsShowBackgroundCommand = Yes
	s.WaitNextJob = WaitNextJobFirstToFinish

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
