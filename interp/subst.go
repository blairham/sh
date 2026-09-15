// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"context"
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
	src := span.Value
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

	// The alias tables go on it, because a substitution's commands are
	// commands: `alias t=echo; v=$(t hi)` leaves `hi` in v in every shell of
	// the panel that expands aliases at all, and left it empty here (#2096).
	// Parsed whole rather than a line at a time, which is measured — an
	// alias defined on a substitution's first line does not reach its
	// second in dash, ksh93 or zsh.
	// **Here rather than where the line was parsed**, which four of the seven
	// columns do not do. Measured 2026-09-15 without an alias anywhere:
	// `echo before; v=$(if); echo after` writes `before` in bash 3.2, ksh93
	// and zsh and writes nothing in dash, bash 5.3, that build as `sh` and
	// BusyBox ash — so the split runs through bash, and #2357's reading of it
	// as dash alone is an artifact of measuring with an alias, which bash
	// does not expand in a non-interactive shell. `false && v=$(if)` is the
	// pair: the three lazy columns never read the body at all.
	//
	// Not taken, and the reason is that it cannot be a flag read here: the
	// body would have to be parsed where the line is, with the alias table
	// and the dialect as they stood then, and the *tree* kept — a field on
	// the span beside syntax.Span.Arith and syntax.Span.Param, filled by the
	// parser. Checking the body at the top of each line and throwing the
	// tree away would answer the two corpus rows and still hand a live alias
	// table to the parse, which is one rule with two implementations. See
	// docs/spec/grammar/substitutions.md, and the corpus rows
	// `subst/a-body-that-will-not-parse-stops-the-line` and
	// `subst/a-body-that-will-not-parse-in-a-branch-never-taken`.
	p := r.ParseWithAliases(src, r.bodyDialect(span))
	// Where the body sits in the script, so that what it reports is reported
	// where a reader can find it. The span's own line is the body's first,
	// because a span starts at its opening delimiter — and it accumulates,
	// so a substitution inside a substitution is still placed in the file
	// rather than in whichever body most recently began. One dialect numbers
	// the older spelling from the top of the body instead, which is
	// Diagnostics.BackquotedSubstitutionRestartsLines.
	//
	// **Read once and used twice**, by the refusal below and by the runner
	// that runs what parsed. Two copies of this rule is how the refusal came
	// to place a body its own runner would have placed correctly.
	base := r.lineBase + int(span.Pos.Line) - 1
	if span.Backquoted && r.diag().BackquotedSubstitutionRestartsLines {
		base = 0
	}
	f := p.Parse()
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
		construct := ""
		if span.Backquoted && r.diag().SubstitutionParseFailureNamesTheConstruct {
			construct = "command substitution"
		}
		r.errf("%s", r.diagLineNamed(construct, "%s\n", r.diag().ParseFailure(shiftParseError(err, base))))
		if span.Backquoted && !r.ask(r.sem().SubstitutionParseErrorIsFatal, "a substitution body that does not parse ending the shell") {
			// The word expands to nothing and the statement goes on, which
			// is a failed *expansion* rather than a failed script. The
			// status is left alone for the same reason: the command that
			// holds the word is about to run and report its own.
			return ""
		}
		r.status = r.diag().SyntaxStatus()
		r.stopTheShell()
		return ""
	}

	if span.CurrentShell {
		// Here rather than on a copy, which is the whole of why this
		// spelling exists: `${ x=1;}` leaves x set where `$(x=1)` does not.
		//
		// The `<file` form below is not reached from here, and that is
		// measured rather than left out: `${ <f ;}` prints the file in ksh93
		// alone, is empty in bash 5.3 and is `bad substitution` in zsh 5.9.2
		// and bash 3.2. Four answers for one spelling is not this form.
		return r.currentShellSubst(ctx, f, span)
	}

	// `$(<file)` is the file, with nothing run. Asked after the parse because
	// the body is source until it is expanded, and before the subshell
	// because there is no command here for a subshell to hold.
	if r.dialect().ReadFileSubstitution {
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
	// And the third level of indirection, beside `eval` and a sourced file:
	// measured, `set -x; echo $(:)` traces the body at `++ ` in the one
	// dialect that counts them. See Runner.tracePrefixDepth.
	sub.indirection = r.indirection + 1
	sub.lineBase = base
	sub.Stdout = &out
	// The same group a subshell gets, and the same lifetime: the expansion
	// does not finish until the body has. See Runner.anchorForkedBody.
	defer sub.anchorForkedBody()()
	if _, err := sub.Run(ctx, f); err != nil {
		r.diagf("%v\n", err)
		return ""
	}
	// The status of a substitution is the status of what ran inside it, which
	// `x=$(false)` relies on.
	r.status = sub.status
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
func (r *Runner) currentShellSubst(ctx context.Context, f *syntax.File, span syntax.Span) string {
	var out bytes.Buffer
	savedOut, savedBase := r.Stdout, r.lineBase
	putBackReply := func() {}
	if span.ReplyValue {
		putBackReply = r.localizeReply()
	} else {
		r.Stdout = &out
	}
	// The body was parsed on its own, so its lines count from one; the
	// script it was written in did not. Same offset the subshell form
	// carries, and put back afterwards because this runner goes on being
	// used.
	r.lineBase = savedBase + int(span.Pos.Line) - 1
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.Stdout, r.lineBase = savedOut, savedBase
			putBackReply()
			r.diagf("%v\n", err)
			return ""
		}
		if r.ctl != controlNone {
			// `exit` or a `break` inside the body stops it, and carries on
			// stopping whatever it was written in — this is the current
			// shell, so there is no boundary here to absorb it.
			break
		}
	}
	r.Stdout, r.lineBase = savedOut, savedBase
	if span.ReplyValue {
		// Read before the name is put back, and taken whole: this is a
		// parameter's value rather than captured output, so the trailing
		// newlines the other form strips are text here. Measured,
		// `v=$'a\n\n'; echo "[${| REPLY=$v; }]"` keeps both.
		v, _ := r.getVar("REPLY")
		putBackReply()
		return v
	}
	return strings.TrimRight(out.String(), "\n")
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
// **What this is not is a scope.** bash gives the body a variable frame —
// `local` is legal inside one there and an error at the top level — and this
// engine gives neither form of the construct a frame at all, so `${ local z=1;
// echo $z; }` says `local: can only be used in a function` here. That is one
// gap and not two: the visible corner of it is that `${| unset REPLY; }` reads
// bash's *outer* REPLY, because unsetting a local there reveals what it shadows
// and there is nothing here for it to reveal. Recorded rather than worked
// around, so a frame — when the blank form gets one — fixes both spellings at
// once instead of finding a second answer already written here (#2656).
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
	return d
}

// dialect is the dialect this runner parses nested input with. It is a method
// rather than a field so the default is the core rather than the zero value,
// which would be posix and would refuse constructs the outer parse accepted.
func (r *Runner) dialect() syntax.Dialect {
	if r.Dialect != nil {
		return *r.Dialect
	}
	return syntax.Core()
}
