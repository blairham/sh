// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"strings"

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
//     lists — or a declaration word plainDeclarationName lists, every operand
//     of which is a plain name written as itself (plainDeclarationOperands),
//     which is `local a b c` (#6373). A modifier the grammar took in front
//     of it, zsh's `nocorrect`, decides nothing for a command with a word
//     behind it: simpleCommand reads it only for the line that has nothing
//     left to run, and for the trace, which the route leaves to
//     simpleCommand;
//   - plainBuiltinArgv, from the runner once the command has started: every
//     word is one simpleCommand's own plain-word shortcuts would answer (see
//     interp/plainword.go), the name is no function's and no modifier's, it
//     resolves to a builtin, and nothing is in a state the route does not
//     carry — tracing, or a process substitution an enclosing command holds.
//
// Where the second half declines, the command goes to simpleCommand with its
// DEBUG firing marked as made, which is what fired=true says: the trap has
// run once, and running it again would be a second firing for one command.
//
// TestAPlainBuiltinAnswersAsSimpleCommandDoes and, for the declaration lines,
// TestAPlainDeclarationLineAnswersAsSimpleCommandDoes run the same scripts
// with the route on and off and require the same output and status;
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
	if len(w.Spans) != 1 || w.Spans[0].Kind != syntax.Literal {
		return false
	}
	if plainDeclarationName(w.Spans[0].Value) {
		return plainDeclarationOperands(c.Args[1:])
	}
	return plainBuiltinName(w.Spans[0].Value)
}

// plainDeclarationOperands reports whether a declaration's operand words are
// written as the route takes them, from the tree: at least one, and each a
// single unquoted literal that is a plain name, or a plain name given the
// value of a bare parameter, `name=$1` (see paramValuedOperand). Asked here
// and not only of the argv, so that a line with any other value on it goes
// straight to simpleCommand rather than starting down the route to be
// declined there.
func plainDeclarationOperands(words []*syntax.Word) bool {
	if len(words) == 0 {
		return false
	}
	for _, w := range words {
		if paramValuedOperand(w) {
			continue
		}
		if len(w.Spans) != 1 {
			return false
		}
		if s := w.Spans[0]; s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted || !isPlainName(s.Value) {
			return false
		}
	}
	return true
}

// paramValuedOperand reports whether a declaration's operand is written
// `name=$param`: a plain name and its `=` as one unquoted literal, then a bare
// unquoted parameter. simpleCommand takes such a word as the assignment
// operand it is, and expandAssignArg reads its value through plainParamValue
// whenever that answers, which is what the route does with it too.
func paramValuedOperand(w *syntax.Word) bool {
	if len(w.Spans) != 2 {
		return false
	}
	s, p := &w.Spans[0], &w.Spans[1]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		p.Kind != syntax.ParamExp || p.Quoting != syntax.Unquoted || !bareParamOnly(p.Param) {
		return false
	}
	name, rest, eq := strings.Cut(s.Value, "=")
	return eq && rest == "" && isPlainName(name)
}

// paramValuedOperandsAt is where the operands paramValuedOperand accepts stand
// in a declaration's argv, which is where they stand among its words: each
// word of a line the route takes is one field. It is the record simpleCommand
// keeps of a declaration's assignment operands — see Runner.declarationOperands
// — and nil for a line with none.
func paramValuedOperandsAt(words []*syntax.Word) []int {
	var at []int
	for i, w := range words {
		if i > 0 && paramValuedOperand(w) {
			at = append(at, i)
		}
	}
	return at
}

// plainDeclarationName is the declaration words the route runs a line of for,
// where every operand is a plain name written as itself: `local a b c`. Such a
// line goes through simpleCommand as the five do. Its words are literals with
// no `=` in them, so none is an assignment operand and the declaration's own
// steps there — the operand walk, the prefix ordering, the operands' array
// literals and the store after the utility — have nothing to do, and the
// utility's own route for a plain line takes it from there (see
// interp/plainline.go). The one record simpleCommand writes for a declaration
// that it does not write for the five is the line's assignment words, and the
// route writes it the same way. See plainDeclarationOperands, which says
// which lines those are.
func plainDeclarationName(name string) bool {
	switch name {
	case "local", "typeset", "declare":
		return true
	}
	return false
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
	// writes its line, and a process substitution an enclosing command holds
	// is set aside for the length of this one by scopeProcSubs and released
	// by releaseHeldBodyTraces. And a command reached with the shell already
	// unwinding is simpleCommand's to give up.
	if r.tracing() || len(r.procSubs) > 0 || r.ctl != controlNone {
		return nil, nil, false
	}
	declaring := plainDeclarationName(c.Args[0].Spans[0].Value)
	// Sized for one field a word, as simpleCommand sizes its own: a plain
	// word is exactly one.
	argv = make([]string, 0, len(c.Args))
	for _, w := range c.Args {
		if lit, plain := r.plainLiteralWord(w); plain {
			argv = append(argv, lit)
			continue
		}
		if declaring && len(argv) > 0 && paramValuedOperand(w) {
			// A declaration's `name=$param`, which simpleCommand takes as an
			// assignment operand only where it is a declaration's at all, and
			// takes through `set -k` or the store in turn where those apply;
			// the route takes neither. Its text is expandAssignArg's own, from
			// the same read, or the word is simpleCommand's.
			if !r.paramValuedOperandsPlain(c, argv) {
				return nil, nil, false
			}
			one := syntax.Word{Spans: w.Spans[1:]}
			v, plain := r.plainParamValue(&one)
			if !plain {
				return nil, nil, false
			}
			argv = append(argv, w.Spans[0].Value+v)
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

// paramValuedOperandsPlain reports whether a declaration's `name=$param`
// operand is, in this runner, the assignment operand that simpleCommand would
// store with nothing in front of it: the line is a declaration utility's in
// the dialect's reading, `set -k` is not reaching for it, and its operands are
// not stored in turn before the utility runs. argv is the line's words so far.
func (r *Runner) paramValuedOperandsPlain(c *syntax.SimpleCmd, argv []string) bool {
	return !r.keywordAssignments && !r.declarationOperandsStoredInTurn() && r.declarationCommand(c, argv)
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
	declaring := plainDeclarationName(argv[0])
	var valuedAt []int
	if declaring {
		r.plainDeclarationLines++
		if valuedAt = paramValuedOperandsAt(c.Args); valuedAt != nil {
			r.plainValuedDeclarationLines++
		}
	}
	// What a plain word's expansion leaves behind it. See plainLiteralFields.
	r.braceName = false
	r.lastWordEdges = wordEdges{}
	// The word list's hold for a refused pattern, which the words never
	// filled: open for the words and closed behind them, so the builtin runs
	// with an empty one and the outer list's comes back at the end. See
	// openWordList and raiseHeldRefusal.
	outerGlob := r.progGlob
	r.progGlob = programGlob{}
	// The per-command records simpleCommand writes once the words are in,
	// each of which comes to nothing for a line with no operands.
	r.dashPrecommand = false
	r.declarationOperands, r.lexedSubscriptOperands, r.sourceClosedSubscriptOperands = nil, nil, nil
	r.arrayOperands, r.operandWasRejoined = nil, false
	// A declaration's `name=$param` operands, recorded as simpleCommand
	// records the assignment operands it expands.
	r.declarationOperands = valuedAt
	// `$_` moves to the last word before the command runs, or, behind a
	// declaration the reserved word runs, to the word in front of its first
	// assignment operand. See simpleCommand.
	r.noteInputLevelArgument(argv)
	r.lastArg, r.lastArgSet = argv[len(argv)-1], true
	if declaring {
		if w, ok := r.underscoreBeforeADeclarationsAssignment(c); ok {
			r.lastArg = w
		}
	}
	// The redirections — none — are applied for what applying them clears
	// and for the pipeline element's entry they settle, and what that holds
	// is put back once the builtin is over, as simpleCommand's deferred
	// closing does: none of the five keeps its redirections the way `exec`
	// does. See applyRedirs. No process substitution is held, so there is
	// no body for releaseHeldBodyTraces to let go.
	r.fdVarStores, r.openedStdin = nil, nil
	closers, err := r.applyRedirs(ctx, nil, false, redirOwnerThisShell)
	if err != nil {
		r.endPlainBuiltin(closers, outerGlob)
		return err
	}
	r.fdVarStores, r.openedStdin = nil, nil
	r.redirectForBuiltin, r.redirForCommandWord = "", ""
	r.exitWaived = false
	st, fatal := r.runPlainBuiltin(ctx, c, argv, fn)
	r.endPlainBuiltin(closers, outerGlob)
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
func (r *Runner) runPlainBuiltin(ctx context.Context, c *syntax.SimpleCmd, argv []string, fn Builtin,
) (st int, fatal bool) {
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
	if plainDeclarationName(argv[0]) {
		// The one declaration record of the line that is not empty for a
		// line of plain names: in the column that stores a declaration's
		// assignment words ahead of the utility, it is the command itself,
		// which a refusal reads for a name written as an assignment. None
		// is, here, but the record is what simpleCommand leaves for the
		// call. See Runner.declarationWithAssignmentWords.
		r.assignmentWordsOf = r.declarationWithAssignmentWords(c, argv)
	}
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
// command is over, in their order: what the redirection list held put back,
// a prefix's trace line held for a shell that stopped, the open trace line
// handed back from a child, the outer word list's hold for a refused pattern,
// and this command's process substitutions — none were open when it started
// — taken away.
func (r *Runner) endPlainBuiltin(closers []io.Closer, outerGlob programGlob) {
	for _, c := range closers {
		_ = c.Close()
	}
	r.writeHeldPrefixTrace()
	if line := r.openTrace; line != nil && line.owner == r {
		line.keptForAChild = false
	}
	r.progGlob = outerGlob
	if len(r.procSubs) > 0 {
		r.removeProcSubs(r.takeProcSubs())
	}
}
