// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// A plain builtin: `[ $i -lt 2000 ]`, `:`, `true`, `false` — a command that
// is nothing but plain words naming one of five builtins that run nothing of
// the script's own. In a counting loop it is one command in three, and
// simpleCommand answered it the way it answers every command: a frame of
// several kilobytes set up for the word loop, a declaration question per
// word, the precommand scan, the pathname stage, the prefix walks, the
// redirection bookkeeping and eight deferred calls, for a line where every
// one of those comes to nothing (#6496).
//
// plainBuiltinCommand is simpleCommand for such a line, with each statement
// simpleCommand reaches that makes a difference here and none that does not.
// Two halves decide whether it may answer at all:
//
//   - plainBuiltinShape, from the tree alone: no assignment, no redirection,
//     and a first word that is a literal naming a builtin plainBuiltinName
//     lists. A modifier the grammar took in front of it, zsh's `nocorrect`,
//     decides nothing for a command with a word behind it: simpleCommand
//     reads it only for the line that has nothing left to run, and for the
//     trace, which the route leaves to simpleCommand;
//   - plainBuiltinArgv, from the runner once the command has started: every
//     word is one simpleCommand's own plain-word shortcuts would answer (see
//     interp/plainword.go), the name is no function's and no modifier's, it
//     resolves to a builtin, and nothing is in a state the route does not
//     carry — tracing, an element's pipe still being entered, a process
//     substitution an enclosing command holds.
//
// Where the second half declines, the command goes to simpleCommand with its
// DEBUG firing marked as made, which is what fired=true says: the trap has
// run once, and running it again would be a second firing for one command.
//
// TestAPlainBuiltinAnswersAsSimpleCommandDoes runs the same scripts with the
// route on and off and requires the same output and status;
// TestThePlainBuiltinRoadIsTheOneItWasCheckedAgainst fails when what
// simpleCommand reaches on the way to a builtin changes, which is the moment
// to read this file again.

// plainBuiltinShortcut switches plainBuiltinCommand on. A variable so a test
// can run the same script with simpleCommand answering every command.
var plainBuiltinShortcut = true

// plainBuiltinName is the five builtins the route runs. Each takes its
// operands as strings and nothing else, runs no command of the script's own,
// writes nothing to standard output, makes no declaration and is no
// precommand modifier: `[` and `test` read their operands and set a status,
// `:` and `true` set 0 and `false` 1. The list is writesNoOutput's, and for a
// related reason — a builtin that never writes is one whose call needs no
// hold, and one that never runs anything is one whose call needs none of the
// state a command line sets up for the commands it runs.
func plainBuiltinName(name string) bool {
	switch name {
	case "[", "test", ":", "true", "false":
		return true
	}
	return false
}

// plainBuiltinShape reports whether c is written as a command the route may
// take, from the tree alone: a cheap first look, so that a command which is
// not one never starts down the route. Whether its words really are plain is
// plainBuiltinArgv's question, asked of each word with plainLiteralWord, so
// the first word is only looked at here for the name it spells.
func plainBuiltinShape(c *syntax.SimpleCmd) bool {
	if !plainBuiltinShortcut || len(c.Args) == 0 || len(c.Assigns) != 0 || len(c.Redirs) != 0 {
		return false
	}
	w := c.Args[0]
	return len(w.Spans) == 1 && w.Spans[0].Kind == syntax.Literal && plainBuiltinName(w.Spans[0].Value)
}

// plainBuiltinArgv is the command's argv and the builtin it names, where the
// route may run it; ok is false where simpleCommand has to.
//
// It reads and writes nothing: each word is answered by plainLiteralWord or
// plainParamWord, which read a literal's text or an ordinary scalar's value
// and nothing else, so a command declined here reaches simpleCommand having
// moved no state at all.
func (r *Runner) plainBuiltinArgv(c *syntax.SimpleCmd) (argv []string, fn Builtin, ok bool) {
	// The state a line of plain words passes through in simpleCommand
	// without touching, but only in these conditions: a traced command
	// writes its line, an element whose pipe is still being entered settles
	// it in applyRedirs, and a process substitution an enclosing command
	// holds is set aside for the length of this one by scopeProcSubs and
	// released by releaseHeldBodyTraces. And a command reached with the
	// shell already unwinding is simpleCommand's to give up.
	if r.tracing() || r.entering.pending || len(r.procSubs) > 0 || r.ctl != controlNone {
		return nil, nil, false
	}
	// Sized for one field a word, as simpleCommand sizes its own: a plain
	// word is exactly one.
	argv = make([]string, 0, len(c.Args))
	for _, w := range c.Args {
		if lit, plain := r.plainLiteralWord(w); plain {
			argv = append(argv, lit)
			continue
		}
		if v, plain := r.plainParamWord(w); plain {
			argv = append(argv, v)
			continue
		}
		return nil, nil, false
	}
	name := argv[0]
	// A function of the name is found ahead of the builtin, and a modifier
	// is read by the precommand scan before the builtin is looked up; both
	// are simpleCommand's to answer, whatever they then decide.
	if _, isFunc := r.funcs[r.namespaceFuncLookup(name)]; isFunc {
		return nil, nil, false
	}
	if _, modifier := r.precommand(name); modifier {
		return nil, nil, false
	}
	if fn, ok = r.lookupBuiltin(name); !ok {
		return nil, nil, false
	}
	return argv, fn, true
}

// plainBuiltinCommand is simpleCommand for a command plainBuiltinShape
// accepted. See the file comment. The statements below are the ones
// simpleCommand reaches for such a command, in its order; each says what it
// stands for there.
func (r *Runner) plainBuiltinCommand(ctx context.Context, c *syntax.SimpleCmd, fired bool) error {
	if !r.startSimpleCommand(ctx, fired) {
		return nil
	}
	argv, fn, ok := r.plainBuiltinArgv(c)
	if !ok {
		return r.simpleCommand(ctx, c, true)
	}
	r.plainBuiltins++
	// What a plain word's expansion leaves behind it. See plainLiteralFields.
	r.braceName = false
	r.lastWordEdges = wordEdges{}
	// The word list's hold for a refused pattern, which the words never
	// filled: open for the words and closed behind them, so the builtin runs
	// with an empty one and the outer list's comes back at the end. See
	// openWordList and raiseHeldRefusal.
	outerGlob := r.progGlob
	r.progGlob = programGlob{}
	defer func() { r.progGlob = outerGlob }()
	// The per-command records simpleCommand writes once the words are in,
	// each of which comes to nothing for a line with no operands.
	r.dashPrecommand = false
	r.declarationOperands, r.lexedSubscriptOperands, r.sourceClosedSubscriptOperands = nil, nil, nil
	r.arrayOperands, r.operandWasRejoined = nil, false
	// `$_` moves to the last word before the command runs. See simpleCommand.
	r.noteInputLevelArgument(argv)
	r.lastArg, r.lastArgSet = argv[len(argv)-1], true
	// The redirections — none — are applied for what applying them clears.
	// See applyRedirs. No process substitution is held, so there is no body
	// for releaseHeldBodyTraces to let go.
	r.fdVarStores, r.openedStdin = nil, nil
	if closers, err := r.applyRedirs(ctx, nil, false, redirOwnerThisShell); err != nil || closers != nil {
		// Not reachable with no element entry pending, which
		// plainBuiltinArgv made sure of, and said so rather than dropped: a
		// closer here would be one this route never closes.
		panic("interp: a plain builtin's empty redirection list opened something")
	}
	r.fdVarStores, r.openedStdin = nil, nil
	r.redirectForBuiltin, r.redirForCommandWord = "", ""
	r.exitWaived = false
	st, fatal := r.runPlainBuiltin(ctx, argv, fn)
	r.endPlainBuiltin()
	switch {
	case fatal:
		r.fatalStatusSettled = false
	case r.unspecified:
		r.status = 2
	case r.fatalStatusSettled:
		r.fatalStatusSettled = false
	default:
		r.status = st
	}
	return nil
}

// runPlainBuiltin is simpleCommand's builtin bracket for a line with no
// prefix and no operand: the state it sets for the builtin's call and puts
// back, the call, and what it decides from what the call left. fatal is the
// case simpleCommand returns on without writing the status.
func (r *Runner) runPlainBuiltin(ctx context.Context, argv []string, fn Builtin) (st int, fatal bool) {
	// The prefix's names, held and kept — none — for the call. See
	// Runner.prefixHeldNames.
	outerHeld, outerKept, outerShadowed := r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed
	outerUndo := r.prefixHeldUndo
	r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed, r.prefixHeldUndo = nil, nil, nil, nil
	// The builtin on the record, and every declaration record a builtin's
	// call carries, cleared for this one: the five read none of them, and a
	// command they could reach would find what simpleCommand leaves there.
	outer := r.inBuiltin
	r.inBuiltin = argv[0]
	outerFreezing := r.freezing
	outerLiterals := r.literalOperands
	r.literalOperands = nil
	outerAssignments := r.assignmentWordsOf
	r.assignmentWordsOf = nil
	outerLetterContainers := r.containerByThisLinesLetter
	r.containerByThisLinesLetter = nil
	outerOverANonArray := r.operandsOverANonArray
	r.operandsOverANonArray = nil
	outerArgvLen := r.declarationArgvLen
	r.declarationArgvLen = len(argv)
	outerCompounds := r.compoundOperands
	r.compoundOperands = nil
	outerCompoundUnset := r.compoundOperandUnset
	r.compoundOperandUnset = nil
	outerIndexed, outerTable, outerGlobal := r.indexedLetterHere, r.tableLetterHere, r.globalLetterHere
	r.indexedLetterHere, r.tableLetterHere, r.globalLetterHere = nil, nil, nil
	outerHeldListing := r.heldListing
	r.heldListing = heldListing{}
	r.freezing = nil
	outerWritten := r.commandWordWasWritten
	r.commandWordWasWritten = true
	outerPrivateRan := r.privateDeclarationRan
	r.privateDeclarationRan = false
	outerHideRan := r.hideLetterWritten
	r.hideLetterWritten = false
	outerRanAWord := r.commandRanAWord
	r.commandRanAWord = false
	outerKeptLiterals := r.refusedLineKeptItsLiterals
	r.refusedLineKeptItsLiterals = false
	st = r.callBuiltin(ctx, argv[0], fn, argv[1:])
	keptLiterals := r.refusedLineKeptItsLiterals
	r.refusedLineKeptItsLiterals = outerKeptLiterals
	if !r.commandRanAWord {
		r.reportExitValue(st)
	}
	r.commandRanAWord = outerRanAWord
	r.privateDeclarationRan = outerPrivateRan
	r.hideLetterWritten = outerHideRan
	r.commandWordWasWritten = outerWritten
	r.inBuiltin = outer
	// A line with no operand has none to store once the builtin is back, so
	// what simpleCommand decides there comes down to the records that could
	// still fail it. See the builtin route in simpleCommand.
	if st == 0 || keptLiterals {
		switch {
		case r.ctl == controlExit:
			fatal = true
		case keptLiterals:
		case r.assignFailed, r.lettersRefusedTheOperand:
			st = 1
		}
	}
	r.applyDeferredFreeze()
	if code, listed := r.listingHeldForItsOperands(); listed && st == 0 && !fatal {
		st = code
	}
	r.freezing = outerFreezing
	r.literalOperands = outerLiterals
	r.assignmentWordsOf = outerAssignments
	r.containerByThisLinesLetter = outerLetterContainers
	r.declarationArgvLen = outerArgvLen
	r.compoundOperands = outerCompounds
	r.compoundOperandUnset = outerCompoundUnset
	r.indexedLetterHere, r.tableLetterHere, r.globalLetterHere = outerIndexed, outerTable, outerGlobal
	r.heldListing = outerHeldListing
	r.operandsOverANonArray = outerOverANonArray
	kept, shadowed := r.prefixKeptNames, r.prefixShadowed
	r.prefixHeldNames, r.prefixKeptNames, r.prefixShadowed = outerHeld, outerKept, outerShadowed
	r.prefixHeldUndo = outerUndo
	r.restoreVarsExcept(nil, kept, shadowed)
	return st, fatal
}

// endPlainBuiltin is what simpleCommand's deferred calls do once such a
// command is over: a prefix's trace line held for a shell that stopped, the
// open trace line handed back from a child, and this command's process
// substitutions — none were open when it started — taken away.
func (r *Runner) endPlainBuiltin() {
	r.writeHeldPrefixTrace()
	if line := r.openTrace; line != nil && line.owner == r {
		line.keptForAChild = false
	}
	if len(r.procSubs) > 0 {
		r.removeProcSubs(r.takeProcSubs())
	}
}
