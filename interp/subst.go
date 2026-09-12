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
	p := r.ParseWithAliases(src, r.dialect())
	f := p.Parse()
	if err := p.Err(); err != nil {
		// A substitution re-parses, so the syntax-error status is the
		// dialect's here too — not only in whatever first read the script.
		//
		// And it is fatal: dash, bash, ksh93 and zsh all abandon the script
		// rather than continue with an empty substitution. They detect it
		// when they parse the whole input; this parses the body at expansion
		// time, so the same outcome has to be produced deliberately. Without
		// it the diagnostic appeared and the next command ran regardless,
		// which is the shape this package keeps finding.
		r.diagf("%v\n", err)
		r.status = r.diag().SyntaxStatus()
		r.ctl = controlExit
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
	sub.inheritJobs(jobBoundarySubstitution)
	sub.inCommandSubst = true
	// And the third level of indirection, beside `eval` and a sourced file:
	// measured, `set -x; echo $(:)` traces the body at `++ ` in the one
	// dialect that counts them. See Runner.tracePrefixDepth.
	sub.indirection = r.indirection + 1
	// Where the body sits in the script, so that what it reports is reported
	// where a reader can find it. The span's own line is the body's first,
	// because a span starts at its opening delimiter — and it accumulates,
	// so a substitution inside a substitution is still placed in the file
	// rather than in whichever body most recently began.
	sub.lineBase = r.lineBase + int(span.Pos.Line) - 1
	if span.Backquoted && r.diag().BackquotedSubstitutionRestartsLines {
		sub.lineBase = 0
	}
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

// currentShellSubst runs a `${ … ;}` body on this runner.
//
// Everything it does outlives it, so there is nothing to clone and nothing to
// merge back — only the output to catch and the writer to put back
// afterwards. The status is the body's last command's for the same reason: it
// is this runner's status, set where every other command sets it.
func (r *Runner) currentShellSubst(ctx context.Context, f *syntax.File, span syntax.Span) string {
	var out bytes.Buffer
	savedOut, savedBase := r.Stdout, r.lineBase
	r.Stdout = &out
	// The body was parsed on its own, so its lines count from one; the
	// script it was written in did not. Same offset the subshell form
	// carries, and put back afterwards because this runner goes on being
	// used.
	r.lineBase = savedBase + int(span.Pos.Line) - 1
	for _, st := range f.Stmts {
		if err := r.stmt(ctx, st); err != nil {
			r.Stdout, r.lineBase = savedOut, savedBase
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
	return strings.TrimRight(out.String(), "\n")
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
