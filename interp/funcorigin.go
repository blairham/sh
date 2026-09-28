// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// funcOrigin is where a function's body came from.
//
// Two facts, in one record because they are learned at one moment and a
// definition route that recorded one without the other would be a route
// whose functions report the wrong place. The file half has been kept since
// #1706; the line half is #2565.
type funcOrigin struct {
	// file is where the definition was read, because that is the file the
	// body's frame reports rather than the file that called it. A function
	// declared in a sourced library and called from the script names the
	// library.
	file string
	// lineBase is the offset the text the body was read from was running
	// at, which the body goes on being numbered from however it is called
	// later.
	//
	// Borrowed text is the only way it is not zero. `eval`'s text is
	// numbered from the caller's lines in two dialects — see
	// Semantics.EvalTextContinuesTheCallersLines — and a function *defined*
	// in that text is read at that offset and called after the text has
	// been left, so the offset has to travel with the definition rather
	// than with the run. Measured 2026-09-12 over a script file whose line
	// 2 is `eval 'f() { nosuchcmd-xyz; }; f'`: bash 5.3 says `line 2` and
	// this engine said `line 1`, because Runner.callFuncAs reset the offset
	// to nothing and the body ran at the script's own numbering (#2565).
	//
	// It is not an axis of its own. Where the dialect numbers `eval`'s text
	// from one, nothing is in force when the definition is read and nothing
	// is recorded, so the two readings coincide by construction rather than
	// by a second switch that could disagree with the first.
	lineBase int
	// wrapperLines is how many lines a definition *seam* put in front of the
	// body before parsing it, and nought for a definition the parser read
	// out of the program.
	//
	// [Runner.defineFromText] is handed a body without its braces — what a
	// file on the function search path holds — and reads it by wrapping it
	// in a declaration of its own. That wrapper is a real line to the
	// parser, so every position inside the body is one greater than the line
	// the file actually has, and an *absolute* reader of those positions is
	// one too high: `$funcsourcetrace` reported the declaration a line below
	// itself, and the call site inside such a file reported a line past the
	// one that made the call. Measured 2026-09-25 against zsh 5.9.2, an
	// `autoload`ed file whose declaration is on line 2 reading `:3` here and
	// `:2` there (#4471).
	//
	// It stood because every other reader consumes the line as a
	// *subtrahend* — `$LINENO` in a body and a diagnostic's `name:N:` are
	// both `at - funcLine`, and both carry the same extra line, so the
	// wrapper cancels itself. Which is why this is subtracted from the
	// definition line and from the body's offset **together**: move one
	// without the other and the readings that were right stop being.
	wrapperLines int
	// text is the source the definition was read out of, where that was not
	// the script's own — see runningText. Unset for a function the script
	// defined, which is quoted against the script's text wherever it is
	// called and so needs no record.
	text runningText
}

// originHere is where a definition read at this moment came from: the file the
// shell is in, the offset the text it is reading was running at, and that text
// itself.
//
// Two callers, and they are the two ways a body comes to exist. A declaration
// records this and the table answers with it whenever the name is called
// later. A **nameless** function has no name to record under and no later:
// it is defined and run in one place, so its call asks this directly and
// there is nothing to remember. Spelled once because the two answers have to
// be the same answer — a nameless function's frame reports the file its
// declaration would have reported had it been given a name.
func (r *Runner) originHere() funcOrigin {
	return funcOrigin{file: r.currentFile(), lineBase: r.lineBase, text: r.runText}
}

// recordFunctionOrigin is the one write to the table, so that a new way of
// defining a function cannot quietly skip it.
//
// An origin with nothing in it is deleted rather than stored, so a lookup
// for a name nobody recorded and a lookup for a name recorded as coming
// from nowhere answer the same.
func (r *Runner) recordFunctionOrigin(name string, o funcOrigin) {
	if o == (funcOrigin{}) {
		delete(r.funcOrigins, name)
		return
	}
	if r.funcOrigins == nil {
		r.funcOrigins = map[string]funcOrigin{}
	}
	r.funcOrigins[name] = o
}

// definitionLine is the line a declaration at this parsed position sits on in
// the file the body was read from: the offset the text was running at, less
// whatever a seam's wrapper added in front of it.
//
// The two corrections are one question — *where is this really* — and they
// arrive by different routes, so they are added here rather than at each of
// the two callers. A function defined inside an autoloaded body needs both at
// once: its own record carries the body's offset and no wrapper, while the
// body around it carries the wrapper and no offset.
func (o funcOrigin) definitionLine(parsed int) int {
	return parsed + o.lineBase - o.wrapperLines
}

// bodyLineBase is the offset a body read at this origin runs under, which
// moves with the definition line so that the difference between them — every
// relative reading in the shell — is what it was.
func (o funcOrigin) bodyLineBase() int { return o.lineBase - o.wrapperLines }

// functionFile is where a function was defined, or the empty string for one
// whose definition route had no file to name.
func (r *Runner) functionFile(name string) string {
	return r.funcOrigins[name].file
}

// AtFunctionDefinition installs an observer this shell runs whenever a
// function is defined, with the name that was just given a body.
//
// Beside AtEveryFunctionCall and for the same kind of reason: the *moment* is
// the core's and what to do with it is the dialect's. One shell in the panel
// marks every function defined inside an `emulate … -c` so that the emulation
// is re-entered each time that function is later called, and the mark can
// only be taken at the definition — a name written into the table and a name
// redefined outside the emulation are the same event to this hook and
// opposite answers to the dialect reading it.
//
// It is told about **every** definition and not only the ones a dialect would
// mark, which is what makes taking a mark *off* possible: measured on zsh
// 5.9.2, a function made sticky by `emulate -R sh -c` and then redefined at
// the top level stops being sticky, and a hook that only fired inside an
// emulation could not have seen that happen.
//
// The runner is a parameter rather than something the closure caught, for
// AtEveryFunctionCall's reason: a subshell is a clone, so an observer writing
// through a captured pointer would mark the shell it was registered in.
func (r *Runner) AtFunctionDefinition(f func(r *Runner, name string)) {
	r.atFunctionDefinition = append(r.atFunctionDefinition, f)
}

// functionDefined tells the observers. Called from the definition routes and
// nowhere else — a route that skipped it would leave a mark standing over a
// body it is not about.
func (r *Runner) functionDefined(name string) {
	for _, f := range r.atFunctionDefinition {
		f(r, name)
	}
}
