// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"context"

	"github.com/blairham/sh/internal/panicguard"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A shell held open across several inputs, for a front end that is neither a
// script nor a terminal.
//
// The three routes MainArgs reads all end the shell when their input runs out,
// because that is what `-c`, a script file and a piped program mean. A prompt
// does not, and until now the only way to hold a shell open was repl — which
// needs a terminal, a line editor and a person. An agent protocol needs the
// first half and none of the second: a session whose variables, functions,
// working directory and traps survive from one input to the next, driven by
// something other than a keyboard.
//
// It is here rather than in the front end that wants it for the reason
// everything else about an invocation is here. A second implementation of
// "parse a line, run it, notice the dialect changed underneath you" is a
// second implementation of the front end, and the last time this repository
// had one of those the dialect binaries scored fourteen cases worse than the
// core for reasons that had nothing to do with any dialect.

// Session is a shell that stays open across several inputs.
//
// Each input is run the way `-c` runs a command string — the origin is
// labeled `-c` in a parse failure's location, the shell keeps its own name,
// and there are no positional parameters — but the runner is not rebuilt, so
// what one input set the next one sees.
//
// A Session is not safe for concurrent use. One shell runs one program at a
// time, and two inputs interleaved in one runner would give neither the
// variables it wrote; a front end that accepts a second request while one is
// running refuses it rather than serializing it, because a caller waiting for
// a turn that has not started is owed an answer rather than a delay.
type Session struct {
	sh Shell
	r  *interp.Runner
}

// NewSession opens one.
//
// The status is what the shell would have exited with had this been an
// invocation — a dialect prelude that would not run is the only way it fails —
// and it is 0 for a session that opened, which is the ordinary case. A nil
// Session always comes with a non-zero status.
func NewSession(sh Shell) (*Session, int) {
	sh = sh.withDefaults(nil)
	// The command-string route, because that is what each input will be. The
	// runner reads it for the status one dialect gives a failed expansion,
	// the fatality another gives a readonly reassignment, and the route
	// letters in `$-`.
	r := sh.newRunner(sh.Name, nil, sh.Diagnostics, interp.RouteCommandString)
	// Every input is a command string, so the route's answer is the same for
	// all of them and is settled here rather than per input.
	r.SetAliasExpansionBase(sh.Dialect.ExpandAliases.Has(syntax.RouteFromCommandString))
	if sh.Prelude != "" {
		if code := sh.source(r, sh.Name); code != 0 {
			return nil, code
		}
	}
	return &Session{sh: sh, r: r}, 0
}

// Run runs one input and reports the status the shell is left with, without
// ending the shell.
//
// ctx belongs to this input rather than to the session, which is what lets a
// front end interrupt one: a command started under it is killed when it is
// canceled, and what the shell has set stays set for the next input.
//
// Guarded per input, the way repl guards a typed line and for the same reason.
// A session is worth keeping across an interpreter bug — the alternative is
// that one malformed construct takes a connection and every other session on
// it — so a panic here becomes a status rather than a stack trace.
func (s *Session) Run(ctx context.Context, src string) (status int) {
	if s.sh.guard().Do(func() { status = s.run(ctx, src) }) {
		return panicguard.Status
	}
	return status
}

func (s *Session) run(ctx context.Context, src string) int {
	in := commandSource(s.sh, src, nil)
	if in.wholeFirst {
		// One dialect reads a command string whole before running any of it,
		// so the failure is reported before anything has run. Read from the
		// runner's dialect rather than the shell's, because a previous input
		// may have changed it: `set -o posix` in one turn governs the parse
		// of the next.
		p := syntax.NewParser(src, s.dialect().On(syntax.RouteFromCommandString))
		p.Parse()
		if err := p.Err(); err != nil {
			s.sh.sayRemarks(in.dg, in.name, p.Remarks(), 0, true)
			s.sh.errf("%s", in.dg.ParseDiagnostic(in.name, in.input, err, src))
			return in.dg.StatusForParseError(err)
		}
	}
	pr := wholeProgram(src, s.dialect().On(syntax.RouteFromCommandString))
	// Aliases are expanded when a line is parsed and the table is the
	// runner's, so joining the two is the front end's job here exactly as it
	// is for a script. An alias defined by one input is available to the next,
	// which is the rule every shell has for a line.
	//
	// Whether the shell expands at all is the runner's now and was decided
	// once, in NewSession — not per input, which would undo a `shopt -u
	// expand_aliases` the session ran earlier.
	pr.aliases = s.r.ExpandingAlias
	pr.globalAliases = s.r.ExpandingGlobalAlias
	pr.suffixAliases = s.r.ExpandingSuffixAlias
	status, how := s.sh.executeLines(ctx, s.r, pr, in)
	if how != endingRanOut {
		// A parse failure or a refusal has a status of its own, and neither
		// ends the session: the next input is a fresh line, not a fresh
		// shell. The EXIT trap is deliberately *not* run here — it fires once,
		// when the session closes, and firing it per input would run it as
		// many times as the client sends a prompt.
		return status
	}
	return s.r.ExitStatus()
}

// dialect is the parser configuration this session is on now, which is the
// runner's rather than the shell's: a dialect is runtime state and an input
// that changed it governs the ones after it.
func (s *Session) dialect() syntax.Dialect {
	if s.r.Dialect != nil {
		return *s.r.Dialect
	}
	return s.sh.Dialect
}

// Exited reports whether the shell was asked to stop — `exit` in an input, or
// a fatal signal. A front end that keeps feeding a session that has exited is
// feeding a shell that is not there.
func (s *Session) Exited() bool { return s.r.Exited() }

// Close ends the session and reports the status to exit with, running the EXIT
// trap once as the end of a script does.
func (s *Session) Close(ctx context.Context) int { return s.r.Finish(ctx) }

// Runner is the interpreter this session runs in.
//
// Exported for the front end that has to describe a session rather than drive
// it — the working directory a `cd` moved to, whether the shell has exited. It
// is not a license to run a program on it: that is Run's, and a second caller
// running one would be the concurrent use the type does not allow.
func (s *Session) Runner() *interp.Runner { return s.r }
