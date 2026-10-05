// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// RunStartupFile runs a shell's own startup file — the run-commands file, the
// login profile, `$ENV`, `$BASH_ENV` — as the sourced script it is.
//
// It exists because a startup file *is* one, and ours was not. The front end
// parsed the file itself, for the sake of a diagnostic that names the file the
// way a file is named, and then handed the statements to [Runner.Run] — which
// is the entry point for a *script's own top level*. So there was nothing for
// a `return` to return from, and the refusal that belongs to a script's top
// level fired inside a person's `~/.bashrc`:
//
//	bash: line 2: return: can only `return' from a function or sourced script
//
// Measured across the panel with `echo BEFORE; return 3; echo AFTER` as the
// whole file: all six columns accept it, stop reading the file there, and say
// nothing. Nobody diagnoses. `return` in an rc is how a real `.bashrc` bails
// out early, most often as `[ -z "$PS1" ] && return`, so a refusal here both
// writes a line into every startup and leaves the file running past the point
// it meant to stop (#1422).
//
// The status is where the panel splits, and only over the *argument*. `return`
// with no argument means the status before it everywhere; `(exit 5)` as the
// last line of a startup file leaves 5 everywhere. But `return 3` at the top
// of one leaves 3 in dash, ksh93 and zsh and leaves bash with whatever the
// command before it left. See Semantics.StartupFileReturnCarriesItsArgument,
// which has the grid.
//
// **Read a line at a time, and run as it is read** (#5869). The front end
// used to parse the whole file first and stop when that failed, so one typo at
// the bottom of a `~/.zshenv` ran none of the file and then stopped the shell
// before the command it was started for. Every column reads a startup file
// the way it reads a script — measured 2026-10-04 with `echo before`, an
// unknown command, and then an unclosed `${x` or a stray `)`:
//
//	zsh 5.9.2    .zshenv, .zprofile, .zshrc   before, the two complaints, main
//	bash 5.3.20  $BASH_ENV, .bash_profile,    before, the two complaints, main
//	             .bashrc
//	ksh93u+      $ENV under -i and under -E   before, the two complaints, main
//	dash 0.5.12  $ENV under -i                before, the two complaints, main
//	BusyBox ash  $ENV under -i                before, the two complaints, main
//
// So a failure costs the rest of *that file* and nothing else: zsh goes on to
// read the startup files after it, and the program still runs. That is the
// same boundary an error raised while the file runs has, which the caller
// draws with GiveUpTheFile.
//
// Not through `.`'s reader, which already reads this way, because a startup
// file is not a `.`: ksh93's `.` parses its file whole before running any of
// it and dash's `.` ends the shell over a parse failure, and neither does so
// for `$ENV`. Each line goes through RunPart, which is what ran the whole
// file before, so everything that is not about reading is unchanged.
//
// failed is handed a parse failure, and whether any of the file ran before it,
// and writes it; what it returns is the status the file leaves — the caller's, because how a startup file's failure
// is named and what it leaves in `$?` are the front end's to say: a startup
// file is named by its path in three dialects and by the shell in two. It is
// also handed a line the reader refused on its own (syntax.File.Refused), for
// the writing alone; that line goes and the next one runs, leaving 1, as it
// does on every other route that reads a line at a time. Nil writes the
// failure the way a script's is written, named by the path, and leaves the
// status a script would exit with.
//
// path is what the shell opened, and it is what a diagnostic raised at the top
// level of the file names. Without it the file had no frame, `currentFile` was
// empty, and the location fell back to the shell's own name — which on the
// script route is the script's path, so a `set -u` failure on line 3 of a
// `~/.zshenv` sent a person to line 3 of a script that was fine. Every shell
// in the panel that reads a startup file names the startup file (#1123).
func (r *Runner) RunStartupFile(ctx context.Context, path, src string, failed func(err error, ran bool) int) (int, error) {
	// The frame a `return` returns from. A count rather than a flag, and
	// raised the same way `.` raises it, because a startup file may source
	// another and each of them is its own boundary.
	r.sourceDepth++
	defer func() { r.sourceDepth-- }()
	// And the frame a *diagnostic* names, which is what `.` pushes for the
	// same reason. Marked as the shell's own rather than a call, so that the
	// one dialect whose `$0` follows the stack keeps answering with the
	// shell — see Frame.Startup.
	r.pushFrame(Frame{File: path, Startup: true})
	defer r.popFrame()
	// And what the shell is inside while it reads it, which is the file and
	// not yet the program: see EvalContextStartupFile.
	defer r.enterEvalContext(EvalContextStartupFile)()
	if failed == nil {
		failed = func(err error, _ bool) int {
			r.errf("%s", r.diag().ParseDiagnostic(path, "", err, src))
			return r.diag().StatusForParseError(err)
		}
	}
	// The line this file has got to is the file's own, for the one dialect
	// that locates a parse failure there (Diagnostics.ParseFailureIsLocatedWhereTheProgramGotTo):
	// a failure on the first line of `$ENV` is not at the line the file read
	// before it reached. Put back afterwards so the program after it starts
	// from where it would have.
	outerReached := r.reachedLine
	r.reachedLine = 0
	defer func() { r.reachedLine = outerReached }()
	// A file the shell reads, so the grammar is a file's — `ksh -c` ends an
	// unterminated quote at the end of its string, and no file it reads gets
	// that — and the aliases are the ones defined so far, by this file's own
	// earlier lines as much as by a file before it: `alias a='echo hit'` and
	// then `a` in a `~/.zshenv` prints `hit` in zsh and bash alike.
	route := syntax.RouteFromScriptFile
	p := r.ParseWithAliases(src, r.dialect().On(route))
	// And the grammar a line sets is the one the next line is read in — the
	// rule the front end's own reader and `.`'s keep, for the same reason: a
	// `setopt` or a `shopt -s extglob` in a startup file is there for what
	// comes after it.
	dialectRead := r.Dialect
	// Whether any of the file ran before the failure, which is the one thing
	// besides the failure itself that the status after it turns on in one
	// dialect — see Diagnostics.StartupParseFailureStatusFor.
	stopped, ran := false, false
	for {
		if r.Dialect != dialectRead && r.Dialect != nil {
			dialectRead = r.Dialect
			p.SetDialect(r.ParsingDialect(r.dialect().On(route)))
		}
		// Reading the next unit of a file, which is one of the moments a held
		// signal waits for. See Runner.ReadingTheNextUnitOfInput.
		r.releaseSignalsHeldForInput()
		f, ok := p.NextLine()
		if !ok || p.Err() != nil {
			// The end, or a line the reader stopped inside — which comes back
			// with its statements taken off it, so none of it runs.
			break
		}
		if f.Refused != nil {
			failed(f.Refused, ran)
			r.status, ran = 1, true
			continue
		}
		ran = ran || len(f.Stmts) > 0
		if err := r.RunPart(ctx, f); err != nil {
			return r.status, err
		}
		if r.ctl != controlNone {
			// `return`, `exit`, or an error that costs the file: whichever it
			// was, nothing more of the file is read, and the caller says what
			// it meant.
			stopped = true
			break
		}
	}
	if err := p.Err(); err != nil && !stopped {
		// Everything before it has run, and the rest of the file goes. A
		// shell that had stopped reading never met the line.
		r.status = failed(err, ran)
	} else if !ran && r.status != 0 && r.ask(r.sem().StartupFileThatRunsNothingLeavesZero, "the status a startup file with no command in it leaves") {
		// A file with no command in it, which zsh takes as having left 0 and
		// every other column takes as having left nothing (#5883).
		r.status = 0
	}
	if r.ctl == controlReturn {
		// Caught, so the shell goes on to the next startup file and then to
		// the prompt rather than staying in a returning state. Exactly what
		// `.` does at the end of a sourced file, and for the same reason.
		r.ctl = controlNone
		if !r.ask(r.sem().StartupFileReturnCarriesItsArgument, "the status a `return` in a startup file leaves") {
			// bash: the argument is dropped and the shell's status is
			// whatever the last command before the `return` left. Measured:
			// an rc of `return 3` leaves 0 at the first prompt and one of
			// `false; return 3` leaves 1.
			//
			// Read from the field the RETURN trap reads, which is the same
			// moment asked about for a different reason — the trap's action
			// sees `$?` as the `return` found it. One field rather than two,
			// because a `return` that set one and not the other would be
			// answering the same question twice and drifting.
			r.status = r.returnSeenStatus
		}
	}
	return r.status, nil
}
