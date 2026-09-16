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
	// text is the source the definition was read out of, where that was not
	// the script's own — see runningText. Unset for a function the script
	// defined, which is quoted against the script's text wherever it is
	// called and so needs no record.
	text runningText
}

// recordFunctionOrigin is the one write to the table, so that a new way of
// defining a function cannot quietly skip it.
//
// An origin with nothing in it is deleted rather than stored, so a lookup
// for a name nobody recorded and a lookup for a name recorded as coming
// from nowhere answer the same.
func (r *Runner) recordFunctionOrigin(name, file string, lineBase int, text runningText) {
	if file == "" && lineBase == 0 && !text.borrowed {
		delete(r.funcOrigins, name)
		return
	}
	if r.funcOrigins == nil {
		r.funcOrigins = map[string]funcOrigin{}
	}
	r.funcOrigins[name] = funcOrigin{file: file, lineBase: lineBase, text: text}
}

// functionFile is where a function was defined, or the empty string for one
// whose definition route had no file to name.
func (r *Runner) functionFile(name string) string {
	return r.funcOrigins[name].file
}
