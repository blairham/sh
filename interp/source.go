// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/blairham/sh/syntax"
)

// This file implements `eval` and `.`, the two special builtins that run shell
// source in *this* shell rather than a child.
//
// They were both in the specialBuiltins table long before either existed, so
// the table asserted something false: `eval echo hi` was `command not found`
// while IsSpecialBuiltin("eval") said yes. That is the gap this closes.
//
// The pair shares one engine because the pair is one idea — text, parsed, run
// on the current runner, so an assignment inside it survives. They differ in
// three measured ways, which is what `sourced` names.

// Registered here rather than in the builtins literal because these two are
// the only builtins that run arbitrary shell: each reaches the dispatcher,
// which reads the map they would be declared in, and Go rejects that as an
// initialization cycle. Assigning after the literal is built is the whole fix
// and costs nothing at the call site.
//
// `source` is deliberately absent. dash does not have it — `command -v source`
// says so — which makes that name a dialect's answer, added in its Apply.
func init() {
	builtins["eval"] = biEval
	builtins["."] = biDot
}

// sourced is how a run of borrowed text differs from the script around it.
type sourced struct {
	// label names the text in a diagnostic: "eval", or the path of the file.
	label string

	// named, when set, is what a diagnostic calls this text whatever the
	// dialect's own word for eval's text or for a sourced file would be. It
	// is the name of the *variable* the text came out of: measured, bash
	// reports a syntax error in `PROMPT_COMMAND` as `PROMPT_COMMAND: line N:`
	// — neither the builtin's name nor the `(eval)` one dialect writes — and
	// that name is the person's only way back to the text. See
	// Runner.EvalVariable, its one caller.
	named string

	// eval marks the text as `eval`'s rather than a file's, because one
	// dialect calls it something of its own — `(eval)` — where the others
	// use the builtin's name.
	eval bool

	// syntaxStatus is what a parse failure reports when it is not fatal.
	// It is passed rather than read from the dialect because the two callers
	// disagree: measured, a parse failure inside `eval` carries the dialect's
	// ordinary syntax-error status, but inside a *sourced file* zsh reports
	// 126 where its own syntax-error status is 1.
	syntaxStatus int

	// fatalStatus is what the builtin reports when the text was given up
	// over an *error* — see Semantics.FatalErrorEndsBorrowedTextOnly. Passed
	// rather than read from the dialect because the two callers disagree
	// again, the same way they do about a parse failure: measured, a file
	// `.` gave up over an error reports 126 in zsh where the same failure
	// inside `eval` reports 1, and ksh93 says 1 for both. Zero means the
	// status the error itself carried.
	fatalStatus int

	// read, when set, is handed each group of physical lines as the reader
	// consumes it, before those lines run. Only `fc`'s editor road fills it
	// in; see fcHistory.edit and #4030.
	read func(borrowedLines)

	// catchReturn stops at `return` instead of letting it unwind the function
	// around it.
	//
	// True for `.` and false for `eval`, and measured both ways: `return`
	// inside a sourced file ends the source and becomes its status, while
	// `f() { eval return 3; echo no; }` returns from *f* — the `echo` does not
	// run in any shell in the panel. So eval is transparent to control flow
	// and `.` is a boundary for exactly one kind of it.
	catchReturn bool
}

// borrowedLines is one group of physical lines as the reader consumed them.
//
// It exists because the two things a shell does with the input it is reading
// — echo it under the verbose option, and put it in the history list — are
// both about the *lines* and not about the commands they parsed into, and a
// reader that hands back a tree has thrown the lines away. Measured 2026-09-21
// on bash 5.3.20, an `fc` editor leaving `for i in a b` / `do` / `echo $i` /
// `done`: all four lines are echoed before any of it runs, because all four
// had to be read to find the end of one command. So the echo is per line as
// read, and the statement's reconstructed text would be one line where bash
// writes four.
type borrowedLines struct {
	// text is the physical lines, each with the newline that ended it.
	text string

	// body is the index among those lines of the one the command begins on,
	// or -1 where the group held no command at all — the blank and comment
	// lines a reader steps over on its way to one. Measured, and the reason
	// this is not simply zero: an editor leaving `# c` / `echo A` makes bash
	// record *two* entries, where the same two lines leading a `for` are one.
	body int

	// whole says these are the whole text, handed over because the dialect
	// reads borrowed text through before running any of it rather than a
	// line at a time. What a list should then hold is not measurable from a
	// script — the two shells that read text whole record nothing from `fc`
	// in one, and ksh93 refuses the editor road there before an editor is
	// chosen — so the caller keeps what it did before rather than inventing
	// an answer. See fcHistory.edit.
	whole bool

	// dialect is the grammar these lines were read under, for a reader that
	// has to ask where a line boundary falls inside them.
	dialect syntax.Dialect
}

// sourceName is what a diagnostic calls this text.
func (s sourced) sourceName(d Diagnostics) string {
	switch {
	case s.named != "":
		// A variable's name beats both, because neither of the others is
		// true of it: the text is not a file and the person did not type
		// `eval`.
		return s.named
	case s.eval && d.EvalSourceName != "":
		return d.EvalSourceName
	case !s.eval && d.SourceFileIsTheBuiltin:
		// The builtin that read the file rather than the file itself.
		return "."
	}
	return s.label
}

// naming is where this kind of borrowed text puts its name.
func (s sourced) naming(d Diagnostics) SourceNaming {
	if s.eval {
		return d.EvalNaming
	}
	return d.SourceFileNaming
}

// route is how this text reached the parser, for the grammar answer that
// depends on it.
//
// The fourth measured difference between the pair, and the one that is about
// the *text* rather than about the run around it. Measured 2026-09-07,
// ksh93u+ 2012-08-01, from a script file so that neither answer can be the
// invocation's:
//
//	eval "echo 'abc"   prints abc, status 0
//	. ./q.sh           `q.sh: syntax error at line 1: `'' unmatched`
//
// where q.sh holds that same line. So a string handed over is a command
// string wherever it was handed over from, and a file is a file — which is
// exactly the split syntax.Dialect.CloseQuotesAtEOF records for the
// invocation, reached by a second pair of doors.
func (s sourced) route() syntax.ProgramRoutes {
	if s.eval {
		return syntax.RouteFromCommandString
	}
	return syntax.RouteFromScriptFile
}

// runsWhatItParsed is whether this kind of borrowed text runs the commands it
// has already read when a later line will not parse.
//
// Two fields because the panel does not group them, which is the same shape
// this file already has for naming and for the grammar route. Measured
// 2026-09-11, counting a side effect rather than reading a transcript —
// `eval "printf x >> f\nif; then"`, and a sourced file holding the same two
// lines:
//
//	bash 5.3, bash 3.2, dash	both run the first line
//	zsh                     	the file does, `eval` does not
//	ksh93                   	neither does
//
// It matters beyond the transcript. Everything before the offending line
// really happened in the columns that answer Yes, so a `.` of a file that sets
// six names and has a typo on the last line leaves six names set there and
// none where the text is read through first.
func (s sourced) runsWhatItParsed(sem *Semantics) Answer {
	if s.eval {
		return sem.EvalRunsWhatItParsed
	}
	return sem.SourcedFileRunsWhatItParsed
}

// readerAxis names the question for a shell that has not answered it.
func (s sourced) readerAxis() string {
	if s.eval {
		return "whether `eval` runs the commands it has read before a later line fails to parse"
	}
	return "whether a sourced file runs the commands it has read before a later line fails to parse"
}

// hasALaterLine reports whether src has a newline with anything but blanks
// after it — which is what makes the reader's answer decidable.
//
// A line changes how a *later* line parses and never how its own does, so
// one-line text reads the same whichever way it is read. The trailing
// newline every file ends with is not a later line, and treating it as one
// made `. inc.sh` ask the question of a one-line file.
func hasALaterLine(src string) bool {
	return strings.ContainsRune(strings.TrimRight(src, " \t\r\n"), '\n')
}

// nextBorrowedLine hands back the next group of statements to run: the whole
// text at once where it was parsed through first, and one logical line at a
// time where it is read as it is run.
//
// The reader is syntax.Parser.NextLine, which the prompt and the script route
// already use — `driver/program.go` reads a script incrementally, which is why
// the *script* route has always run what it had. The gap this closes is that
// borrowed text did not go through the same reader.
func nextBorrowedLine(p *syntax.Parser, whole **syntax.File) (*syntax.File, bool) {
	if *whole != nil {
		f := *whole
		*whole = nil
		return f, true
	}
	return p.NextLine()
}

// bodyLineOf is which of the lines from at onwards the command in f begins
// on, counting from zero, or -1 where the group holds no command.
//
// A line before it is one the reader stepped over — blank, or a comment — and
// those are not part of the command that follows: measured, an editor leaving
// `# c` / `echo A` has bash record `# c` and `echo A` as two entries.
func bodyLineOf(src string, at int, f *syntax.File) int {
	if len(f.Stmts) == 0 {
		if f.Refused != nil {
			// A line the reader gave up on is still a line it read as a
			// command's, and the list holds it as one.
			return 0
		}
		return -1
	}
	begins := int(f.Stmts[0].Pos().Offset)
	if begins < at {
		return 0
	}
	return strings.Count(src[at:begins], "\n")
}

// borrowedTextFailed reports a parse failure in borrowed text and says what
// the builtin leaves behind.
func (r *Runner) borrowedTextFailed(err error, s sourced, src string) int {
	r.reportBorrowedParseFailure(err, s, src)
	// POSIX makes a special builtin's failure fatal to a non-interactive
	// shell. dash is the only member of the panel that does it here; the
	// other three report the error and carry on.
	if r.ask(r.sem().BuiltinSyntaxErrorFatal, "a parse failure inside a special builtin being fatal") {
		r.fatalQuiet()
		return r.status
	}
	return s.syntaxStatus
}

// reportBorrowedParseFailure writes the diagnostic and nothing else, for the
// two callers that differ only in what they do next: the reader that has
// stopped, above, and the one that gave up a single line and is about to read
// the one after it. Split rather than written twice, because the line number,
// the naming and the echo are three rules and a second copy of them is how one
// of the two ends up saying something slightly different.
func (r *Runner) reportBorrowedParseFailure(err error, s sourced, src string) {
	// The failure's own line, not the caller's. `.` on line 1 of a script
	// that sources a file whose `if` never closes is reported at the
	// line in *that file* by every shell in the panel, and this reported
	// line 1 for all of them.
	// The failure's own line, moved the way every other diagnostic from
	// inside this text is moved. It is a no-op for a sourced file and for the
	// three dialects that number `eval`'s text from one; in the two that
	// continue the caller's lines it is what keeps the two numbers in one
	// sentence agreeing, since `syntax error: unexpected end of file from
	// \`if\' command on line N` carries a second one that has to shift with
	// the first (#2462).
	line, own := r.line, r.diag().ParseFailureLine(err)
	if own > 0 {
		// The route's origin as well as the text's offset, which is what
		// Runner.lineOf adds for every *run-time* diagnostic from the same
		// text. It was missing here, so one column's `eval` parse failure was
		// the only line it wrote on `-c` that did not count from zero —
		// `eval: line 1` against BusyBox's `line 0` (#3141).
		line = own + r.lineBase + r.lineOrigin
		err = shiftParseError(err, r.lineBase+r.lineOrigin)
	}
	d := r.diag()
	if d.BorrowedTextRendersTheCallStack {
		// The chain, and then the innermost text's name with no location
		// after it: the message this shell writes already carries `at line
		// N`, and it does not say the number twice. See
		// Diagnostics.BorrowedTextRendersTheCallStack — the run-time half is
		// Runner.locationPrefixNamed, and the two render one rule from the two
		// places a diagnostic about borrowed text is written.
		chain, innermost := r.borrowedStack(d, r.locationFileOrName())
		r.errf("%s%s: %s\n", chain, innermost, d.ParseFailure(err))
	} else {
		r.errf("%s\n", d.SourceReport(s.naming(d), r.name(), s.sourceName(d),
			line, d.ParseFailure(err)))
	}
	// And the offending line quoted back, for the dialect that writes
	// one. Only when the failure said where it was: `own` is an offset
	// into this text, and the fallback above is the *caller's* line,
	// which would quote a line out of the wrong file (#1728).
	if own > 0 {
		// `own` indexes the borrowed text and `line` is what goes in front of
		// the quote. The same number for both is right only where the text
		// numbers itself from one, and wrong in exactly the dialects the line
		// above shifts (#3194).
		r.errf("%s", d.SourceEcho(s.naming(d), r.name(), s.sourceName(d), line, own, err, src))
	}
}

// evalLinesAxis is the one sentence both askers use, so a refusal reads the
// same whichever of them met it first.
const evalLinesAxis = "the lines of `eval`'s text continuing the caller's"

// runSourced parses src and runs it on this runner.
//
// The status is the last command's, or 0 when nothing ran — which is not the
// same as leaving the status alone. Measured: `false; eval ""` and `false;
// . empty.sh` both end at 0 in every shell in the panel, so an empty script
// *clears* a failure rather than preserving it.
func (r *Runner) runSourced(ctx context.Context, src string, s sourced) int {
	// Text being read again, which is one level of indirection: `eval` and a
	// sourced file both arrive here, and one dialect's trace prefix counts
	// them. See Runner.tracePrefixDepth.
	r.indirection++
	defer func() { r.indirection-- }()
	// And an execution unit, so a bare `exit` or `return` in the text
	// reports what the text has run rather than what the caller left behind.
	// Both routes here are units in the column that keeps the register:
	// measured, `false; eval exit` and `false; . f` with `return` in the
	// file both report 0 there and 1 in the rest. See interp/unitstatus.go.
	defer r.enterExecutionUnit()()
	// Where this text's lines sit, which is a question every borrowed text
	// has to answer and none of them used to: whatever offset was in force
	// stayed in force, so a `. f.sh` inside a command substitution reported
	// the file's first line as the substitution's line — measured 2026-09-12,
	// `x=$(. ./inc.sh)` on line 3 read `$LINENO` as 3 where bash and zsh both
	// say 1.
	//
	// A sourced file's lines are its own in every shell in the panel, so the
	// offset is nothing. `eval`'s text is the axis: three dialects number it
	// from one and two continue the caller's lines, which moves a
	// diagnostic's line and `$LINENO` together — see
	// Semantics.EvalTextContinuesTheCallersLines. The offset there is the
	// line the `eval` word is on, less one, so the text's first line *is*
	// that line. It is the same lineBase a command substitution's body runs
	// under, and for the same reason, so nothing downstream has to know which
	// kind of borrowed text it is inside.
	outerBase := r.lineBase
	r.lineBase = 0
	defer func() { r.lineBase = outerBase }()
	if s.eval && r.line != 1+r.lineOrigin {
		// Asked at the disagreement and nowhere else: on the route's *first*
		// line the two readings are the same offset — nothing — so an `eval`
		// there has nothing to disagree about, and that is the shape most
		// `-c` text has.
		//
		// The first line is `1+r.lineOrigin` and not 1, which is the whole of
		// #3141's eval row. One column numbers a `-c` program from zero, so
		// its first line is 0 and its *second* is 1 — and the old `r.line > 1`
		// read that second line as the first and left the offset at nothing.
		// Measured 2026-09-17 in the digest-pinned alpine image: `ash -c
		// $'echo one\neval "echo )"'` is `eval: line 1` there.
		if r.ask(r.sem().EvalTextContinuesTheCallersLines, evalLinesAxis) {
			// In the route's own numbering, since lineOrigin is added again
			// by everything that renders a line out of this offset. Without
			// the subtraction a zero-based route counts its origin twice.
			r.lineBase = r.line - 1 - r.lineOrigin
		}
		if r.unspecified {
			return 2
		}
	}
	// And the text itself, for a diagnostic that quotes it — at the offset
	// just settled, so a line of the caller's numbering indexes it. See
	// runningText.
	outerText := r.runText
	r.runText = runningText{text: src, base: r.lineBase, borrowed: true}
	defer func() { r.runText = outerText }()
	// What this text is called, for a run-time diagnostic raised inside it:
	// the value that knows is here and the diagnostic is written far away.
	r.borrowed = append(r.borrowed, borrowedText{sourced: s, callerLine: r.line})
	defer func() { r.borrowed = r.borrowed[:len(r.borrowed)-1] }()
	if !s.eval {
		// Inside a *file*, which stops a prompt's wording from reaching the
		// diagnostics of the lines in it: a sourced file is a file however
		// it was reached, and the shell that names one keeps its name and
		// its line at a prompt. See Runner.diag and Runner.AtPrompt (#2024).
		r.borrowedFiles++
		defer func() { r.borrowedFiles-- }()
	} else {
		// And text handed to `eval` is a place of its own to one dialect,
		// which names it rather than the file or the function around it.
		// The mark is the frame count, so anything the text calls stands
		// above it — see Runner.evalTextFloor.
		outerFloor := r.evalTextFloor
		r.evalTextFloor = len(r.frames) + 1
		defer func() { r.evalTextFloor = outerFloor }()
	}
	d := r.dialect().On(s.route())
	// With this shell's alias tables, because borrowed text is text this
	// shell reads: `alias t=echo` before an `eval "t hi"` or a `. f.sh` that
	// uses `t` prints `hi` in every shell of the panel that expands aliases
	// at all, and printed `command not found` here (#2096).
	p := r.ParseWithAliases(src, d)
	var whole *syntax.File
	// Read a line at a time where the dialect reads as it runs, rather than
	// only after a whole-text parse has failed.
	//
	// That used to be the fallback and the note beside it said parsing has
	// no effect of its own, so text that parses runs the same either way.
	// **That is false wherever a line can change how the next one parses**,
	// and an alias is the plain case: `alias a='echo hit'` on one line and
	// `a` on the next prints `hit` from a sourced file in dash and zsh and
	// does not in ksh93 — which is exactly how the three answer
	// sourced.runsWhatItParsed, so it is that axis rather than a new one.
	// A `setopt` or a `shopt` that moves the grammar is the same shape.
	//
	// So the question is now asked of text that parses as well as of text
	// that does not — but only where it can decide anything. A line can
	// change how a *later* line parses and not how its own does, so text
	// with no newline in it reads the same either way and is not asked:
	// `eval "echo hi"` still runs in a runner that has chosen no dialect.
	byLine := false
	if hasALaterLine(src) {
		byLine = r.ask(s.runsWhatItParsed(r.sem()), s.readerAxis())
		if r.unspecified {
			return 2
		}
	}
	// Where the reader has got to in the text, for the caller that is shown
	// the lines rather than the tree. A whole-text read consumes all of it
	// before any of it runs, which is the reading that makes one echo of the
	// whole text right in the dialects that do it — measured 2026-09-21, zsh
	// 5.9.2 writes `echo A` and `echo B` and only then `A` and `B`, where
	// bash interleaves them. So the granularity is this axis's and not a new
	// one.
	consumed := 0
	consume := func(through, body int, whole bool) {
		if s.read == nil || through < consumed {
			// Nothing new was read. A failure can point back at a line
			// already handed over — the construct it left open began there
			// — and consuming from a position behind the cursor would hand
			// over the *next* line, which nobody has read.
			return
		}
		end := min(through, len(src))
		if i := strings.IndexByte(src[end:], '\n'); i >= 0 {
			// Through the newline that ended the last line read, since a
			// reader that stopped at one has read it.
			end += i + 1
		} else {
			end = len(src)
		}
		if end <= consumed {
			return
		}
		s.read(borrowedLines{
			text: src[consumed:end], body: body, whole: whole, dialect: d,
		})
		consumed = end
	}
	if !byLine {
		// Before the parse rather than after it, because text that will not
		// parse was still read: measured, an editor leaving a line bash
		// cannot parse has that line echoed and recorded before the syntax
		// error is reported.
		consume(len(src), 0, true)
		whole = p.Parse()
		if err := p.Err(); err != nil {
			if whole != nil && err == whole.Refused {
				// A line the reader gave up rather than the text — see
				// syntax.File.Refused. A whole-text read has no next line to
				// go on to, so what is left is the status the line left: 1,
				// a failed command's, and not the syntax-error status a text
				// this builtin could not read at all reports.
				r.reportBorrowedParseFailure(err, s, src)
				return 1
			}
			return r.borrowedTextFailed(err, s, src)
		}
	}
	// Whatever a borrowed script reports is the *script's*, not this
	// builtin's. Measured: an unset parameter inside a sourced file is
	// `./f.sh:2: NOPE: parameter not set` in the one dialect that names
	// builtins at all, with no `.` anywhere in it. Leaving the marker set
	// would have put one there on every line the script produced.
	outer := r.inBuiltin
	r.inBuiltin = ""
	defer func() { r.inBuiltin = outer }()

	if s.catchReturn {
		// Inside a sourced file there is something for a `return` to return
		// from, which is the question one dialect asks before allowing one
		// at all.
		r.sourceDepth++
		defer func() { r.sourceDepth-- }()
	}
	// Cleared only when there is nothing to run, which is where the two
	// measured facts part company. `false; eval ""` and `false; . empty.sh`
	// both end at 0 in every shell in the panel, so text with no commands in
	// it *clears* a failure rather than preserving it — but `false; eval
	// "echo $?"` prints 1 in all six, so text with a command in it is shown
	// the caller's status and not a fresh one.
	//
	// This cleared it before the first command instead, which answered the
	// first fact and got the second wrong in the direction nothing catches:
	// borrowed text asking `$?` read 0 after a failure — success, reported at
	// status 0, by the parameter whose whole job is to say otherwise. It is
	// also what a prompt hook needs, since every one of them is handed the
	// status of the line before it (Runner.FireChain).
	ran := false
	// Borrowed text is a *file* of statements as far as giving one up goes,
	// which is what abandoned records: measured, `readonly r=1` and then
	// `r=2` inside `eval` or inside a sourced file reports, gives up that
	// statement and its line, and runs the line after it — in bash 5.3 and
	// bash 3.2 alike. Without this the give-up cost the whole borrowed text,
	// which is the same error as the one this file is named for, one level
	// down. The rule is RunPart's, and it is spelled the same way there.
	//
	// And it stops being a file once the shell is inside a subshell, which is
	// the half that was missing: there the give-up passes straight through
	// and the subshell ends with it. See the clause that reads r.inSubshell
	// below for the measurement (#2747).
	// Borrowed text is the other statement loop that reads a shell's input,
	// so a give-up inside it gives up a line of *this* text — see
	// Runner.giveUpLine, and the note on `abandoned` just below for the
	// measurement. The caller's line goes back afterwards, or a give-up in
	// the function that called this `eval` would give up a line of text that
	// has stopped running.
	outerInputLine := r.inputLine
	defer func() { r.inputLine = outerInputLine }()
	abandoned, stopped := 0, false
	for !stopped {
		f, ok := nextBorrowedLine(p, &whole)
		if !ok {
			break
		}
		if p.Err() == nil {
			// A line the reader stopped inside comes back with its
			// statements taken off it and the error on the parser, so what
			// it consumed is not f.Last but where the failure is. The clause
			// below the loop hands that over; here it would look like a line
			// with no command in it and become an entry per physical line.
			consume(int(f.Last.Offset), bodyLineOf(src, consumed, f), false)
		}
		if f.Refused != nil {
			// A construct in the line did not read and only the line goes
			// with it — see syntax.File.Refused. Borrowed text is read the
			// same way the top level is, so it says so and goes on, leaving
			// the status a failed command leaves. Measured: `. f.sh` and
			// `eval` over a file whose second line holds one both print the
			// first line, complain, run the third, and answer `$?` of 1 when
			// the bad line is the last.
			r.reportBorrowedParseFailure(f.Refused, s, src)
			ran, r.status = true, 1
			continue
		}
		for _, st := range f.Stmts {
			ran = true
			if abandoned != 0 && r.lineOf(st.Pos()) <= abandoned {
				continue
			}
			r.inputLine = r.inputLineOf(st)
			if err := r.stmt(ctx, st); err != nil {
				// A Builtin returns a status and not an error, so there is
				// nowhere for this to go but a diagnostic — the same place
				// runTrapBody puts it, and for the same reason.
				r.diagf("%s: %v\n", s.label, err)
				return 2
			}
			if r.ctl == controlAbandon {
				if r.inSubshell {
					// Except inside a subshell, where the give-up is not
					// caught here at all: it goes on out and the subshell
					// ends with it. Measured 2026-09-15 on bash 5.3.15 and
					// bash 3.2.57 with a reassignment to a readonly name and
					// again with `shopt -s failglob` and an unmatched
					// pattern, in a two-line `eval` so that giving up the
					// line and giving up the text are different outcomes:
					//
					//	readonly r=1
					//	( eval 'r=2
					//	echo inner'
					//	echo sub )
					//	echo after
					//
					// writes `after` alone, where the same `eval` at the top
					// level writes `inner` and then `after` — so this is the
					// subshell changing the answer and not bash's `eval`
					// never catching a give-up. The `!r.inSubshell` half is
					// what the row above it needs.
					//
					// Not an axis: no other column has a give-up to ask it
					// about. Every route to controlAbandon that a script can
					// reach is answered the abandoning way by one preset
					// only — a reassignment to a readonly name, a failed
					// expansion, a refused compound-kind change, a whole-array
					// subscript and an empty table key are all fatal in dash,
					// ksh93, zsh and BusyBox ash — and a fatal error inside
					// borrowed text is FatalErrorEndsBorrowedTextOnly's
					// question, already measured and answered. Measured all
					// the same: ksh93 and zsh both reach the statement after
					// the subshell's `eval`, and dash, BusyBox ash and bash
					// under the name `sh` end the shell over the refusal
					// before the question arises (#2747).
					stopped = true
					break
				}
				r.ctl, abandoned = controlNone, r.abandonLine
				continue
			}
			if r.ctl != controlNone {
				stopped = true
				break
			}
		}
	}
	// A failure the reader reached only because everything before it ran. The
	// text that was parsed through first has already been reported and
	// returned above, so this is the incremental reader alone — and never
	// after an `exit` or a `return`, because a shell that stopped reading
	// never met the line.
	if !stopped {
		if err := p.Err(); err != nil {
			// The lines the reader got through before it gave up, which it
			// read and which therefore count: measured, an editor leaving
			// `for i in a b` / `do` / `fi` / `done` has bash echo the first
			// three and record `for i in a b; do fi`, and never reach the
			// fourth.
			var se *syntax.Error
			if errors.As(err, &se) {
				consume(int(se.Pos.Offset), 0, false)
			}
			return r.borrowedTextFailed(err, s, src)
		}
		// And the lines after the last command, which a reader looking for
		// another one reads and finds nothing in. Not after a stop: a shell
		// that has stopped reading never meets them.
		consume(len(src), -1, false)
	}
	// Cleared only when there was nothing to run, which is where the two
	// measured facts part company. `false; eval ""` and `false; . empty.sh`
	// both end at 0 in every shell in the panel, so text with no commands in
	// it *clears* a failure rather than preserving it — but `false; eval
	// "echo $?"` prints 1 in all six, so text with a command in it is shown
	// the caller's status and not a fresh one.
	//
	// This cleared it before the first command instead, which answered the
	// first fact and got the second wrong in the direction nothing catches:
	// borrowed text asking `$?` read 0 after a failure — success, reported at
	// status 0, by the parameter whose whole job is to say otherwise. It is
	// also what a prompt hook needs, since every one of them is handed the
	// status of the line before it (Runner.FireChain).
	if !ran {
		r.status = 0
	}
	if s.catchReturn && r.ctl == controlReturn {
		r.ctl = controlNone
	}
	// An error the text gave up over, caught here in the dialects that make
	// borrowed text the boundary: the caller then carries on at the command
	// after the builtin, and this is what the builtin reports.
	if st, ok := r.caughtBorrowedError(s); ok {
		return st
	}
	return r.status
}

// caughtBorrowedError catches an error the text just run gave up over,
// reporting the status the builtin should carry.
//
// Nothing is caught unless the dialect says the borrowed text is the
// boundary, and a request to stop is never caught — `exit 7` inside `eval` or
// inside a sourced file exits 7 in every shell in the panel, and so does
// errexit firing there.
func (r *Runner) caughtBorrowedError(s sourced) (int, bool) {
	if !r.pendingFileError() {
		return 0, false
	}
	if !r.ask(r.sem().FatalErrorEndsBorrowedTextOnly,
		"an error inside text a special builtin is running ending that text rather than the shell") {
		return 0, false
	}
	if r.abandon == abandonParamError &&
		r.ask(r.sem().ParamErrorIsAnExitRequest, "`${x?word}` ending the shell rather than the text it is in") {
		// The one operand a dialect calls a request to stop rather than an
		// error, so the catch above does not apply to it.
		return 0, false
	}
	if r.abandon == abandonUsage &&
		r.ask(r.sem().BuiltinUsageErrorEscapesBorrowedText,
			"a builtin's complaint about how it was called ending the shell rather than the text it is in") {
		// The second exception, and the same shape as the first: the
		// dialect catches every fatal error here except the ones a builtin
		// raised about its own call. See
		// Semantics.BuiltinUsageErrorEscapesBorrowedText for the pairs that
		// measured the line.
		return 0, false
	}
	status := r.status
	r.takeFileError()
	if s.fatalStatus != 0 {
		status = s.fatalStatus
	}
	return status, true
}

// parseMessage strips the parser's own "line:col: " prefix.
//
// The parser reports a position because a caller drawing a caret needs one; a
// diagnostic already says where it happened in the dialect's own shape, and
// leaving this on printed it twice.
func parseMessage(err error) string {
	var se *syntax.Error
	if errors.As(err, &se) {
		return se.Msg
	}
	msg := err.Error()
	if i := strings.Index(msg, ": "); i >= 0 && strings.IndexFunc(msg[:i], func(r rune) bool {
		return r != ':' && (r < '0' || r > '9')
	}) < 0 {
		return msg[i+2:]
	}
	return msg
}

// biEval joins its arguments and runs the result.
//
// The join is with a single space and it is the *arguments* that are joined,
// not the original words: `eval echo a b c` prints "a b c" because eval
// rejoins them, and `eval "echo" "a""b"` prints "ab" because the shell had
// already joined those two into one word before eval saw anything. Both are
// unanimous across the panel.
func biEval(r *Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		// Not r.status: an eval with nothing to run reports success even
		// after a failure, which is measured and unanimous.
		return 0
	}
	args, code := r.evalOptions(args)
	if code != 0 {
		return code
	}
	return r.runSourced(ctx, strings.Join(args, " "), sourced{
		eval:         true,
		label:        "eval",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
}

// evalOptions reads whatever of a leading dash-word this shell reads as
// options for `eval`, and hands back the words the text is joined from.
//
// The whole of the dialect's question is Semantics.EvalOptions — three
// answers, because zsh takes the `--` marker and refuses no letter behind it
// and neither of the other two readings covers that. See EvalOptionReading.
//
// Asked narrowly. A first word that does not begin with a dash reaches no
// question — `eval echo hi` is the same command in all seven columns — and
// neither does a lone `-`, which is a command word everywhere here. That
// second exclusion is measured rather than tidy: `eval - echo hi` prints `hi`
// in zsh, which reads as an option being eaten, and is not. `eval "- echo
// hi"` as one argument prints `hi` there too, so it is the text running with
// a bare `-` in command position discarded — a separate fact about that
// shell, filed as issue 3236. A reading that ate the dash here would have
// made `eval - -- echo hi` run `echo hi`, and it runs `--`.
//
// It may hand back nothing — `eval --` is every word being a marker — and the
// caller runs the empty text rather than short-circuiting to 0. A branch that
// returned early was here and was taken out: no probe could tell the two
// apart. `eval --` after a `false` is 0 either way, and the pair is identical
// under `set -x`, under `set -v` and in what `$_` holds afterwards, in this
// shell and in bash 5.3.20. A branch nothing can distinguish reads as a case
// somebody covered.
func (r *Runner) evalOptions(args []string) ([]string, int) {
	if len(args) == 0 || len(args[0]) < 2 || args[0][0] != '-' {
		return args, 0
	}
	switch r.sem().EvalOptions {
	case EvalReadsNoOptions:
		return args, 0
	case EvalTakesTheEndMarkerOnly:
		if args[0] == "--" {
			return args[1:], 0
		}
		return args, 0
	case EvalReadsOptions:
		// The shell's own builtin option reader, with no letters to know.
		// That is not a degenerate call: it is what carries the `--`, the
		// `--help` answer, the wording of the refusal, its usage line and
		// the fatality a special builtin's usage error has in the columns
		// that end the script over it. Writing a refusal here instead would
		// have been a second helper with none of those — which is how this
		// tree keeps growing two spellings of one rule.
		rest, _, code := r.builtinOptions("eval", args, "")
		return rest, code
	}
	r.diagf("%s\n", r.unanswered("how much of a leading dash-word `eval` reads as options"))
	r.status = 2
	r.unspecified = true
	return nil, 2
}

// biDot implements `.`, and `source` where a dialect registers that name too.
//
// `source` is deliberately not in the builtins table: dash does not have it,
// and `command -v source` says so, which makes it a dialect's answer rather
// than the substrate's. The dialects that have it add it in Apply.
func biDot(r *Runner, ctx context.Context, args []string) int {
	// A lookup a second name asked for belongs to *this* call and not to the
	// file it reads: measured, a `.` inside a file that `source` found in the
	// current directory is the ordinary builtin again and does not look there.
	// Taken rather than read, so the flag cannot still be standing while the
	// file runs. See DotLooksInCurrentDirectoryFirst.
	here := r.dotCurrentDirectoryFirst
	r.dotCurrentDirectoryFirst = false
	if len(args) == 0 {
		return r.dotNoOperand()
	}

	args, search, code := r.dotOptions(args)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		// Every option and no operand, which is the same complaint a bare
		// `.` gets — measured, bash prints `filename argument required` and
		// its usage line for `. -p /tmp` exactly as it does for `.` alone.
		return r.dotNoOperand()
	}

	if r.restricted && restrictedPath(args[0]) {
		// A path rather than a bare name, which is the whole of the rule: a
		// restricted shell still reads a file its own PATH finds, and
		// refuses one the script *points* at. Measured — `. f` is silent at
		// 0 while `. ./f`, `. d/g` and `. /etc/profile` are all this
		// sentence at 1 — and the builtin names itself by the word the
		// script invoked it with, so `source d/g` says `source`.
		//
		// Before the resolution rather than after, so a refused path is
		// never looked for: the search is what the mode is confining.
		return r.restrictedOperandRefusal(orElse(r.inBuiltin, "."), args[0])
	}
	display, path, err := r.resolveDotPath(args[0], here, search)
	if err != nil {
		return r.dotFailed(args[0], err)
	}
	if b, ok := r.dotReadsItsOwnInput(path); ok {
		return r.runDotText(ctx, args, display, b)
	}
	// The read is an open to the gate, exactly as the same syscall behind a
	// `<` redirect is: `.` pulls a file into the interpreter and then runs
	// it, which is the re-entry the seams exist for. A denial is routed
	// through the unreadable-file diagnostic rather than allowed()'s generic
	// refusal, so a file the policy withholds is reported the way a file the
	// kernel withholds is — same wording shape, same fatality axis.
	action := r.act(Action{Kind: ActionOpen, Path: path})
	// Not for a pipe this shell made for a substitution in this command:
	// `. <(cmd)` names a path the interpreter chose, so refusing it refuses
	// the construct. See ownPipe.
	if !r.ownPipe(path) && r.openQuietlyDenied(action) {
		return r.dotFailed(args[0], errRefused)
	}
	// Opened by the shell itself, so the one shape where the published name
	// is not the one this process opens applies — see Runner.substOpenPath.
	b, err := r.readFileGated(ctx, &action, r.substOpenPath(path))
	if errors.Is(err, errRefused) {
		// A link that reached a file the policy withholds, reported exactly
		// as the withheld name above is — same diagnostic, same fatality
		// axis, and nothing said about where the link went.
		return r.dotFailed(args[0], errRefused)
	}
	if err != nil {
		if code, isDir := r.dotDirectoryOperand(ctx, args[0], path, action, err); isDir {
			return code
		}
		r.emit(ctx, Event{Kind: EventError, Action: action, Err: err})
		return r.dotFailed(args[0], err)
	}
	r.emit(ctx, Event{Kind: EventAccess, Action: action})

	return r.runDotText(ctx, args, display, b)
}

// dotDirectoryOperand answers a read that failed because the operand names a
// directory, and reports whether it was that.
//
// The panel splits down the middle here, which is why it is a question rather
// than an errno: zsh and dash open the directory, read no commands out of it
// and call that a script that did nothing, while bash and ksh93 call it an
// error — see Semantics.DotDirectoryOperandIsAnError. We reported `no such
// file or directory` at 127, an answer no shell gives, and 127 is the tell:
// it is the status that says the path was never opened, for a path that
// exists (#1577).
//
// Three conditions, and the third is the one that took a measurement to find.
// The read has to have failed, the path has to be a directory, and the
// failure must not be the kernel withholding it — because a directory with no
// read permission is a *permission* failure in every column, not a directory
// one. Measured 2026-09-08 on a mode-000 directory, `. ./noread/`:
//
//	zsh 5.9.2   `permission denied: ./noread/`, status 127
//	dash        `.: cannot open ./noread/: Permission denied`, and it ends
//	bash 5.3.15 `./noread/: Permission denied`, status 1
//	ksh93u+     `.: ./noread/: cannot open [Permission denied]`, and it ends
//
// — no column calls that one a directory, including the two that call a
// readable directory success. Deciding on the stat alone made both of them
// report success for a directory they could not open at all.
//
// After the read rather than before it, so that the gate sees the open the
// shell actually attempted and the event stream records it once.
func (r *Runner) dotDirectoryOperand(ctx context.Context, name, path string,
	action Action, err error,
) (int, bool) {
	if errors.Is(err, fs.ErrPermission) {
		return 0, false
	}
	st, serr := r.stat(path)
	if serr != nil || !st.IsDir() {
		return 0, false
	}
	isError := r.ask(r.sem().DotDirectoryOperandIsAnError,
		"a directory operand to `.` being an error")
	switch {
	case r.unspecified:
		// Refused by name, before the operand is either sourced or complained
		// about — the two readings differ in the status and in whether
		// anything is said at all, so there is no version of carrying on that
		// is not a guess.
		return 2, true
	case !isError:
		// Opened and read to its end, which yielded no commands. That is an
		// empty script: status 0 and nothing said. Measured, `false; . ./;
		// echo $?` is `0` in zsh and dash — the same answer an empty *file*
		// gets in all six columns — and `. ./ && echo ok` prints `ok` in both.
		r.emit(ctx, Event{Kind: EventAccess, Action: action})
		return 0, true
	}
	return r.dotFailed(name, errIsADirectory), true
}

// dotFailed reports a file `.` could not read.
//
// A bare name PATH did not have and a path that would not open are the same
// failure to three of the four and two different messages in dash — "not
// found" for the first and "cannot open …" for the second — so the format is
// chosen by which it was.
func (r *Runner) dotFailed(name string, err error) int {
	format := r.diag().DotCannotOpen
	switch {
	case errors.Is(err, errIsADirectory) && r.diag().DotIsADirectory != "":
		// The one failure bash gives a sentence of its own, and the one it
		// names the builtin in. See Diagnostics.DotIsADirectory.
		format = r.diag().DotIsADirectory
	case errors.Is(err, errNotOnPath) && r.diag().DotNotFound != "":
		format = r.diag().DotNotFound
	case errors.Is(err, errNotOnSearchPath):
		// A list the call named itself, which the one shell that has `-p`
		// words differently from a file it simply could not open — see
		// Diagnostics.DotSearchPathMiss.
		format = orElse(r.diag().DotSearchPathMiss, ".: %[1]s: file not found")
	}
	if r.dotFailureNamesItsOperand() {
		// The location names the file that would not open, in place of the
		// script's own name. Set around the one message rather than written
		// into r.Name: `$0` does not move, measured on the line after. See
		// Diagnostics.DotFailureNamesTheFileItCouldNotOpen.
		outer := r.dotFailureFile
		r.dotFailureFile = name
		defer func() { r.dotFailureFile = outer }()
	}
	r.diagf("%s\n", Wording(format, ".: %[1]s: %[2]s", name, reason(err), r.inBuiltin))
	// dash and ksh93 end the script here; bash and zsh report it and go on.
	if r.ask(r.sem().DotMissingFileFatal, "`.` failing to open a file being fatal") {
		r.fatalUsageQuiet()
		return r.status
	}
	return r.diag().dotCannotOpenStatus()
}

// dotFailureNamesItsOperand answers whether this failure is in the one place
// the operand replaces the name: the top level of a script the shell was
// given.
//
// A function, `eval` and a sourced file each name themselves in that slot and
// keep it — measured in
// Diagnostics.DotFailureNamesTheFileItCouldNotOpen — so the rename is asked
// only where the *script's* name is what would be written, which is where the
// call stack is empty. The `-c` and standard-input routes have no script name
// to replace and are excluded by the route.
func (r *Runner) dotFailureNamesItsOperand() bool {
	return r.diag().DotFailureNamesTheFileItCouldNotOpen &&
		r.Route == RouteScriptFile && len(r.frames) == 0
}

// reason is the part of an os error a shell prints, without the operation and
// the path it already named itself.
//
// Capitalized, because every shell in the panel does: they print the C
// strerror text — "No such file or directory" — where Go's syscall.Errno
// lowercases it. Four dialects differed from the real shell by that one letter
// until this was here.
func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	msg := err.Error()
	if msg == "" {
		return msg
	}
	return strings.ToUpper(msg[:1]) + msg[1:]
}

// errNotOnPath is what resolveDotPath returns when no candidate existed. It
// carries no path because there is no one file to name.
var errNotOnPath = errors.New("no such file or directory")

// errNotOnSearchPath is the same miss with `-p` in front of it, and it is a
// different error because bash words the two differently: a bare operand
// nothing could find is `f: No such file or directory`, and an operand a
// given list could not find is `.: f: file not found`.
var errNotOnSearchPath = errors.New("file not found")

// errIsADirectory is a directory operand, in the dialects that call one an
// error. Written here rather than taken from the read because a read of a
// directory does not fail with the same errno everywhere one is, and because
// the wording ksh93 wants — `cannot open [Is a directory]` — is this sentence
// with reason's capital on it.
var errIsADirectory = errors.New("is a directory")

// DotLooksInCurrentDirectoryFirst makes the next `.` resolve a bare operand
// against the current directory *before* PATH, and returns the function that
// puts the lookup back.
//
// It exists because one shell's second name for `.` is not a synonym for it.
// Measured 2026-09-12 on zsh 5.9.2, in a directory holding `plain.sh` with a
// different file of the same name on PATH:
//
//	source plain.sh   reads the copy in the current directory, status 0
//	. plain.sh        reads the copy on PATH, status 0
//
// and with the file only in the current directory, `source` reads it while `.`
// is `no such file or directory` at 127. So the current directory is not a
// *fallback* there the way Semantics.DotFallsBackToCurrentDirectory is in
// bash — it comes first, and wins over PATH — and the two names disagree about
// it inside one shell.
//
// That last part is why this is not an axis. A Semantics field is one answer
// per runner, so no value of one can make two builtins of the same runner
// resolve the same operand differently.
//
// Nor is it something a dialect could wrap from outside: the search is inside
// resolveDotPath, and a dialect that resolved the file itself and passed
// `./name` on would change the spelling every diagnostic, `$0` and the source
// stack use. Measured on the same shell, a current-directory hit is reported
// as the bare operand — `bad.sh:4: parse error` — where `source ./bad.sh` is
// `./bad.sh:4:` and a PATH hit is the joined path.
//
// The name stays the dialect's to choose. This says what the lookup is, not
// which builtin has it.
func (r *Runner) DotLooksInCurrentDirectoryFirst() (restore func()) {
	outer := r.dotCurrentDirectoryFirst
	r.dotCurrentDirectoryFirst = true
	return func() { r.dotCurrentDirectoryFirst = outer }
}

// resolveDotPath finds the file `.` should read.
//
// An operand with a slash in it is a path and is used as written. Without one
// it is searched on PATH — which surprises people, and is POSIX, and is
// unanimous in the panel, including that PATH wins over an identically named
// file in the current directory.
//
// Only if PATH misses does the current directory come into it, and only in
// bash: measured, `PATH=/usr/bin:/bin; . dotcwd.sh` finds the file in bash and
// is "not found" in dash, ksh93 and zsh. A caller that passes
// currentDirectoryFirst asks the opposite order for this one call, which is
// what one dialect's second name for the builtin needs — see
// Runner.DotLooksInCurrentDirectoryFirst.
//
// It returns two forms of the answer, because two callers want two different
// things. path is the file to *read*, resolved against this runner's
// directory, which is the only form the caller can safely open. display is
// what a diagnostic and the call stack name, which is the path as the shell
// constructed it and not where it resolved to: measured, `. ./inc.sh` is
// reported as `./inc.sh`, a PATH hit as the joined `<dir>/inc.sh`, and the
// current-directory fallback as the bare operand — the resolved absolute path
// appears in none of them, and the shell asking its own source stack gets the
// same spelling the diagnostics use.
func (r *Runner) resolveDotPath(name string, currentDirectoryFirst bool, search dotSearch) (display, path string, err error) {
	if strings.ContainsRune(name, filepath.Separator) {
		return name, r.atDir(name), nil
	}
	if search.given {
		// `-p` replaces the search rather than adding to it, and the
		// fallbacks go with it: measured, `. -p /nowhere f` in a directory
		// holding f is `f: file not found` in bash, where the same call
		// without the option reads it. It wins over the switch below too.
		for _, dir := range search.list {
			if dir == "" {
				dir = "."
			}
			joined := filepath.Join(dir, name)
			if candidate := r.atDir(joined); r.readableFile(candidate) {
				return joined, candidate, nil
			}
		}
		return "", "", fmt.Errorf("%w", errNotOnSearchPath)
	}
	if !r.SearchesPathForSource() {
		// The switch off: the operand is a path relative to this shell's
		// directory and nothing else, which is what a shell with neither
		// half of this mechanism does. See Runner.SearchesPathForSource.
		return name, r.atDir(name), nil
	}
	if currentDirectoryFirst {
		// Before PATH rather than after it, and reported as the bare operand
		// exactly as the fallback below is. See
		// DotLooksInCurrentDirectoryFirst for what was measured.
		if candidate := r.atDir(name); r.readableFile(candidate) {
			return name, candidate, nil
		}
	}
	pathVar, _ := r.getVar("PATH")
	for _, dir := range filepath.SplitList(pathVar) {
		if dir == "" {
			// An empty PATH element means the current directory, which is a
			// POSIX rule and is not the same as the bash fallback below: this
			// one was asked for.
			dir = "."
		}
		joined := filepath.Join(dir, name)
		if candidate := r.atDir(joined); r.readableFile(candidate) {
			return joined, candidate, nil
		}
	}
	if r.ask(r.sem().DotFallsBackToCurrentDirectory, "`.` looking in the current directory after PATH misses") {
		if candidate := r.atDir(name); r.readableFile(candidate) {
			return name, candidate, nil
		}
	}
	if r.posixDotSearchOnly {
		// POSIX mode took the fallback away, so PATH was the whole of the
		// search and the miss is a search miss — the sentence `-p` already
		// reaches, and the one bash writes here. Measured 2026-09-21 on bash
		// 5.3.20: `. notthere` is `notthere: No such file or directory`
		// under bash's own name and `.: notthere: file not found` under
		// `set -o posix`, with `source` naming itself in place of the dot.
		//
		// Gated on the fallback having been *taken away* rather than on the
		// mode, so a shell that never had one keeps its own wording for a
		// PATH miss: dash as `sh` still writes `.: notthere: not found`
		// through Diagnostics.DotNotFound, which this route would step over.
		return "", "", fmt.Errorf("%w", errNotOnSearchPath)
	}
	return "", "", fmt.Errorf("%w", errNotOnPath)
}

// dotNoOperand is `.` with nothing to read, which the options reader reaches
// as well as the builtin's own first line: `. -p /tmp` gets the same
// complaint `.` alone gets, measured in bash.
//
// Four answers in the panel, so this asks two questions rather than guessing.
// dash calls a missing operand success and does nothing at all; the rest call
// it an error, and ksh93 alone makes it fatal.
func (r *Runner) dotNoOperand() int {
	if !r.ask(r.sem().DotWithNoOperandIsAnError, "`.` with no operand being an error") {
		return 0
	}
	// The word the script invoked the builtin by is the verb, for the reason
	// DotCannotOpen takes it: `source` and `.` are one builtin, and bash
	// 5.3.20 writes `source: filename argument required` and `source: usage:
	// source …` for the first (#3217). A wording with no verb ignores it.
	invoked := orElse(r.inBuiltin, ".")
	usage := Wording(r.diag().DotNoOperand, ".: filename argument required", invoked)
	switch {
	case r.diag().DotNoOperandSilent:
		// An error with no sentence, which one column really does write —
		// see Diagnostics.DotNoOperandSilent. Written as a case of the same
		// switch rather than a guard before it, so the status below is
		// reached on this route exactly as it is on the other two.
	case r.diag().DotNoOperandUnprefixed:
		r.errf("%s\n", usage)
	default:
		r.diagf("%s\n", usage)
	}
	// The status is the measured one either way, rather than the generic
	// fatal status: ksh93 ends the script here *and* reports 2, where the
	// fatal status it uses everywhere else is 1. Getting that from
	// fatalQuiet gave 1 and was wrong by one.
	status := r.diag().dotNoOperandStatus()
	if r.ask(r.sem().DotWithNoOperandIsFatal, "`.` with no operand being fatal") {
		// Through the usage door and not stopTheShell, which is what makes
		// the fatality survivable in front of `command`. abandonRequested is
		// the zero value on purpose — a controlExit nobody annotated is a
		// request to stop — so a site that sets the control word and nothing
		// else has said "end the shell" rather than "this builtin failed",
		// and Runner.takeSpecialBuiltinFailure lets a request through.
		//
		// It is a complaint about how the builtin was *called*, which is
		// abandonUsage's own description, and it is the one route into this
		// builtin that was not going through it: `command . /nonexistent`
		// and `command . -Z f` both reported and carried on while `command .`
		// alone ended the script. Measured 2026-09-18 on ksh93u+ 2012-08-01,
		// where the second spelling is an alias for exactly that — `whence -v
		// source` is `source is an alias for 'command .'` — so `source` with
		// no operand writes the usage line, leaves 2 behind, and the script
		// runs on (#3473).
		r.status = status
		r.fatalUsageQuiet()
		r.status = status
	}
	return status
}

// dotSearch is where `.` looks for an operand with no slash in it, which is
// two questions and not one: whether a search happens at all, and over what.
type dotSearch struct {
	// list is `-p`'s argument, split like a PATH. Non-nil means an explicit
	// list was given, empty included — `. -p "" f` reads f from the current
	// directory, because an empty element is the current directory here
	// exactly as it is in PATH.
	list []string
	// given says `-p` was written, which is what separates an explicit empty
	// list from no option at all.
	given bool
}

// dotOptions reads the options `.` and `source` take, which is a dialect
// question before it is an option question.
//
// Four of the five references read a leading dash-word as an option — three
// of them only so as to say they have not got it — and zsh reads it as the
// name of the file, so `. -p dir f` there is a complaint about a file called
// `-p`. Which word the operand is depends on the answer, so it comes first.
//
// A leading `--` is the exception and is taken before that answer is asked,
// because the panel does not disagree about it. Measured 2026-09-22 with an
// absolute path, so PATH search is not a confound:
//
//	. -- /abs/f.sh   zsh 5.9.2, bash 5.3, bash as `sh`, bash 3.2,
//	                 dash and ksh93 all source the file at 0
//
// `source -- /abs/f.sh` is the same in all of them but dash, which has no
// such builtin at all. One `--` and one only: `source -- -- f` in zsh is a
// complaint about a file named `--`, which is what says it is end-of-options
// and not a word the builtin skips.
//
// This was zsh's alone to get wrong and it got it wrong at startup. F-Sy-H
// opens with `builtin source -- "$plugin_dir/lib/lifecycle.zsh" || return`,
// so reading the `--` as the filename made the plugin's first line fail and
// the `|| return` abandoned the rest of it — a real `~/.zshrc` printed
// `(anon):source:29: no such file or directory: --` and loaded no syntax
// highlighting (#4210).
func (r *Runner) dotOptions(args []string) (rest []string, search dotSearch, code int) {
	if len(args) == 0 || !strings.HasPrefix(args[0], "-") || args[0] == "-" {
		return args, search, 0
	}
	if args[0] == "--" {
		// Nothing after it is an option, whatever the dialect answers, so
		// this returns rather than falling into the loop below. The loop
		// keeps a `--` case of its own for the other position it can stand
		// in — after an option that took an argument, as in `. -p /tmp -- f`.
		return args[1:], search, 0
	}
	if !r.ask(r.sem().DotReadsOptions, "`.` reading a leading dash-word as an option") {
		// Not an error: the word is the filename, and resolving it will
		// produce the dialect's own complaint about a file that is not there.
		return args, search, 0
	}
	for len(args) > 0 && strings.HasPrefix(args[0], "-") && args[0] != "-" {
		switch args[0] {
		case "--":
			return args[1:], search, 0
		case "-p":
			if !r.ask(r.sem().DotTakesTheSearchPathOption, "`. -p list file`") {
				if r.unspecified {
					return nil, search, r.status
				}
				return nil, search, r.badBuiltinOption(r.inBuiltin, args[0])
			}
			if len(args) < 2 {
				r.diagf("%s\n", Wording(r.diag().DotOptionNeedsAnArgument,
					".: %[1]s: option requires an argument", args[0]))
				r.builtinUsageLine(r.inBuiltin)
				return nil, search, 2
			}
			search.list, search.given = splitSearchList(args[1]), true
			args = args[2:]
		default:
			if r.unspecified {
				return nil, search, r.status
			}
			return nil, search, r.badBuiltinOption(r.inBuiltin, args[0])
		}
	}
	return args, search, 0
}

// splitSearchList reads `-p`'s argument the way PATH is read, with one
// difference that is not a difference: an argument holding nothing at all is
// a list of one empty element rather than a list of none, so `. -p "" f`
// reads f from the current directory. Measured in bash, and it is the same
// rule an empty element inside a longer list already follows.
func splitSearchList(v string) []string {
	if v == "" {
		return []string{""}
	}
	return filepath.SplitList(v)
}

// atDir resolves a relative path against *this runner's* directory rather than
// the process's.
//
// A Runner carries its own Dir precisely so that two of them embedded in one
// program do not fight over a single process-wide cwd, and reaching for
// os.Stat or os.ReadFile on a bare relative path quietly opts out of that. It
// passed in a test that happened to run from the right directory and found
// nothing anywhere else. redirect.go resolves paths the same way, for the same
// reason.
// An empty path stays empty rather than becoming the directory itself.
// filepath.Join(r.Dir, "") is r.Dir, so joining is how an operand that names
// nothing turns into an operand that names something that certainly exists —
// which is a wrong answer no caller can see is wrong. It made every file test
// a directory answers yes to say yes to an unset variable; see fileTest.
func (r *Runner) atDir(path string) string {
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) && r.Dir != "" {
		return filepath.Join(r.Dir, path)
	}
	return path
}

// readableFile reports whether a path is something `.` could read, which
// excludes a directory: `. /tmp` is not a source file in any shell that
// reports it at all. A method so the probe passes the gate; a candidate the
// policy hides is passed over like a candidate that is not there, and a name
// whose every candidate is hidden ends at the not-found diagnostic.
func (r *Runner) readableFile(path string) bool {
	st, err := r.stat(path)
	return err == nil && !st.IsDir()
}

// runDotText runs what a `.` read, however it read it: the operands after the
// file become its positional parameters, it runs in a frame of its own, and
// the RETURN trap fires as it finishes.
func (r *Runner) runDotText(ctx context.Context, args []string, display string, b []byte) int {
	// Whether the caller's parameters come back, which is not settled until
	// the file has run: one dialect lets a `set` the file ran itself stand.
	// See Semantics.DotSetCancelsTheRestore, asked at the bottom of this
	// function — after the file, because that is when there is something to
	// ask about, and not from inside the deferred restore, because the
	// refusal an unanswered axis writes has a status to leave behind and the
	// return value is already chosen by then.
	restoreParams, passedParams := true, false
	// Arguments after the file become its positional parameters, and are put
	// back afterwards. dash is the exception: it ignores them, so a script
	// there still sees the caller's `$1`. With no arguments at all, every
	// shell leaves the parameters alone — which is why this is guarded on
	// having some rather than on the axis.
	if len(args) > 1 {
		pass := r.ask(r.sem().DotPassesArguments, "`.` giving a sourced file its own positional parameters")
		if r.unspecified {
			// Refused before the file runs rather than after. A sourced file
			// that sees its own `$1` and one that sees the caller's are two
			// different programs, so there is no version of running it that
			// is not a guess — and the refusal would be lost anyway: the flag
			// is cleared per command, so the first line of the file would
			// erase it.
			return 2
		}
		if pass {
			saved := r.Params
			r.Params = append([]string(nil), args[1:]...)
			// The file gets a list of its own, so it gets a mark of its
			// own: what the caller had already replaced is not what this
			// file replaces. Put back below whichever way the restore goes,
			// so the outer list's mark survives the inner file — measured
			// on bash 5.3.20, a sourced file whose own sourced file lets a
			// `set` stand still has its own parameters restored.
			outerReplaced := r.paramsReplacedBySet
			r.paramsReplacedBySet = false
			defer func() {
				if restoreParams {
					r.Params = saved
				}
				r.paramsReplacedBySet = outerReplaced
			}()
			passedParams = true
		}
	}

	// A frame of its own, because a sourced file is a place a script can be
	// *in*: the functions it declares remember it, and a script asking where
	// it is while being sourced means the file rather than whatever sourced
	// it. Named for the builtin, which is what the shells put in the stack —
	// and carrying the operand as the shell constructed it rather than the
	// resolved path, which is the spelling diagnostics and the source stack
	// were measured to use.
	// The operand as well as the file, because `$0` takes the first and a
	// diagnostic takes the second, and a PATH search is where they part
	// company. See Frame.Operand.
	r.pushFrame(Frame{File: display, Name: sourceFrameName, Operand: args[0]})
	// Popped once, wherever the file ends. The RETURN trap below wants the
	// frame already gone and an early return wants it gone too, so the pop
	// is a closure the defer and the ordinary path share rather than two
	// pops that could both run.
	popped := false
	popSourceFrame := func() {
		if popped {
			return
		}
		popped = true
		r.popFrame()
	}
	defer popSourceFrame()

	// Where the `.` itself stands, because that is where a RETURN trap
	// fired from here counts as having fired and the file's own lines are
	// about to walk over the record. Measured 2026-09-13 on bash 5.3.15 and
	// bash 3.2: a two-line RETURN body at the end of a one-line sourced file
	// reports the line of the `.` and one past it, where this engine
	// reported the sourced file's own 1 and 2.
	dotLine := r.line
	st := r.runSourced(ctx, string(b), sourced{
		label:        display,
		syntaxStatus: r.diag().sourcedSyntaxStatus(),
		fatalStatus:  r.diag().SourcedFatalStatus,
		catchReturn:  true,
	})
	// The RETURN trap fires as a sourced file finishes — wherever the trap
	// was set, which is the half of the rule functions do not share. The
	// action sees the file's status, and an `exit` of its own wins.
	r.status = st
	r.line = dotLine
	// The frame goes **before** the trap body runs, which is the half of
	// this rule a function's RETURN does not share. Measured 2026-09-22 on
	// bash 5.3.20, `set -o functrace` with the trap set at the top level:
	//
	//	fn3(){ . ./inc; }; fn3     the trap fires twice, and the firing that
	//	                           ends the *file* reports `${FUNCNAME[@]}`
	//	                           as `fn3 main` and `${BASH_SOURCE[0]}` as
	//	                           the caller's file
	//	. ./inc  at the top level  `${FUNCNAME[@]}` empty, BASH_SOURCE[0] the
	//	                           caller's file
	//	g(){ :; }; g               `g main` — the function's own frame is
	//	                           still there for its own firing
	//
	// So a sourced file's trap runs in the frame that sourced it, and a
	// function's runs in its own. Here the file's frame stood through both,
	// so the action saw `source` where bash saw the caller — and the DEBUG
	// firing in front of the action saw it too.
	popSourceFrame()
	r.runReturnTrap(ctx, sourcedFrame)
	// And now whether the file's own `set` stands. Only a file that was
	// given parameters of its own has a restore to cancel, and only one that
	// replaced them has canceled it — so the axis is consulted where both
	// hold and nowhere else, which keeps the dialect that ignores the words
	// from being asked a question it has no second list for.
	if passedParams && r.paramsReplacedBySet &&
		r.ask(r.sem().DotSetCancelsTheRestore,
			"a `set` in a sourced file standing rather than the caller's positional parameters coming back") {
		restoreParams = false
	}
	return r.status
}

// dotReadsItsOwnInput is the text a `.` of the command's own standard input
// reads, and whether the operand named it.
//
// `/dev/stdin` and `/dev/fd/0` are the command's standard input, which is not
// the process's here: a here-document, a here-string or a pipe into a builtin
// is a stream this shell holds and never put on descriptor 0. Opening the
// path read whatever the *process* was started with instead, so
// `. /dev/stdin <<<'echo hi'` ran nothing at status 0 where bash 5.3.20, zsh
// 5.9.2, ksh93u+ and dash all run the text (#3402).
//
// No gate: nothing is opened. The stream was already this command's.
func (r *Runner) dotReadsItsOwnInput(path string) ([]byte, bool) {
	if path != "/dev/stdin" && path != "/dev/fd/0" {
		return nil, false
	}
	if r.Stdin == nil {
		return nil, true
	}
	b, err := io.ReadAll(r.Stdin)
	r.NoteErrno(err)
	return b, true
}
