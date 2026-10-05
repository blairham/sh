// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// What the shell is currently *inside*, as a stack.
//
// Not the call stack. [Runner.CallStack] holds the units a `return` and a
// `$0` are about — a function and a sourced file — and it deliberately knows
// nothing about an `eval`, a command substitution or a trap action, because
// none of those is a unit anything else asks about. One dialect publishes a
// parameter that *is* about all of them, so the shell has to keep the wider
// record; see the dialect's own file for the words it writes and for what a
// script reads them for.
//
// The core keeps the stack and a dialect names its entries, which is the same
// seam [Runner.SetFunctionNesting] and [Runner.SetRegexMatch] use: what is
// here is a set of shapes a shell enters, and nothing in it is a word.

// EvalContext is one shape the shell can be inside.
//
// The set is what a sweep of the shell being modeled distinguishes, measured
// 2026-09-27 — see the dialect for the table. The ones it does **not**
// distinguish are as much of the answer: a subshell `( … )`, a brace group, a
// loop body, an `always` block and an arithmetic expansion all leave the
// stack exactly as they found it, so none of them is a value here.
type EvalContext uint8

const (
	// EvalContextScript is the program the shell was given as a file, or on
	// standard input. The bottom of the stack on those two routes, and never
	// pushed: it is the shell running its own program.
	EvalContextScript EvalContext = iota
	// EvalContextCommandString is that same bottom on the `-c` route.
	EvalContextCommandString
	// EvalContextSourcedFile is a file `.` or `source` is reading.
	EvalContextSourcedFile
	// EvalContextFunctionBody is a function body, an anonymous function
	// included — measured, `() { … }` pushes exactly what a named call does.
	EvalContextFunctionBody
	// EvalContextEval is text `eval` is running.
	EvalContextEval
	// EvalContextTrap is a trap action.
	EvalContextTrap
	// EvalContextCommandSubstitution is `$( … )`, and the backquoted
	// spelling with it — measured, the two answer identically.
	EvalContextCommandSubstitution
	// EvalContextProcessSubstitutionRead is `<( … )`: the body whose output
	// the word names a path to read.
	EvalContextProcessSubstitutionRead
	// EvalContextProcessSubstitutionWrite is `>( … )`, the other direction.
	EvalContextProcessSubstitutionWrite
	// EvalContextTempFileSubstitution is `=( … )`, the spelling with a file
	// where the other two have a pipe.
	EvalContextTempFileSubstitution
	// EvalContextAutoloadedBody is a function's body on the call that loaded
	// it: the first call of a name declared for loading, which reads the
	// function's file and runs what it defined, and no call after that. One
	// dialect publishes it, and a function file reads it to tell the call
	// that loaded it apart from being sourced or run as a script.
	EvalContextAutoloadedBody
	// EvalContextAutoloadedFile is a function's file being run as a script
	// so that it can define the function, which is what a ksh-style load
	// does before it runs the definition the file left.
	EvalContextAutoloadedFile
	// EvalContextStartupFile is a startup file the shell is reading before
	// its program, and it stands *in place of* the route's bottom rather than
	// on top of it: the program has not started, so there is no script or
	// command string to be inside yet. Measured 2026-10-05 on zsh 5.9.2, the
	// one shell with the parameter, reading it in each of `.zshenv`,
	// `.zprofile`, `.zshrc` and `.zlogin` under `-l -i -c` and in `.zshenv`
	// ahead of a script file:
	//
	//	at the top of the file        file
	//	in a function it calls        file shfunc
	//	in $( … ) in it               file cmdsubst
	//	in eval in it                 file eval
	//	in a file it sources          file file
	//	the program after it          cmdarg, or toplevel
	//
	// where this shell said `cmdarg` or `toplevel` in the file's place on
	// every row (#5885).
	EvalContextStartupFile
)

// EvalContextStack is the stack outermost first, with the route's own entry
// at the bottom.
//
// The bottom is computed rather than pushed, because it is a fact about the
// invocation and not about anything the shell entered — the same reason
// [Route] is carried in rather than decided here. A Runner whose route was
// never said reads as a script, which is what an embedder driving statements
// directly is doing.
func (r *Runner) EvalContextStack() []EvalContext {
	if len(r.evalContexts) > 0 && r.evalContexts[0] == EvalContextStartupFile {
		// Read before the program, so the file is the bottom and there is no
		// route's entry under it. See EvalContextStartupFile.
		return slices.Clone(r.evalContexts)
	}
	out := make([]EvalContext, 0, len(r.evalContexts)+1)
	bottom := EvalContextScript
	if r.Route == RouteCommandString {
		bottom = EvalContextCommandString
	}
	out = append(out, bottom)
	return append(out, r.evalContexts...)
}

// enterEvalContext pushes one and hands back the leave.
//
// Written `defer r.enterEvalContext(c)()` at every site, which is the shape
// that cannot be unwound in the wrong order — and the order matters here in
// a way it does not for a table: a missed pop leaves every later read of the
// parameter wrong rather than one.
func (r *Runner) enterEvalContext(c EvalContext) func() {
	r.evalContexts = append(r.evalContexts, c)
	depth := len(r.evalContexts)
	return func() {
		if len(r.evalContexts) >= depth {
			r.evalContexts = r.evalContexts[:depth-1]
		}
	}
}

// EnterEvalContext is enterEvalContext for a dialect, whose construct the
// core cannot see: loading a function is a dialect's builtin running a body
// the core was handed, so it is the dialect that knows when that body is the
// one the load is running. Written `defer r.EnterEvalContext(c)()` for the
// reason enterEvalContext gives.
func (r *Runner) EnterEvalContext(c EvalContext) func() { return r.enterEvalContext(c) }

// CallNextBodyAs puts c over the body of the next function call to open a
// frame, inside that frame, and hands back what takes the request back if no
// call took it. Written `defer r.CallNextBodyAs(c)()` around the code that
// makes the call.
//
// For a dialect whose load runs the body through a route it does not own the
// frame of. zsh's hand-written `autoload -X` is the case: measured 2026-10-04
// on zsh 5.9.2, the body it loads runs under `eval` and then its own frame,
// and the word for a load stands over that frame —
// `cmdarg shfunc eval shfunc loadautofunc` — so it cannot be pushed before the
// call the way EnterEvalContext would push it (#5897).
func (r *Runner) CallNextBodyAs(c EvalContext) func() {
	r.nextBodyContext, r.nextBodyContextSet = c, true
	return func() { r.nextBodyContextSet = false }
}

// pushEvalContext is enterEvalContext for a context that ends with the runner
// it is pushed on: a substitution's body runs in a clone, so the entry goes
// on that clone and nothing has to take it off again.
func (r *Runner) pushEvalContext(c EvalContext) {
	r.evalContexts = append(r.evalContexts, c)
}
