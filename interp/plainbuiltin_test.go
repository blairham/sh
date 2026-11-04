// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainBuiltinScript runs the five builtins the route takes through what
// simpleCommand answers for them, each beside a shape the route declines: the
// status and `$_` they leave, a refusal and its wording, `set -e` and the ERR
// and DEBUG traps, a DEBUG action that turns tracing on or defines a function
// of the command's name before the command runs, `set -x`, a function of the
// name, an element of a pipeline, a split or unset parameter, `set -u`, a
// positional parameter, a loop inside a function, a substitution, and a
// negated test. A refusal that ends a shell is made inside a subshell, so the
// script carries on.
const plainBuiltinScript = `p() { printf '<%s>' "$@"; echo; }
i=0; while [ $i -lt 3 ]; do i=$((i + 1)); done; p "$i"
[ a = a ]; p "$?" "$_"
[ a = b ]; p "$?" "$_"
test 3 -gt 2; p "$?" "$_"
test; p "$?"
[ ]; p "$?"
: a b c; p "$?" "$_"
true; p "$?" "$_"
false; p "$?" "$_"
[ 1 -lt ]; p "$?"
[ x -lt 1 ]; p "$?"
[ 1 -eq 1; p "$?"
test 1 2 3 4 5; p "$?"
set -- 5 7; [ $1 -lt $2 ]; p "$?" "$_"
n=4; [ $n -eq 4 ] && p and; [ $n -eq 5 ] || p or
! [ $n -eq 4 ]; p "$?"
x=$([ 1 -eq 1 ] && echo yes); p "$x"
(set -e; [ 1 -eq 2 ]; echo no); p "$?"
(set -e; false; echo no); p "$?"
trap 'echo err' ERR; [ a = b ]; false; trap - ERR
trap 'echo dbg' DEBUG; :; [ 1 -eq 1 ]; trap - DEBUG
trap 'set -x' DEBUG; [ 2 -eq 2 ]; trap - DEBUG; set +x
set -x; [ 3 -eq 3 ]; : traced; set +x
test() { echo "fn $*"; }; test 1 = 1; unset -f test; test 1 = 1; p "$?"
trap 'true() { echo shadowed; }' DEBUG; true; trap - DEBUG; unset -f true; true; p "$?"
[ 1 -eq 1 ] | cat; p "$?"
: | [ 1 -eq 1 ]; p "$?"
[ 1 -eq 1 ] | [ 2 -eq 3 ]; p "$?"
IFS=2; v=121; [ $v = 121 ]; p "$?"; IFS=' '
[ $nosuch ]; p "$?"
(set -u; [ $nosuch ]; echo no) 2>&1 | cat; p done
w='a b'; [ $w = 'a b' ] 2>/dev/null; p "$?"
f() { i=0; while [ $i -lt 4 ]; do i=$((i + 1)); done; p "$i"; }; f
until false; do p until; break; done
k=0; while true; do k=$((k + 1)); [ $k -ge 2 ] && break; done; p "$k"
[ --help ] >/dev/null 2>&1; p help
a=2; [ a -lt 3 ]; p "$?"
(readonly ro=1; v='ro=5'; [ v -eq 5 ]; p "$?" "$ro")
[ lone = lone ]
p "$_"
:
p "$_"
false
p "$?" "$_"
(readonly ro=1; ro=2 true; p "$?") 2>&1
trap 'echo dbg' DEBUG; [ $nosuch ]; trap - DEBUG
f() { [ 1 -eq 1 ]; cat "$1"; }; f <(echo ps)
eval ': y' x; p "$_"
eval '[ 4 -eq 4 ]; cat' <(echo held)
[ 1 -lt ] 2>/dev/null; p quiet
(exec; echo kept >/dev/null; echo after-exec)
v='x=5'; x=1 eval '[ v -eq 5 ]'; p "$x"
p end
`

// plainBuiltinModifierScript is the shapes the route meets in a dialect with
// modifiers: one the grammar takes in front of the command, a name the
// dialect reads as a modifier, and a builtin switched off.
const plainBuiltinModifierScript = `p() { printf '<%s>' "$@"; echo; }
nocorrect true; p "$?" "$_"
nocorrect; p "$?"
nocorrect [ 1 -eq 1 ]; p "$?" "$_"
false; true; p "$?" "$_"
true false; p "$?" "$_"
false; p "$?" "$_"
test 1 -eq 1; p "$?" "$_"
p end
`

// plainBuiltinRows are lines the script writes under every vector, from its
// first command to its last, which the run with the route off must reach for
// the comparison to have compared the whole script.
var plainBuiltinRows = []string{
	"<3>\n",
	"sh: [: 1: unary operator expected\n<2>\n",
	"sh: [: missing ]\n<2>\nsh: test: too many arguments\n<2>\n",
	"<and>\n<or>\n<1>\n<yes>\n<1>\n<1>\nerr\nerr\n",
	"+ [ 2 -eq 2 ]\n",
	"+ [ 3 -eq 3 ]\n+ : traced\n+ set +x\nfn 1 = 1\n<0>\nshadowed\n<0>\n<0>\n",
	"sh: nosuch: parameter not set\n<done>\n",
	"<4>\n<until>\n<2>\n<help>\n",
	"<false>\nsh: ro: readonly variable\ndbg\ndbg\nps\n",
	"held\n<quiet>\nafter-exec\n",
	"<end>\n",
}

// plainBuiltinVectors are the readings the script is run under, each with the
// rows only it writes: the substrate's own with `$_` and the two traps; one
// whose test operands are arithmetic, so a comparison reads a name and an
// assignment inside one is refused; one where `$_` moves only between the
// lines the shell reads; and one whose DEBUG trap fires after the command
// rather than before it.
func plainBuiltinVectors() map[string]plainBuiltinVector {
	with := func(set func(*Semantics)) func(*Runner) {
		return func(r *Runner) {
			sem := *r.Semantics
			sem.UnderscoreTracksTheLastArgument = Yes
			sem.UnderscoreIsAParameterAtAll = Yes
			sem.TrapHasDebugCondition, sem.TrapHasErrCondition = Yes, Yes
			sem.DebugTrapRunsBeforeTheCommand, sem.ErrTrapRunsInSubshells = Yes, Yes
			sem.DebugTrapRunsInsideCalls, sem.DebugTrapRefiresOnEnteringAFunction = Yes, No
			sem.PrefixToAFrozenNameIsCheckedFirst = FrozenPrefixCheckedWithTheCommand
			sem.UnderscoreHoldsTheCallAcrossEvalAndSource = Yes
			if set != nil {
				set(&sem)
			}
			r.Semantics = &sem
		}
	}
	underscores := "<0><]>\n<1><]>\n<0><2>\n<1>\n<1>\n<0><c>\n<0><true>\n<1><false>\n"
	lone := "<]>\n<:>\n<1><false>\n"
	return map[string]plainBuiltinVector{
		"standard": {
			setup: with(nil),
			rows: []string{
				underscores, "sh: [: x: integer expected\n<2>\n", "dbg\ndbg\ndbg\n",
				"sh: [: a: integer expected\n<2>\nsh: [: v: integer expected\n<2><1>\n", lone,
				"after-exec\nsh: [: v: integer expected\n<1>\n<end>\n",
			},
		},
		"arithmetic operands": {
			setup: with(func(s *Semantics) {
				s.TestBuiltinComparisonOperandsAreArithmetic = Yes
				s.ReadonlyRefusalInABuiltinIsFatal = No
			}),
			rows: []string{underscores, "<help>\n<0>\nsh: ro: readonly variable\n", "<1><1>\n", lone, "after-exec\n<5>\n<end>\n"},
		},
		"input-level underscore": {
			setup: with(func(s *Semantics) { s.UnderscoreMovesOnlyBetweenInputCommands = Yes }),
			rows:  []string{"<0><>\n<1><>\n<0><>\n", lone},
		},
		"debug after the command": {
			setup: with(func(s *Semantics) { s.DebugTrapRunsBeforeTheCommand = No }),
			rows:  []string{underscores, lone, "ps\n<x>\nheld\n"},
		},
		"an unanswered operand axis": {
			setup: with(func(s *Semantics) { s.TestBuiltinComparisonOperandsAreArithmetic = Unspecified }),
			rows: []string{
				lone, "after-exec\nsh: `[ n -eq 5 ]` reading its operands as arithmetic: " +
					"the shells disagree here and no dialect was chosen\n<1>\n<end>\n",
			},
		},
		"saved streams": {
			setup: with(func(s *Semantics) { s.OwnDescriptors = SavedStreamsAndPipeReadEndsTakeNumbers }),
			rows:  []string{underscores, lone},
		},
		"modifiers": {
			script: plainBuiltinModifierScript,
			enable: func(d *syntax.Dialect) { d.ReservedPrecommands = map[string]bool{"nocorrect": true} },
			setup: func(r *Runner) {
				with(nil)(r)
				r.SetPrecommand("true", PrecommandTransparent)
				r.SetBuiltinEnabled("false", false)
			},
			rows: []string{"<0><>\n<0>\n<0><]>\n", "<0><1>\n<end>\n"},
			min:  2,
		},
	}
}

// plainBuiltinVector is one reading plainBuiltinScript, or a script of its
// own, is run under: the grammar and semantics it is run with, the rows only
// it writes, and how many commands the route must take in it at least.
type plainBuiltinVector struct {
	script string
	enable func(*syntax.Dialect)
	setup  func(*Runner)
	rows   []string
	min    int
}

// TestAPlainBuiltinAnswersAsSimpleCommandDoes runs plainBuiltinScript under
// each of plainBuiltinVectors with plainBuiltinCommand on and off, and
// requires byte-identical output and status, the route taken with it on and
// never with it off (#6496).
func TestAPlainBuiltinAnswersAsSimpleCommandDoes(t *testing.T) {
	for name, v := range plainBuiltinVectors() {
		t.Run(name, func(t *testing.T) {
			script, rows, least := v.script, v.rows, v.min
			if script == "" {
				script, rows, least = plainBuiltinScript, append(slices.Clone(plainBuiltinRows), v.rows...), 30
			}
			run := func(on bool) (string, int, int) {
				was := SetPlainBuiltinShortcutForTest(on)
				defer SetPlainBuiltinShortcutForTest(was)
				var r *Runner
				out, st := runGrammar(t, script, func(d *syntax.Dialect) {
					d.ProcessSubstitution = true
					if v.enable != nil {
						v.enable(d)
					}
				}, func(rr *Runner) {
					v.setup(rr)
					r = rr
				})
				return out, st, PlainBuiltinsForTest(r)
			}
			full, fullStatus, fullTaken := run(false)
			short, shortStatus, taken := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			if fullTaken != 0 {
				t.Errorf("the route ran %d commands with it off; want none", fullTaken)
			}
			if taken < least {
				t.Errorf("the route ran %d commands; want at least %d", taken, least)
			}
			for _, row := range rows {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

// theRoadToAPlainBuiltin is every method and field simpleCommand reaches on
// the runner from its first statement to the end of its builtin route —
// outside a function call's body, which a builtin never enters — as
// plainBuiltinCommand was checked against it. The word loops are on the road
// here, unlike theRoadToNoWords, because a plain builtin has words.
//
// A name added here is one plainBuiltinCommand does not reach: when this test
// fails, decide whether the new call can do anything for a command of plain
// words naming one of plainBuiltinName's five, with no prefix and no
// redirection; make it in plainBuiltinCommand if so, or decline the command
// in plainBuiltinArgv where the answer depends on the runner's state; and
// then update the list. A name gone from here is one simpleCommand no longer
// reaches, which plainBuiltinCommand may still be doing.
var theRoadToAPlainBuiltin = []string{
	"IsSpecialBuiltinHere", "aPrefixSuppliesThePath", "applyArrayOperandsPastAFailedOpen",
	"applyDeferredFreeze", "applyRedirs", "applyTheBuiltinPrefix", "armProgramGlob",
	"arrayOperandName", "arrayOperands", "ask", "assignFailed", "assignOperands", "assignmentGaveUp",
	"assignmentWordsOf", "beginGlobUnit", "beginPrefixWalk", "builtinDeclarationOperand",
	"builtinNameWordAhead", "callBuiltin", "commandRanAWord", "commandRunsInThisShell",
	"commandWordWasWritten", "compoundOperandUnset", "compoundOperands",
	"compoundOperandsHoldingNothing", "containerByThisLinesLetter", "containerLetterOverALiteral",
	"ctl", "dashPrecommand", "declarationArgvLen", "declarationCommand", "declarationNameSplit",
	"declarationOperands", "declarationWithAssignmentWords", "declinesRmStar", "diag", "diagf",
	"disarmProgramGlob", "dropHeldCommandTrace", "endsTheShellOverABadTarget",
	"equalsOpensATildeContextFor", "equalsTildeContextWord", "execOptionWordsAhead", "exitWaived",
	"expandADeclarationsAssignArg", "expandArrayOperands", "expandAssignArg", "expandEquals",
	"expandErr", "expandPrefixTraceValues", "expandThePrefixUpToTheFrozenName", "expandWordEscaped",
	"failedExpansion", "failedExpansionStatus", "failedExpansionStatusIs", "fatal",
	"fatalPosixSpecialUsageQuiet", "fatalStatusSettled", "fdVarStores", "fdVarStoresThroughThePrefix",
	"flushHeldCommandTrace", "freezing", "funcs", "givesUpForAFailedPrefix",
	"globFieldsUnlessSuppressed", "globalLetterHere", "globalOperandsRunOnTheShellsOwnCell",
	"heldExportRouteOf", "heldListing", "heldTrace", "hideLetterWritten", "holdBuiltinStdin",
	"holdCommandHashAcrossAPrefixedPath", "holdsItsLineForACompoundOperand", "inBuiltin",
	"indexedLetterHere", "jobSpecCommandWord", "keepRedirs", "keywordAssign", "keywordAssignments",
	"keywordPromotable", "lang", "lastArg", "lastArgSet", "leadingModifierWords",
	"lettersRefusedTheOperand", "lexedSubscriptOperands", "listingHeldForItsOperands",
	"literalOperands", "literalOperandsOverANonArray", "literalsPlacedOutliveTheUtility",
	"loadsAnUndefinedFunctionFirst", "locatedUnderACall", "lookupBuiltin", "markExecOpened",
	"namesNoBuiltin", "namespaceFuncLookup", "noteANegatedCall", "noteExecReplacedStdin",
	"noteInputLevelArgument", "nullCommand", "openTrace", "openWordList", "openedStdin",
	"operandLocatedUnderTheCall", "operandWasRejoined", "operandsKeepTheirAssignments",
	"operandsOverANonArray", "outsideTheEqualsContextWord", "pathnameStage", "pathnameStageStopped",
	"plainLiteralFields", "plainLiteralWord", "plainParamWord", "plainScratch", "precommand",
	"precommands", "prefixBeforeADeclarationsOperand", "prefixCheckedFirst", "prefixCommandOf",
	"prefixEntriesStay", "prefixExpandedBeforeTheRedirections", "prefixFailureSpeaker",
	"prefixHeldNames", "prefixHeldUndo", "prefixKeptNames", "prefixLocalDeclaration",
	"prefixShadowed", "prefixSpeaker", "prefixTraceStoppedForNoParent", "prefixWalkFailed",
	"presentedButSwitchedOff", "privateDeclarationRan", "raiseHeldRefusal", "reachOperandInTurn",
	"readonlyScopesItsOperands", "redirErr", "redirFailureStatusOfItsOwn", "redirFds",
	"redirForCommandWord", "redirectForBuiltin", "refuseArrayOperandsEarly",
	"refuseExecWithoutACommand", "refusePrefixes", "refusePrefixesEarly", "refuseSubscriptedPrefixes",
	"refusedLineKeptItsLiterals", "rejoinedArrayOperand", "rejoinsArrayOperand",
	"releaseHeldBodyTraces", "reportExitValue", "reservedWordOutranksAFunction", "restoreVarsExcept",
	"runJobSpecWord", "scopeProcSubs", "sem", "shFileExpansion", "sourceClosedSubscriptOperands",
	"specialBuiltinOutranksAFunction", "startSimpleCommand", "status", "storeOperandInTurn",
	"storeTheAssignmentsAlone", "storeThePrefixAheadOfTheRedirections", "tableLetterHere",
	"takeProgramGlobRefusal", "traceCommand", "traceDestination", "traceEmptyCommand",
	"tracePrefixAndCommand", "tracePrefixAsItGoes", "tracePrefixFollowsTheCommand", "traceTo",
	"tracesEachPrefixEntryOnItsOwnLine", "tracesItsPrefix", "tracesPrefixAsItGoes", "tracing",
	"underscoreAcrossABuiltinsOwnCommands", "underscoreBeforeADeclarationsAssignment", "unspecified",
	"walkThePrefixBeforeTheRedirections", "walkThePrefixBehindTheCommand", "writeHeldPrefixTrace",
	// The kept declaration operand walk (#6373), handed back as simpleCommand
	// ends: a plain builtin is no declaration utility, so no walk was started,
	// the turn is nil and releaseTurn keeps nothing.
	"spares",
}

// TestThePlainBuiltinRoadIsTheOneItWasCheckedAgainst fails when what
// simpleCommand reaches on the way to a builtin changes. See
// theRoadToAPlainBuiltin.
func TestThePlainBuiltinRoadIsTheOneItWasCheckedAgainst(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "runner.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body []ast.Stmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Name.Name == "simpleCommand" {
			body = fn.Body.List
		}
	}
	// The road ends with the builtin route, the statement whose body calls
	// callBuiltin, and passes a function call's statement by with its
	// condition alone: the body is a call, which a builtin is not.
	end, calls := -1, map[string]bool{}
	for i, s := range body {
		if is, ok := s.(*ast.IfStmt); ok && end < 0 && strings.Contains(nodeText(is.Body), "callBuiltin ") {
			end = i
		}
	}
	if end < 0 {
		t.Fatal("found no builtin route in simpleCommand: the guard is looking in the wrong place")
	}
	selectors := func(n ast.Node) {
		if n == nil {
			return
		}
		ast.Inspect(n, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
					calls[sel.Sel.Name] = true
				}
			}
			return true
		})
	}
	functionCalls := 0
	for _, s := range body[:end+1] {
		if is, ok := s.(*ast.IfStmt); ok && strings.Contains(nodeText(is.Body), "callFunc ") {
			functionCalls++
			selectors(is.Init)
			selectors(is.Cond)
			continue
		}
		selectors(s)
	}
	if functionCalls != 1 {
		t.Fatalf("found %d function-call routes ahead of the builtin route in simpleCommand, want one", functionCalls)
	}
	var got []string
	for name := range calls {
		got = append(got, name)
	}
	slices.Sort(got)
	want := slices.Clone(theRoadToAPlainBuiltin)
	slices.Sort(want)
	if added, removed := difference(got, want), difference(want, got); len(added) > 0 || len(removed) > 0 {
		t.Errorf("simpleCommand's road to a builtin now reaches [%s] and no longer [%s]: read "+
			"plainBuiltinCommand against the change, then update theRoadToAPlainBuiltin",
			strings.Join(added, ", "), strings.Join(removed, ", "))
	}
}
