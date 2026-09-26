// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
	"errors"
	"strings"

	"github.com/blairham/sh/syntax"
)

// commandSubst runs the text of a `$( … )` and returns what it wrote.
//
// The inner text was kept raw by the lexer rather than tokenized, because
// what is inside is a program. So it is parsed here, with the same dialect,
// and run on a copy of the state: a substitution is a subshell, and nothing
// it assigns escapes.
//
// Trailing newlines are removed, which is the rule that makes `x=$(pwd)`
// usable at all.
func (r *Runner) commandSubst(ctx context.Context, span syntax.Span) string {
	// A substitution written into a subscript runs once for the whole
	// expansion, however many of the readers of those brackets ask for their
	// text. The two roads to a subscript — the rendered text and the text
	// with its quotes still on — both arrive here, which is why the hold is
	// on the substitution rather than on either of them. See
	// subscriptSubstHold (#3240).
	if v, ok := r.heldSubscriptSubst(r.expandingWord, span.Pos); ok {
		return v
	}
	// And the same for the names one brace fan made, in the columns that run
	// a fanned word's expansions once rather than once per name. Held here
	// for the reason above: an argument and a redirection target fan by
	// different roads and arrive at the same expander. See braceFanHold
	// (#4694).
	if v, ok := r.heldBraceFanWork(span.Pos); ok {
		return v
	}
	v := r.runCommandSubst(ctx, span)
	r.holdSubscriptSubst(r.expandingWord, span.Pos, v)
	r.holdBraceFanWork(span.Pos, v)
	return v
}

// runCommandSubst is commandSubst with the hold taken off: one run of the
// body, every time it is called.
func (r *Runner) runCommandSubst(ctx context.Context, span syntax.Span) string {
	src := span.Value
	// The program in here is a parse of its own, so a hold keyed by where a
	// span sits would answer the wrong span for it — and the body runs
	// commands, which fan braces of their own. Put down for the body and
	// picked up afterwards, so the value this call produces is still held
	// for the other names of the fan outside. See braceFanHold.
	defer r.suspendBraceFan()()
	// An assignment with no command name reports what the substitutions in
	// it reported, and reports success when there are none. Those are two
	// different facts and neither can be read off the status afterwards —
	// `false; x=$(false)` leaves the same 1 that was already there — so the
	// fact that one ran is recorded rather than inferred.
	r.substRan = true
	// Pathname expansion is the same containment asked of globbing. A word
	// that substitutes as *text* expands with it suspended — an assignment's
	// value, a `[[ ]]` operand, a redirection target — and that is a rule
	// about what the word's own metacharacters mean, not about the commands
	// inside a substitution in it. Those are ordinary commands and they glob.
	//
	// Measured 2026-09-11 in a directory holding `aa` and `bb`: `v=$(echo *)`
	// leaves `aa bb` in bash 5.3.15, dash, ksh93 and zsh 5.9.2 — a unanimous
	// panel — where this left the asterisk, at status 0 with nothing said
	// (#1962). `n=$(( $(echo * | wc -w) ))` answered 1 against the panel's 2
	// for the same reason, an arithmetic operand suspending it just as an
	// assignment does.
	//
	// Cleared here rather than around the places that suspend it, which is
	// the distinction #1955 turns on: `case "aa bb" in $(echo *))` globs
	// inside the substitution and matches, in this shell and in the panel, so
	// the flag has to keep not reaching a pattern operand's own text while
	// stopping at the boundary of a program.
	if r.globSuspended {
		r.globSuspended = false
		defer func() { r.globSuspended = true }()
	}

	// The body, parsed. Four of the seven columns have read it already — with
	// the line that holds it, before any of that line ran — and for those
	// this is the second read of the same text. See
	// Runner.readLineSubstitutions for which and for why the first read does
	// not stand in for this one.
	f, base, _, ok := r.readSubstBody(span)
	if !ok {
		return ""
	}

	// The body parsed, so what runs now runs at a level of its own: a quote
	// or a brace the text *around* this substitution left open is not open
	// inside its body, and a refusal in there belongs to the inner level.
	// See substLevel — this is what keeps `echo "$(echo $(for))"` from
	// claiming the outer quote for the inner body (#3355).
	defer r.atFreshSubstLevel(span)()

	if v, answered := r.arithmeticBodySubstitution(f, span); answered {
		// A body that is nothing but `(( … ))` is read as the arithmetic
		// expansion in the one dialect that reads it that way — before the
		// two forms below, because it is not a substitution at all there and
		// neither of them should see it. See
		// Runner.arithmeticBodySubstitution for the three rows that say the
		// text is never run as a command.
		return v
	}
	if span.CurrentShell {
		// Here rather than on a copy, which is the whole of why this
		// spelling exists: `${ x=1;}` leaves x set where `$(x=1)` does not.
		//
		// The `<file` form below is reached from here only where a dialect
		// says so, because the columns split: `${ <f ;}` is the file's text
		// in ksh93, the empty string in bash 5.3, and `bad substitution` in
		// zsh 5.9.2 and bash 3.2, which have no such spelling to read. The
		// split is what makes it an axis rather than a property of the form
		// — see Semantics.CurrentShellSubstitutionReadsAFile.
		if r.lang().ReadFileSubstitution && !span.ReplyValue {
			if rd, ok := readFileSubstitution(f); ok &&
				r.ask(r.sem().CurrentShellSubstitutionReadsAFile,
					"`${ <file ;}` being the file's contents rather than the empty string") {
				return r.readFileSubst(ctx, rd, span)
			}
		}
		return r.currentShellSubst(ctx, f, base, span)
	}

	// `$(<file)` is the file, with nothing run. Asked after the parse because
	// the body is source until it is expanded, and before the subshell
	// because there is no command here for a subshell to hold.
	if r.lang().ReadFileSubstitution {
		if rd, ok := readFileSubstitution(f); ok {
			return r.readFileSubst(ctx, rd, span)
		}
	}

	var out bytes.Buffer
	sub := r.clone()
	// A command substitution is a boundary for a writing body's output where
	// a subshell is not: the value is read the moment this returns, so a body
	// still writing into it has to be joined first. See Runner.collectBodies
	// for the measurement.
	defer sub.collectBodies()()
	sub.inheritJobs(jobBoundarySubstitution)
	sub.inCommandSubst = true
	// Its arrays are a view of the caller's rather than a fork's copy, which
	// is the reading an explicit `( … )` gets too — see
	// Runner.unsetEmptiesAnUnwrittenArray, where the contexts that do not
	// get it are measured.
	sub.arraysAreAView = true
	// **Whether the body's shell holds `set -e`** — the one option a
	// substitution does not simply inherit, and a disagreement rather than a
	// gap. See Semantics.ErrExitEntersACommandSubstitution for the panel.
	//
	// Asked only when the option is on, which is the only place the columns
	// differ: an axis consulted on the common path is one every `$(…)` pays
	// for and that no Runner built without a preset could leave unanswered.
	//
	// The field is cleared rather than the failure being let through,
	// because the body can see which it is: measured, `set -e; echo "[$(case
	// $- in *e*) echo E;; *) echo none;; esac)]"` is `[none]` in bash and
	// BusyBox ash, and `set -o` inside the same body reports `errexit off`
	// there. A body that still carried the letter and merely declined to
	// stop would answer `[E]` and be wrong about itself.
	if sub.errexit && !r.ask(r.sem().ErrExitEntersACommandSubstitution,
		"`set -e` reaching into a command substitution's own shell") {
		sub.errexit = false
	}
	// And the third level of indirection, beside `eval` and a sourced file:
	// measured, `set -x; echo $(:)` traces the body at `++ ` in the one
	// dialect that counts them. See Runner.tracePrefixDepth.
	sub.indirection = r.indirection + 1
	sub.lineBase = base
	// The body is not the shell's own input, which one line counter asks
	// about — see Runner.commandLine.
	sub.inSubstBody = true
	if span.Backquoted {
		// The older spelling's body is the text its own refusals quote, in
		// the dialect that quotes one: “ v=`echo $(for)` “ echoes `echo
		// $(for)` rather than the script's line, measured on bash 5.3.20.
		// See runningText.
		sub.runText = runningText{text: src, base: base, borrowed: true}
		// And the body is text read at expansion time, which the same
		// dialect names in the location of anything refused *inside* it: a
		// `$( … )` written in a backquoted body is `command substitution:`
		// there, where the same `$( … )` written on the script's line is
		// not. See Runner.substFailureRoute.
		sub.inBodyReadAtExpansion = true
	}
	// A body with nothing in it reports success, however the shell holding it
	// was doing. The clone carries the caller's status in so that `$?` inside
	// the body reads the command before the substitution — measured, `false;
	// echo $(echo $?)` writes 1 in every column — and with no command in
	// there to overwrite it, that borrowed status was what came back out.
	//
	// Measured 2026-09-26 from a script file, the whole panel — bash 5.3.20,
	// bash 3.2.57, ksh93u+ 2012-08-01, dash 0.5.12, zsh 5.9.2 and BusyBox ash
	// 1.37.0 — unanimous on all four rows, where this answered 1 for the
	// first three:
	//
	//	false; X=$( )          0    nothing but spaces
	//	false; X=$(\n)         0    nothing but a newline
	//	false; X=`# comment`   0    nothing but a comment
	//	false; echo $(echo $?) 1    the body still reads the caller's status
	//
	// Not a rule of the assignment's: `false; ``` is 0 in the panel too, and
	// that is a word rather than a right-hand side. It is the substitution
	// that reports the success, which is why it is set here — and why it had
	// to be, since the empty-command rule one layer up in Runner.simple now
	// carries a word's substitution out to the command (#4589). Without this
	// the panel's 0 for that line would have become the 1 standing in `$?`.
	if len(f.Stmts) == 0 {
		sub.status = 0
	}
	sub.Stdout = &out
	// The same group a subshell gets, and the same lifetime: the expansion
	// does not finish until the body has. See Runner.anchorForkedBody.
	defer sub.anchorForkedBody()()
	if _, err := sub.Run(ctx, f); err != nil {
		r.diagf("%v\n", err)
		return ""
	}
	// A substitution *inside* this body could not parse its own body, and in
	// the columns that end the script for that the word holding this one is
	// never expanded at all. Taken here as well as at the sequence point in
	// Runner.stmt, because a statement that holds the failing substitution
	// has already begun: measured 2026-09-20 from a script file,
	// `echo A$(echo B$(for)C)D` writes nothing on standard output and leaves
	// 1 in zsh 5.9.2, where this wrote `AD` and left 0 — the stop was in the
	// box and the next command was the one that never ran (#3355).
	//
	// The box rather than the body's own control flow, so the column that
	// contains such a failure in a subshell still contains it: nothing is
	// recorded there, so there is nothing here to take. See
	// Semantics.SubstitutionParseErrorEscapesASubshell and
	// substitutionstop.go.
	//
	// Peeked rather than taken, because every level of nesting is such a
	// boundary: `echo "$(echo "$(echo "$(for)")")"` has three, and a take at
	// the innermost would leave the two outside it expanding their words.
	// The sequence point in Runner.stmt, or the prompt's own drain, still
	// takes it.
	if status, stopped := r.pendingScriptStop(); stopped {
		r.status = status
		// The same annotation Runner.stmt puts on it: an error the shell
		// reported rather than a request to stop, so a prompt above draws
		// the next one (#3300).
		r.ctl, r.abandon, r.errexitStopped = controlExit, abandonSubstParse, false
		return ""
	}
	// The status of a substitution is the status of what ran inside it, which
	// `x=$(false)` relies on.
	r.status = sub.status
	// And what its `alias` *named* outlives it in one column, as an explicit
	// `( … )`'s does. See Runner.adoptAliasNames.
	r.adoptAliasNames(sub)
	// A fork finished with: a real shell reads a substitution's output from
	// a pipe to a child, and reaps that child here. See Runner.childReaped.
	r.childReaped()
	return strings.TrimRight(out.String(), "\n")
}

// currentShellSubst runs a `${ … ;}` or `${| … ;}` body on this runner.
//
// Everything it does outlives it, so there is nothing to clone and nothing to
// merge back — only the output to catch and the writer to put back
// afterwards. The status is the body's last command's for the same reason: it
// is this runner's status, set where every other command sets it.
//
// **The two spellings are one function because they differ in one thing**:
// where the value comes from. The blank form's is what the body printed, so
// its output is caught; the pipe form's is what the body left in `$REPLY`, so
// its output is not caught at all and goes wherever the shell's was going —
// measured 2026-09-13, `echo "[${| echo printed; REPLY=val; }]"` on bash
// 5.3.15 writes `printed` on its own line and then `[val]`. Splitting them
// into two functions would put the line offset, the control-flow break and
// the diagnostic in two places, where a fix to one is a fix to neither.
func (r *Runner) currentShellSubst(ctx context.Context, f *syntax.File, base int, span syntax.Span) string {
	var out bytes.Buffer
	savedOut, savedBase := r.Stdout, r.lineBase
	// Same fact the subshell form gets on its clone, and saved because this
	// spelling runs on the runner it was written in — see Runner.commandLine.
	savedInBody := r.inSubstBody
	r.inSubstBody = true
	defer func() { r.inSubstBody = savedInBody }()
	// The body is a command list of its own, and the fields that say what
	// *this* command's assignments did are the running command's rather than
	// the shell's. Saved across the body for that reason: every simple
	// command clears Runner.substRan on its way in — see the top of
	// Runner.simple — so
	//
	//	readonly q=1; a=${ q=2; }; echo $?
	//
	// reported 0 here against ksh93's 1, the assignment with no command name
	// having been told by its own body that no substitution had run in it.
	// Invisible until the stop above was contained, because the shell used
	// to leave before anything read the status.
	saved := r.assignBookkeeping()
	defer func() { saved() }()
	// The body is a frame a `return` leaves, in both columns that have this
	// spelling. See Runner.hasSomethingToReturnFrom.
	r.currentShellSubstDepth++
	defer func() { r.currentShellSubstDepth-- }()
	// **Whether the body's shell holds `set -e`**, which is the same
	// question the subshell spelling asks and the same answer per column —
	// see Semantics.ErrExitEntersACommandSubstitution, whose doc carries the
	// rows for this spelling too. The body runs in *this* shell, so the
	// option is put back rather than thrown away with a clone.
	//
	// Without it a `false` in the body ended the script in silence where
	// bash 5.3.20 answers the body's output and carries on: measured
	// 2026-09-22, `set -e; echo "${ false; echo in; }" x; echo after` writes
	// `in x` and `after` there and wrote nothing at all here.
	if r.errexit && !r.ask(r.sem().ErrExitEntersACommandSubstitution,
		"`set -e` reaching into a `${ … ;}` body, which runs in this shell") {
		r.errexit = false
		defer func() { r.errexit = true }()
	}
	// And an execution unit of its own, which is the shape #3184 filed: a
	// bare `exit` in the body reports what the body has run and 0 where it
	// has run nothing, while `$?` in the same body still reads the value the
	// shell came in with. See interp/unitstatus.go for the rows and for the
	// controls that put the other five units beside this one.
	defer r.enterExecutionUnit()()
	putBackReply := func() {}
	if span.ReplyValue {
		putBackReply = r.localizeReply()
	} else {
		r.Stdout = &out
	}
	// And in one of the two columns that have the spelling the body is a
	// variable scope as well: a declaration inside it is local to the body
	// and the shell's own name comes back afterwards. See
	// Semantics.CurrentShellSubstitutionBodyIsAScope, and Runner.pushScope
	// for why this is the call's own unwind rather than a lighter copy of it.
	//
	// Opened *inside* the hiding above and closed before it, which is the
	// order rather than a preference: a body that declares REPLY unwinds
	// into the hidden name, and the outer one is put back over that. The
	// hiding stays a mechanism of its own because it says something a scope
	// cannot — the body starts with **no** REPLY, set or unset, which is
	// measured — and localizeReply has the rows.
	closeScope := func() {}
	if r.ask(r.sem().CurrentShellSubstitutionBodyIsAScope,
		"a `${ … ;}` body being a variable scope of its own") {
		sc := r.pushScope(false)
		closeScope = func() { r.popScope(sc) }
	}
	// The body was parsed on its own, so its lines count from one; the script
	// it was written in did not. **The offset readSubstBody worked out**, which
	// is the one the subshell form is given and the one the refusal in there was
	// placed by — this line used to compute a second copy of it from the span
	// alone, so the dialect's own numbering reached the `$( )` spelling and not
	// this one. That is the hazard readSubstBody's own note names: two copies of
	// one rule, and the body's runner placing a command where the refusal would
	// not have. Put back afterwards because this runner goes on being used
	// (#4155, #4239).
	r.lineBase = base
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.Stdout, r.lineBase = savedOut, savedBase
			closeScope()
			putBackReply()
			r.diagf("%v\n", err)
			return ""
		}
		if r.ctl != controlNone {
			// `exit`, a `return` or a `break` inside the body stops it.
			// What that does to the shell the body was written in is
			// boundCurrentShellBody's, below, and it is not one answer for
			// all three.
			break
		}
	}
	r.Stdout, r.lineBase = savedOut, savedBase
	r.boundCurrentShellBody()
	if span.ReplyValue {
		// Read before the name is put back, and taken whole: this is a
		// parameter's value rather than captured output, so the trailing
		// newlines the other form strips are text here. Measured,
		// `v=$'a\n\n'; echo "[${| REPLY=$v; }]"` keeps both.
		v, _ := r.getVar("REPLY")
		closeScope()
		putBackReply()
		return v
	}
	closeScope()
	return strings.TrimRight(out.String(), "\n")
}

// assignBookkeeping saves the fields an assignment with no command name reads
// to decide its status, and hands back what puts them all back.
//
// One function rather than five locals at the call site: they are read
// together in one `else if` chain and a restore that dropped one of them would
// be a status bug nothing else in the package could catch. Runner.unspecified
// is deliberately not among them — an axis no dialect answered is the shell's
// news and not the command's, and it has to reach the caller.
func (r *Runner) assignBookkeeping() func() {
	substRan, assignFailed, expandErr := r.substRan, r.assignFailed, r.expandErr
	badSubscript := r.badSubscript
	disciplineSet, disciplineStatus := r.disciplineStatusSet, r.disciplineStatus
	return func() {
		r.substRan, r.assignFailed, r.expandErr = substRan, assignFailed, expandErr
		r.badSubscript = badSubscript
		r.disciplineStatusSet, r.disciplineStatus = disciplineSet, disciplineStatus
	}
}

// boundCurrentShellBody says what a `${ … ;}` body's unwinding costs the
// shell that held it.
//
// Three kinds of control reach here and they do not answer alike, which is
// why this is a switch and not the single `break` it replaced.
//
// **A loop control goes out.** `for i in 1 2 3; do echo "top $i"; x=${ break;
// }; echo "bottom $i"; done` prints `top 1` and stops, in ksh93 93u+ and bash
// 5.3.20 both — the `break` leaves the substitution, leaves the assignment it
// was the value of, and breaks the loop. `continue` the same. Unanimous, so
// nothing is asked and nothing changes.
//
// **A `return` is contained**, and so is a statement given up over an error a
// dialect does not call fatal. Both are ones this engine had wrong in every
// dialect. For the give-up, measured 2026-09-16 on bash 5.3.20 — the only
// column that has both the construct and a survivable readonly reassignment:
//
//	readonly q=1
//	a=${ q=2; }; echo "funsub after=$?"
//
// reports the refusal and then `funsub after=1` there, where here the
// give-up left the substitution and took the rest of the line with it. For
// the `return`, measured the same day,
//
//	f() { r=${ return 3; }; echo "after-funsub $?"; echo still-in-f; }
//	f; echo "after f $?"
//
// prints `after-funsub 3`, `still-in-f` and `after f 0` in ksh93 and bash
// alike, and here the function returned 3 on the spot. Both columns that have
// the construct agree, so this is a fix rather than an axis — the value of
// the substitution is what the body printed before the `return` and its
// status is the `return`'s.
//
// **A stop is the dialect's**, and the one the two columns split on. See
// Semantics.CurrentShellSubstitutionBoundsAnUnwind: bash ends the shell,
// ksh93 ends the substitution and carries on with the script. It covers an
// error the shell reported as well as a requested `exit`, because ksh93's
// containment does — takeFileError is what clears both, and clearing the kind
// with the control is what keeps a later `exit` from being read as an error
// by the next boundary up.
func (r *Runner) boundCurrentShellBody() {
	switch r.ctl {
	case controlReturn, controlAbandon:
		r.ctl = controlNone
	case controlExit:
		if r.ask(r.sem().CurrentShellSubstitutionBoundsAnUnwind,
			"a stop raised inside `${ … ;}` ending the substitution rather than the shell") {
			r.takeFileError()
		}
	}
}

// localizeReply hides `$REPLY` for the duration of a `${| … ;}` body and hands
// back what puts it there again.
//
// Measured 2026-09-13 on bash 5.3.15, and it is three facts rather than one:
//
//	REPLY=outer; echo "[${| echo "in=[${REPLY+SET}]" >&2; REPLY=x; }]"
//	                                          in=[]      the body starts with none
//	REPLY=outer; v=${| REPLY=inner; }; echo "$REPLY"
//	                                          outer      the outer one comes back
//	unset REPLY; v=${| REPLY=inner; }; echo "${REPLY+SET}"
//	                                          (empty)    and comes back *unset*
//
// So absent and empty are two states here, which is why the set-ness is saved
// beside the value — the same tri-state localizeGetoptsCursor keeps, and for
// the same reason. The name is hidden rather than merely deleted because a
// deleted name still reads through to the environment: `REPLY=outer sh -c
// 'echo "[${| true; }]"'` would answer `[outer]` off the inherited value.
//
// **What this is not is the body's scope**, and it is deliberately not it.
// The body *is* a variable scope in bash — see
// Semantics.CurrentShellSubstitutionBodyIsAScope, which is what makes `local`
// legal inside one — and the scope is opened inside this hiding and closed
// before it, so a body that declares REPLY shadows the hidden name rather
// than the one being put back. Four rows say the two are separate and not one
// mechanism looked at twice: with the scope in force and `REPLY=outer`,
// `${| REPLY=in; }` is `in` with `outer` back afterwards, `${| typeset
// REPLY=in; }` is the same, `${| :; }` is empty rather than `outer`, and an
// unset REPLY is unset again after `${| REPLY=in; }` — all measured on bash
// 5.3.20 and all answered the same way before the scope existed, because the
// body starts with no REPLY at all and a scope cannot say that.
//
// #2656 recorded a fifth row as the visible corner of the missing scope:
// `${| unset REPLY; }` reading bash's *outer* REPLY, on the reasoning that
// unsetting a local there reveals what it shadows. **Re-measured 2026-09-16
// on bash 5.3.20, that is not what it does** — `REPLY=outer; v=${| unset
// REPLY; }` is the empty string there, with `REPLY` still `outer` after, which
// is what this hiding answers on its own and answered before the scope
// landed. So the note is corrected rather than carried: there was one gap
// here and not two.
func (r *Runner) localizeReply() func() {
	held, inVars := r.Vars["REPLY"]
	wasRemoved := r.removed["REPLY"]
	delete(r.Vars, "REPLY")
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed["REPLY"] = true
	return func() {
		if inVars {
			if r.Vars == nil {
				r.Vars = map[string]string{}
			}
			r.Vars["REPLY"] = held
		} else {
			delete(r.Vars, "REPLY")
		}
		if wasRemoved {
			r.removed["REPLY"] = true
		} else {
			delete(r.removed, "REPLY")
		}
	}
}

// bodyDialect is the dialect a substitution's body is read again with: this
// runner's, plus the comment rule the text was *first* read under.
//
// The body is kept as text and parsed a second time when it runs, so the two
// reads have to agree about what a `#` is. Only the front end can know —
// syntax.Dialect.Comments is a statement about a line somebody typed — which
// is why the answer travels on the span rather than living on the runner:
// `eval` and `.` take a *value* and read a `#` the ordinary way in the shell
// this is about, measured, and both of those go through this runner too.
func (r *Runner) bodyDialect(span syntax.Span) syntax.Dialect {
	d := r.dialect()
	d.Comments = span.Comments
	if !span.Backquoted && d.SubstitutionBodyRefusesASteppedOverSeparator &&
		d.SeparatorWhereACommandBelongs != syntax.NoSeparatorWhereACommandBelongs {
		// A `;` standing where a command belongs is read here by the
		// dialect's own rule everywhere but inside the newer spellings'
		// bodies, where one shell takes it only as an and-or's missing
		// operand. See syntax.Dialect.SubstitutionBodyRefusesASteppedOverSeparator
		// for the fourteen measured rows, and note that the *older* spelling
		// keeps the shell's ordinary answer — which is why the span decides
		// rather than the construct.
		//
		// Guarded on the dialect having an answer to narrow: setting the
		// narrower value over a dialect that steps over nothing would widen
		// it, and accept the one position that dialect refuses.
		d.SeparatorWhereACommandBelongs = syntax.SeparatorOnlyWhereAnAndOrWantsOne
	}
	return d
}

// lang is what language this shell speaks, by pointer and without a copy.
//
// The dialect below is 384 bytes and is returned by value, which is right for
// the handful of callers that hand it to a parser and wrong for the far more
// numerous ones that read a single axis off it to answer a question about a
// word. Those are in the inner loops: a startup on a real ~/.zshrc called
// dialect **630,000 times**, and 280,000 of those were one function —
// describesRatherThanSpells asks seven questions and, written as seven
// `r.lang().X` reads, copied the struct seven times to answer them.
//
// So a reader takes this and a parser takes dialect. The difference is not
// cosmetic: this returns the runner's own value, and the two fields dialect
// *adjusts* — ArithPrecedence and CharacterWidth — are therefore not this
// function's to answer. TestNothingReadsAnAdjustedAxisOffLang keeps that
// true, and also keeps anything from writing through the pointer, since a
// runner with no dialect of its own is handed a value every other such
// runner is sharing.
func (r *Runner) lang() *syntax.Dialect {
	if r.Dialect != nil {
		return r.Dialect
	}
	return &coreLanguage
}

// coreLanguage is what a runner with no dialect of its own speaks. Built once
// because syntax.Core builds a map, and never written to — see lang.
var coreLanguage = syntax.Core()

// dialect is the dialect this runner parses nested input with. It is a method
// rather than a field so the default is the core rather than the zero value,
// which would be posix and would refuse constructs the outer parse accepted.
func (r *Runner) dialect() syntax.Dialect {
	// The core is built only when there is nothing to build it for. It is not
	// a constant -- syntax.Core carries a map of the declaration utilities,
	// so every call allocates one and fills it -- and this method is called
	// once per nested parse, which on a real startup is hundreds of thousands
	// of times. Written as `d := syntax.Core()` with the runner's own dialect
	// assigned over it, that map was built and thrown away on every one of
	// them: 156MB of the 494MB a startup allocated, and a runner that is
	// running always has a dialect, so it was thrown away every time.
	var d syntax.Dialect
	if r.Dialect != nil {
		d = *r.Dialect
	} else {
		d = syntax.Core()
	}
	if r.arithPrecedenceMoved {
		// A dialect's run-time option has moved where the arithmetic
		// operators bind, and everything parsed from here on has to be
		// read that way — see Runner.arithPrecedence. Applied to the
		// dialect rather than kept beside it, so the one parser this
		// runner builds nested input with is told once.
		d.ArithPrecedence = r.arithPrecedence
	}
	// Where a character ends, which the grammar has to know before it can say
	// which bytes are metacharacters and cannot work out for itself: the
	// locale is this runner's state and whether the reader decodes it at all
	// is the dialect's answer. Handed over as a function rather than as a
	// codeset so that a `LC_ALL=` on one line is in force for the next, the
	// same reason ArithPrecedence above is applied here rather than kept
	// beside the dialect. See syntax.Dialect.CharacterWidth (#4235).
	d.CharacterWidth = r.CharacterWidth
	return d
}

// ArithPrecedence is the order the binary arithmetic operators bind in for
// this runner right now: the dialect's, unless a dialect's own option has
// moved it.
func (r *Runner) ArithPrecedence() syntax.ArithPrecedencePolicy {
	return r.dialect().ArithPrecedence
}

// SetArithPrecedence moves it, for a dialect whose option namespace has a
// name for the other order — zsh's `c_precedences` is the one in this tree,
// and it is the reason the order cannot live on syntax.Dialect alone.
//
// Asking for the order the dialect already parses with still counts as
// having moved it, and that is deliberate rather than an oversight: what the
// flag records is that an option is now in charge, so turning it back off
// puts the dialect's own answer back rather than leaving the last request
// standing.
func (r *Runner) SetArithPrecedence(p syntax.ArithPrecedencePolicy) {
	r.arithPrecedence, r.arithPrecedenceMoved = p, true
}

// substParseErrorAtItsCloser re-reads a `$( … )` body with the parenthesis
// that closed it on the end, and hands back the refusal that produces.
//
// # The token every shell in the panel names, and the one we named instead
//
// The body is read on its own here, so when its parse runs out it has run out
// of *input* — and every dialect has a measured rule for what that means,
// each of which this engine already implements. The script's own reader never
// runs out: it meets the `)`. So the rule that fired was the right rule for
// the wrong question, and the answer was a token the reference never writes
// (#3296).
//
// Measured 2026-09-16 from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C
// <shell> case.sh` with stdin from /dev/null, over ten bodies of the form
// `v=$(echo hi; X)` — `for`, `if`, `while`, `until`, `case`, `{`, `(`, `f()`,
// a trailing `|` and a trailing `&&`. BusyBox v1.37.0 in the digest-pinned
// Alpine image internal/oracle reaches, under `--init`.
//
//	bash 5.3.20   names `)` in all ten
//	ksh93u+       names `)` in eight; `case` and `(` consume the closer and
//	              are `` `(' unmatched `` instead, and `f()` parses
//	zsh 5.9.2     names `)` in six of the eight it refuses; `while` and
//	              `until` read on and block
//	dash, ash     name `)` in **nine**; `for` alone is their own sentence
//
// So it is not a dialect disagreement and it is not asked as one. It is a
// fact about where the body ends, and the right token falls out of each
// dialect's *existing* rules once the parser is shown the closer — including
// dash's, which judges whatever stands in a loop variable's place as a name
// and so goes on writing `Bad for loop variable` for `for` while gaining
// `")" unexpected` for the other nine.
//
// That last column is why the issue's premise was narrower than the bug: it
// measured `for` alone, found dash and BusyBox byte-perfect there, and
// recorded them as already right. They were right on the one construct every
// shell on the panel treats specially.
//
// # Only the parenthesised spelling
//
// The older spelling is left alone because it is already right: “ v=`echo
// hi; for` “ is `newline` in both bash builds, `for` in zsh and `for` in
// ksh93, which is what this shell writes for it today. A backquoted body ends
// at its backquote rather than where its contents end — see
// syntax.Span.Backquoted — and the columns read it with the script, which is
// the same reason Diagnostics.BackquotedSubstitutionRestartsLines exists one
// message over.
//
// `${ … ;}` is named in the guard as a statement of scope rather than as a
// live branch, and that is said here because a mutant which took it out
// survived. Its spelling *requires* a `;` or a newline before the closing
// brace, so such a body always ends at a token the parser meets and can never
// run out at its end — which is the only thing this function changes.
// Measured 2026-09-16: `v=${ echo hi; for ;}` is “ `;' unexpected “ and the
// same body written over three lines is “ `newline' unexpected “, both
// byte-identical to ksh93u+ before this change and after it. Taking the name
// out would be relying on that; leaving it in says which spelling this is
// about.
//
// # Diagnostic only
//
// The tree from the first parse is already being thrown away — the parse
// failed — so nothing runs from this and nothing is kept. It is on the error
// path alone, so the ordinary body pays nothing for it. And where the body
// *with* the closer parses, there is no second refusal to prefer and the
// first one stands: `v=$( (echo hi )` is the shape, where the missing
// parenthesis is the body's own.
func (r *Runner) substParseErrorAtItsCloser(span syntax.Span, src string, err error) error {
	if span.Backquoted || span.CurrentShell {
		return err
	}
	// A body that ran out with a condition still open is the one shape where
	// the reference does **not** name the closer: it names the last token it
	// read and follows with a sentence about the substitution. Showing the
	// parser the `)` there would answer the right question for a body that
	// met it and the wrong one for a body that never did. See
	// Diagnostics.SubstitutionParseFailureSentence for the ten rows and for
	// the pair one token apart that separates them.
	if r.diag().SubstitutionParseFailureSentence != "" && ranOutInACondition(err) {
		return err
	}
	p := r.ParseWithAliases(src+")", r.bodyDialect(span))
	p.Parse()
	if closed := p.Err(); closed != nil {
		return closed
	}
	return err
}

// ranOutInACondition reports whether a parse failure is the input running out
// while an `if` or `elif` condition was still being read.
//
// The keyword the reader was inside rather than the one that opened the
// construct, which is the difference the rows turn on: `if true; then` is
// inside the `then` clause of an `if` and is answered the other way. See
// syntax.Error.Innermost.
func ranOutInACondition(err error) bool {
	var se *syntax.Error
	if !errors.As(err, &se) || se.Kind != syntax.ErrUnterminated {
		return false
	}
	return se.Innermost == "if" || se.Innermost == "elif"
}

// substFailureLocatedByNameAlone drops the line from a body's refusal where
// the dialect locates that one failure by the shell's name and nothing else,
// and hands back the undo.
//
// The flag is scoped to the one write rather than to the refusal, and that is
// the measurement rather than a convenience. Measured 2026-09-21 on zsh 5.9.2
// from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C zsh -f s.sh` with
// standard input on the null device, `v=$(echo hi; foo())` on line 1:
//
//	s.sh: parse error near `)'
//	s.sh:1: parse error near `v=$(echo hi; foo())'
//
// So the first message has no line and the second has one. A rule applied to
// the whole refusal writes the second one bare as well, which is a row of its
// own rather than a detail — the two messages are what the sweep in #3961
// compares.
//
// Reached only from the expansion route. The *parse* route asks the same
// question in Diagnostics.ParseDiagnostic, through the same predicate, so the
// two cannot drift: a body read with the script is refused there and a body
// read when the word is expanded is refused here.
func (r *Runner) substFailureLocatedByNameAlone(failure error) func() {
	if !r.diag().locatesByNameAlone(failure) {
		return func() {}
	}
	r.locatedByNameAlone = true
	return func() { r.locatedByNameAlone = false }
}

// substRunBase is the offset a substitution's body is *run* at, which is the
// offset its text sits at in every dialect but one.
//
// bash numbers a body's commands from the line the construct's **closing**
// delimiter is on: the body's first command is that line, and the lines below
// it in the body count up from there. Measured 2026-09-23 against bash 5.3.15
// in the pinned debian:sid-slim and bash 5.3.20 on macOS, which agree — a
// script file, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin from /dev/null,
// `echo one` on line one and the substitution written from line two:
//
//	body                                   physical  closer  bash  zsh
//	`$(⏎echo "L=$LINENO"⏎)`                       3       4     4    3
//	`$(⏎⏎echo "L=$LINENO"⏎)`                      4       5     5    4
//	`$(⏎⏎⏎echo "L=$LINENO"⏎)`                     5       6     6    5
//	`$(⏎echo x⏎echo "L=$LINENO"⏎)`                4       5     6    4
//	`$(⏎echo "L=$LINENO"⏎⏎⏎)`                     3       6     6    3
//	`$(⏎# c⏎echo "L=$LINENO"⏎)`                   4       5     5    4
//	`$(echo "L=$LINENO"⏎)`                        2       3     3    2
//	`$(⏎echo "L=$LINENO")`                        3       3     3    3
//	`${⏎echo "L=$LINENO"⏎}`                       3       4     4    —
//
// Row four is what says the anchor is the *first command* rather than every
// command: the second command of that body is one below the closer, not on
// it. Row six is what says it is the first command and not the first line —
// a comment before it takes the closer's line and the command is below it,
// which is why this counts to the tree's first statement rather than counting
// newlines. Rows seven and eight are the controls: with the command and the
// closer on one line every column agrees, so nothing here is a constant
// offset.
//
// **The earlier reading of this was measured wrong and recorded as the
// opener's line** (#4155). Five shapes, all of them with the body's first
// command directly under the opener, where the opener's line and "one below
// the physical line" coincide — and the third row of that table, the one
// written down as the control that ruled out a constant offset, is row three
// here and answers 6 rather than 2 in both binaries. It is not a libc split
// and not a version split; it was simply not what either shell does. See
// docs/spec/oracle.md on grading against the reference the instrument grades
// against.
//
// The refusal is *not* this number — a body that will not parse is reported
// at the line its text is on in every column, bash included, because the
// refusal comes out of the scan that has not reached the closer yet. That is
// why readSubstBody carries two offsets.
//
// A body with no commands in it has nothing to anchor, so the count of
// newlines it opens with stands in: there is no first statement to ask, and
// the only positions left in it are its own refusals.
func (r *Runner) substRunBase(span syntax.Span, src string, f *syntax.File, textBase int) int {
	if !r.diag().SubstitutionBodyIsNumberedFromWhereTheShellWasReading {
		return textBase
	}
	if span.Backquoted && r.diag().BackquotedSubstitutionRestartsLines {
		// The older spelling is numbered from the top of its own body in two
		// columns, and textBase already carries that — see
		// Diagnostics.BackquotedSubstitutionRestartsLines.
		return textBase
	}
	// The line the command that holds this substitution reports itself at,
	// which is where the shell's reader had got to — Runner.commandLine is the
	// whole of why that is the closer's line for `v=$( … )` and the command's
	// own line for `cmd "$( … )"`.
	anchor := r.line - r.lineOrigin
	if anchor <= 0 {
		return textBase
	}
	// Which line of the body that number belongs to, and the two spellings
	// differ: the newer one skips the newlines between the opener and the
	// first command, so the first command *is* the anchor however many were
	// written, while the older one counts them, so the anchor is the line the
	// backquote is on and the body's first line is one below it. Measured
	// 2026-09-23 over nine backquoted shapes — `` v=`echo "L=$LINENO"` `` on
	// line 2 answers 2, and the same body with the backquote ending the line
	// answers 5 with the closer on line 4.
	if span.Backquoted {
		return anchor - 1
	}
	// A body with no commands has nothing to anchor, so the newlines it opens
	// with stand in — the only positions left in it are its own refusals. A
	// comment in front of the first command takes no line of its own either,
	// which falls out of counting to the tree's first statement.
	first := leadingNewlines(src) + 1
	if f != nil && len(f.Stmts) > 0 {
		first = int(f.Stmts[0].Pos().Line)
	}
	// Deliberately the first statement's own line, and **not** the body's
	// second line, which six measured shapes want and five others refuse: a
	// body that starts a command on the opener's line is numbered one lower
	// than this from its next line down, and taking that here moved a command
	// in a `while`, an `if`, a `case` and a `for` the other way. bash's number
	// in there is its parser's line counter rather than a function of the
	// text, and the shapes are in docs/spec/grammar/substitutions.md under
	// what is measured and not held.
	return anchor - first
}

// leadingNewlines counts the newlines a substitution's body opens with, which
// is how far one dialect's numbering of it is out from the file's.
//
// Blanks and tabs before a newline count as part of it: `$(  ⏎echo x)` is the
// same shape as `$(⏎echo x)` to a reader and to that shell. Anything else
// ends the count, because the body has begun.
func leadingNewlines(src string) int {
	n := 0
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n':
			n++
		case ' ', '\t':
		default:
			return n
		}
	}
	return n
}

// spanLineBase is how far into the script a span's own text begins: the line
// the span carries, taken through whatever was between it and the file.
//
// Two offsets rather than one, because a span reaches here by two roads. The
// script's own text is numbered from the file, and Runner.lineBase says how
// far into it the input being run started — a function body, a sourced file,
// a substitution's body. A text lexed **again at run time** is numbered from
// itself, and Runner.substFragmentLine says where that text begins; an
// arithmetic expression is the one that arrives that way. Adding only the
// first located a `$( … )` refused inside a `$(( … ))` at line 1 whatever
// line held it, in every dialect (#3810).
func (r *Runner) spanLineBase(span syntax.Span) int {
	return r.lineBase + r.substFragmentLine + int(span.Pos.Line) - 1
}

// substSource is a substitution's body text as the parse that runs it should
// see it: its own text, and then any here-document bodies the *outer* lexer
// read for it put back where its own text would have held them.
//
// The second half is empty in every dialect but the two that read a body that
// way — see syntax.Span.CarriedHeredocs and
// syntax.Dialect.HeredocBodyFromAfterTheCommand (#3711).
//
// Text rather than a tree, and that is the shape both callers were already
// built on: the lexer's sub-parse throws its tree away and a body is re-read
// against the alias table as it stands when the substitution runs, so a tree
// carried out of the lexer would answer the wrong column for
// `alias t=echo; v=$(t hi)`. Handing back the text costs one re-parse that
// was happening anyway.
//
// A delimiter line of its own after each body, so that the re-parse sees a
// here-document which closes. Where the body ended is a question the outer
// lexer has already answered; this is not a second reader of it.
func (r *Runner) substSource(span syntax.Span) string {
	var carried []*syntax.Redirect
	for _, c := range r.carriedHeredocs {
		if c.At == span.Pos {
			carried = c.Redirs
			break
		}
	}
	if len(carried) == 0 {
		return span.Value
	}
	var b strings.Builder
	b.WriteString(span.Value)
	for _, rd := range carried {
		b.WriteString("\n")
		if rd.Heredoc != nil {
			for _, sp := range rd.Heredoc.Spans {
				b.WriteString(sp.Value)
			}
		}
		b.WriteString(rd.Word.Literal())
		b.WriteString("\n")
	}
	return b.String()
}

// readSubstBody parses a substitution's body and reports the refusal where
// this shell reports one, answering the tree, the offset the body's own lines
// are counted from, and whether it read at all.
//
// Two callers, and the second is the whole of why it is a function: a body is
// read when the substitution runs in three dialects and **with the line that
// holds it** in four, and those two moments have to produce the same sentence
// at the same place or the split becomes a second diagnostic rather than a
// second moment. See Runner.readLineSubstitutions and
// syntax.Dialect.SubstitutionBodyRead (#2857).
func (r *Runner) readSubstBody(span syntax.Span) (*syntax.File, int, int, bool) {
	// The body's own text, and any here-document bodies read for it from the
	// lines after the enclosing command — see substSource, which is span.Value
	// in every dialect but the two that read a body that way.
	src := r.substSource(span)
	// The alias tables go on it, because a substitution's commands are
	// commands: `alias t=echo; v=$(t hi)` leaves `hi` in v in every shell of
	// the panel that expands aliases at all, and left it empty here (#2096).
	// Parsed whole rather than a line at a time, which is measured — an
	// alias defined on a substitution's first line does not reach its
	// second in dash, ksh93 or zsh.
	//
	// **The table as it stands now**, which is what says the read with the
	// line does not replace this one. Measured 2026-09-19, `shopt -s
	// expand_aliases` then `alias t=echo; v=$(t hi); echo "[$v]"` on one
	// line: bash 5.3 answers `[hi]`, so the body it refused with the line is
	// still expanded against the table the line went on to change, while
	// dash answers `t: not found` and `[]` from the read it did first. So
	// bash reads the body twice and dash reads it once, and this shell reads
	// it twice in both — which leaves dash's alias row where it was and
	// leaves bash's right. See
	// `alias/nested-text-expands-where-the-command-string-did-not`.
	p := r.ParseWithAliases(src, r.bodyDialect(span))
	// The text is a substitution's body, so the end of it is the construct's
	// closing delimiter rather than the end of a program, which one lenient
	// closing context can tell apart. Told to both spellings, unlike the
	// `$( )` fact just below: a closing parenthesis and a second backquote
	// close a list alike, and zsh leaves both spellings of `echo x &&`
	// holding `x`. See syntax.Parser.InsideASubstitution.
	p.InsideASubstitution()
	if !span.Backquoted && span.Kind == syntax.CommandSubst {
		// The text is the inside of a `$( )`, cut out of the script by the
		// lexer, and that is a fact only this call site still holds: the
		// parentheses are not in `src`. What turns on it is where a
		// here-document in the body ends — at the end of the text, or at the
		// delimiter the last line of it begins with, in the one dialect that
		// reads it that way.
		//
		// **The backquoted spelling is left out and that is measured**, not
		// symmetry: `` v=`cat <<E` ⏎ `w` ⏎ `E ` `` answers `[w⏎E ]` in bash
		// 5.3, body, exactly as bash 3.2 has it. The old-style substitution
		// does not take the route. See
		// syntax.Dialect.HeredocLastLineIsADelimiterPrefix.
		p.InsideProgramParentheses()
	}
	// Where the body's *text* sits in the script, so that what is refused in
	// it is refused where a reader can find it. The span's own line is the
	// body's first, because a span starts at its opening delimiter — and it
	// accumulates, so a substitution inside a substitution is still placed in
	// the file rather than in whichever body most recently began. One dialect
	// numbers the older spelling from the top of the body instead, which is
	// Diagnostics.BackquotedSubstitutionRestartsLines.
	textBase := r.spanLineBase(span)
	if span.Backquoted && r.diag().BackquotedSubstitutionRestartsLines {
		textBase = 0
	}
	f := p.Parse()
	// And where its *run* counts from, which is a second question with a
	// second answer in one dialect — see substRunBase, and
	// Diagnostics.SubstitutionBodyIsNumberedFromWhereTheShellWasReading for the shapes.
	// Two numbers rather than one because the refusal below wants the text's
	// and the runner that runs what parsed wants the run's; a single base is
	// how the two came to be one wrong answer for both.
	base := r.substRunBase(span, src, f, textBase)
	if err := p.Err(); err != nil {
		// A substitution re-parses, so the syntax-error status is the
		// dialect's here too — not only in whatever first read the script.
		//
		// Whether it is fatal is the dialect's, and it was a constant here.
		// dash, ksh93 and zsh abandon the script; bash reports the failure,
		// expands the word to the empty string and carries the statement and
		// the script on, exiting 0. They all detect it when they parse the
		// whole input; this parses the body at expansion time, so either
		// outcome has to be produced deliberately. Without the stop the
		// diagnostic appeared and the next command ran regardless, which is
		// the shape this package keeps finding — and with it always taken, a
		// script bash finishes stopped here (#2703).
		// Worded by the dialect, and located in the *script* rather than in
		// the body. `%v` on a *syntax.Error prints the parser's own
		// coordinates — `1:3: ";" unexpected` — which is an internal
		// position string arriving in front of a user, inside a message
		// whose prefix has already named the right line in the file (#2460).
		// Its sibling helper for the other two substitution spellings has
		// asked the dialect since it was written; this one never did.
		//
		// Shifted by the same base the body's runner is given, so a refusal
		// and a command that failed in the same body are placed alike.
		// Without the shift the one dialect that writes the line *into* its
		// sentence — `syntax error at line N:` — counted from the body and
		// disagreed with its own prefix.
		// And the tag that goes with scoping it to the word: the one
		// dialect that does not stop names the construct the refusal came
		// from, because the script's own line is no longer the whole story.
		// Both of the next two are the older spelling's alone, and for one
		// reason: the column that scopes this failure to the word reads a
		// `$( … )` body *with the script*, so that spelling's refusal is the
		// line's there — untagged, and fatal. Reproducing the outcome
		// without moving when the body is read means asking about the
		// spelling, which is what Diagnostics.BackquotedSubstitutionRestartsLines
		// already does one message over.
		construct := r.substFailureRoute(span)
		raw := r.substParseErrorAtItsCloser(span, src, err)
		// The refusal's own base, which is the body's where the body was
		// read at expansion time and has lines of its own. See
		// Runner.expansionBodyLine for why this is not Runner.lineBase.
		failureBase := textBase
		if !span.Backquoted && r.expansionBodyLine > 0 {
			failureBase += r.expansionBodyLine - 1
		}
		failure := shiftParseError(raw, failureBase)
		// Placed at the failure's line and followed by the text it was
		// found in, for the dialects that write one. See substecho.go.
		putBack := r.substFailureAtItsLine(span, src, failure)
		// The sentence, with the clause one dialect adds while it is still
		// looking for the closing parenthesis — see Runner.substBodyExpecting.
		// Around this write alone, because the refusal one dialect locates by
		// name is followed by a message that still carries a line.
		byName := r.substFailureLocatedByNameAlone(failure)
		r.errf("%s", r.diagLineNamed(construct, "%s%s\n",
			r.diag().ParseFailure(failure), r.substBodyExpecting(span, failure)))
		byName()
		if echo, at := r.substFailureEcho(span, src, raw, failure, failureBase); echo != "" {
			if at > 0 {
				r.line = at
			}
			// One message per line, because each of them is located: a
			// refused body two substitutions deep writes a line for every
			// level that has something open, and zsh puts the file and the
			// line in front of all of them. Measured 2026-09-20,
			// `echo "$(echo "$(echo "$(for)")")"` on line 1 of a script:
			// `s.sh:2: unmatched "` three times over, not one prefix and
			// three sentences.
			for _, line := range strings.SplitAfter(echo, "\n") {
				if line == "" {
					continue
				}
				r.errf("%s", r.diagLineNamed(construct, "%s", line))
			}
		}
		putBack()
		if span.Backquoted && !r.ask(r.sem().SubstitutionParseErrorIsFatal, "a substitution body that does not parse ending the shell") {
			// The word expands to nothing and the statement goes on, which
			// is a failed *expansion* rather than a failed script. The
			// status is left alone for the same reason: the command that
			// holds the word is about to run and report its own.
			return nil, 0, 0, false
		}
		// **How far the abandonment reaches**, which is a second question and
		// the one #3274 was: a subshell contains it in ksh93 and does not in
		// bash 5.3, bash as `sh`, zsh, dash or BusyBox ash, where the script
		// ends wherever the substitution was written. See
		// Semantics.SubstitutionParseErrorEscapesASubshell for the panel and
		// substitutionstop.go for why the answer is recorded in a box every
		// clone shares rather than checked at each of the seven boundaries a
		// shell clones at.
		//
		// Asked only from inside one, which is where the columns differ: at
		// the top level there is nothing to escape and every column already
		// agrees, so a script without subshells never reaches the axis.
		// The number the refusal leaves behind. Read before the stop is
		// recorded, so the box a subshell's failure travels in and the
		// status this shell reports are the same number rather than two
		// readings of one rule — see Runner.substParseFailureStatus for the
		// column that moves and for what says the failing text is not the
		// script's own line.
		_, offTheScriptsLine := r.borrowedAtLocation()
		status := r.substParseFailureStatus(offTheScriptsLine)
		if r.inSubshell &&
			r.ask(r.sem().SubstitutionParseErrorEscapesASubshell,
				"a substitution body that does not parse ending the script from inside a subshell") {
			r.recordScriptStop(status)
		}
		r.status = status
		// **An error the shell reported, not a request to stop**, which is
		// what every boundary in fileabandon.go splits on. It was raised
		// through stopTheShell and so arrived at those boundaries as
		// abandonRequested — the zero value that file calls "the answer for
		// a site that has not thought about it" — and the one that pays for
		// it is the interactive prompt: `v=$(echo hi; for)` typed at a
		// prompt ended the *session* in every dialect, where all seven
		// reference columns report it and draw the next prompt (#3300). The
		// annotation is the whole fix; Runner.GiveUpTheLine already catches
		// an error and already lets a request through.
		r.ctl, r.abandon, r.errexitStopped = controlExit, abandonSubstParse, false
		return nil, 0, 0, false
	}
	return f, base, textBase, true
}

// readSubstitutionsUpTo parses the bodies of the substitutions a dialect reads
// with the line that holds them, for every line of f up to and including
// limit, and answers how far through the list it got.
//
// **When** a `$( … )` body is parsed is a dialect question and the panel is
// split three ways, which syntax.Dialect.SubstitutionBodyRead records. What
// falls out of it is what a reader sees: measured 2026-09-19 from a script
// file, `echo before; v=$(if); echo after` writes nothing at all in dash,
// BusyBox ash, bash 5.3 and that build as `sh` — the line is refused before
// the `echo` in front of the substitution runs — and writes `before` first in
// bash 3.2, ksh93 and zsh, which read the body only when the word is
// expanded. `false && v=$(if); echo "after=$?"` is the control that makes it
// a statement about *parsing*: the substitution is never reached, and the
// four columns above still refuse the line where the other three print
// `after=1` with nothing said. Either row alone is ambiguous — the first
// could be a shell that gives up the line, the second a shell that swallows
// the failure — and together they can only be the parse moment.
//
// **The logical line is the unit**, which is measured and is why this takes a
// limit rather than reading the whole file: a script whose line 1 is a
// `printf` and whose line 2 holds the refused body writes `start` first in
// every one of the four, and so does a sourced file and a multi-line `eval`
// text. The front ends already read a line at a time, so limit is the whole
// file for them; an embedder handing a whole script to Run gets the shell's
// own granularity from here rather than a different one.
//
// The tree is **thrown away**, which is measured rather than convenient: bash
// 5.3 expands a body against the alias table as it stands when the word is
// expanded, not the one the line was read under — `alias t=echo; v=$(t hi)`
// on one line answers `[hi]` there — so a kept tree would be wrong for that
// column. dash keeps its own and answers `t: not found`, which is a second
// question this does not answer: its column of
// `alias/nested-text-expands-where-the-command-string-did-not` is where it
// shows, and it is no worse for this.
//
// The list is in reading order and a line never goes backwards in it, so a
// cursor is enough to say what has been read.
func (r *Runner) readSubstitutionsUpTo(f *syntax.File, from int, limit int32) int {
	for from < len(f.Substitutions) && f.Substitutions[from].Pos.Line <= limit {
		span := f.Substitutions[from]
		from++
		if !r.readNestedSubstitutions(span) {
			// Reported, and the shell has been told to stop. The rest of the
			// line's substitutions are not read for the same reason the rest
			// of the line does not run.
			return from
		}
	}
	return from
}

// readNestedSubstitutions reads one body and then the bodies inside it,
// reporting whether every one of them parsed.
//
// Recursive, because the nesting is: `v=$(echo $(if))` is refused with the
// line in bash 5.3 where “ v=`echo $(if)` “ and “ v=$(echo `if`) “ are
// not, so it is the spelling of **each** body and not of the outermost one. A
// body is one unit however many lines it runs to — it is inside a line
// already — so there is no limit here, only in the caller.
//
// The offset moves with the recursion so that a refusal inside a nested body
// is still placed in the file rather than in whichever body most recently
// began.
func (r *Runner) readNestedSubstitutions(span syntax.Span) bool {
	body, _, textBase, ok := r.readSubstBody(span)
	if !ok {
		return false
	}
	if len(body.Substitutions) == 0 {
		return true
	}
	outer := r.lineBase
	r.lineBase = textBase
	defer func() { r.lineBase = outer }()
	for _, inner := range body.Substitutions {
		if !r.readNestedSubstitutions(inner) {
			return false
		}
	}
	return true
}
