// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// funcstackNames is `$funcstack`: the units the shell is currently inside,
// innermost first — a function by its name, a sourced file by the word
// `source` was given.
//
// Two things about it are not [dialect/bash]'s `FUNCNAME`, and both were
// measured rather than carried across.
//
// **There is no frame for the script itself.** bash ends `FUNCNAME` with
// `main` at a script's top level; zsh 5.9.2 answers `$#funcstack` as **0**
// there, and 1 — not 2 — inside a function the script called. So the frame
// [interp.Runner.CallStack] appends for the script is dropped here, and a
// translation of `FUNCNAME` that kept its last element would be off by one at
// every depth. It is recognized by carrying neither a name nor an operand,
// which is what that frame is: the two fields a called unit is named by are
// both a call's, and the script was not called.
//
// **A sourced file appears under the path it was found at**, which is the
// file and not the word. `. ./inc.zsh` reports `./inc.zsh`, and that probe
// cannot tell the two apart, because a slash in the operand skips the search
// and the two strings are equal. `PATH=lib; . inc.zsh` is the one that
// discriminates, and zsh 5.9.2 answers with the full `…/lib/inc.zsh` there
// while the operand stays `inc.zsh`. So this reads [interp.Frame.File].
//
// [interp.Frame.Operand] is still what says *which kind* a frame is — it is
// non-empty only for a sourced file, where `File` alone could not tell a
// sourced file from the function that was defined in it, since a function's
// frame carries its defining file under the same name.
//
// The rest agrees with `FUNCNAME` exactly: `[inner][outer]` for a nesting,
// `[f]` for `zsh -c 'f(){ … }; f'`, innermost first in every case.
//
// One gap is deliberate and named here rather than papered over: real zsh
// pushes an `(eval)` frame, so `eval` inside a function `f` reports
// `[(eval)][f]` where this answers `[f]`. `eval` does not enter a frame in
// this interpreter at all — it is not a unit anything else asks about either,
// so inventing an entry here would make this parameter the only thing in the
// shell that believes in a frame nothing pushed. See #1598.
func funcstackNames(r *interp.Runner) []string {
	frames := r.CallStack()
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		switch {
		case f.Operand != "":
			out = append(out, f.File)
		case f.Name != "":
			out = append(out, f.Name)
		}
	}
	return out
}

// namesAUnit reports whether this frame is one `$funcstack` names — a called
// function or a sourced file, and not the script itself or a startup file the
// shell read of its own accord.
//
// Spelled once because `$functrace` is the *same list*, said the other way
// round: element i there is where element i of `$funcstack` was entered from,
// so the two arrays have to select the same frames or they stop lining up.
// Measured on zsh 5.9.2 in every shape below — `${#funcstack}` and
// `${#functrace}` are equal in all of them.
func namesAUnit(f interp.Frame) bool { return f.Operand != "" || f.Name != "" }

// functraceEntries is `$functrace`: for each unit `$funcstack` names, where it
// was entered from, written `place:line` and innermost first.
//
// Measured 2026-09-25 against zsh 5.9.2 under `-f`, and the shape is not the
// one the name suggests. It is **not** a file and a line — that is
// `$funcfiletrace` — it is *the caller*, named the way that caller would be
// named in a diagnostic:
//
//   - **A call made from inside a function body** is the function's own name
//     and the offset into it, counting from the line the definition was
//     written on. `f()` on line 2 calling `g` on line 4 is `f:2`, and a call
//     on the definition's own line is `f:0`. Both spellings of a definition
//     count from the same place: `function k {` on line 6 calling on line 7
//     is `k:1`, and a body whose `{` is on its own line does not shift it.
//   - **A call made from a file** — the top level of a script, a sourced
//     file, a startup file — is that file and the absolute line in it.
//     `t1.zsh:10`, `./src.zsh:3`.
//   - **A call made from the top level of `-c` or of standard input** is the
//     shell's own name, which is `$0`: `/opt/homebrew/bin/zsh:1`.
//
// This is the same counting `$LINENO` and a diagnostic's location already do
// here — see Runner.locationNameAndLine — which is the point of deriving it
// from the stack rather than keeping a second one: the offset a debug trap
// prints and the offset a refusal carries are one fact.
//
// The file the second case names is the file of the frame **below** this one
// and never this frame's own. [interp.Frame.File] is where a function was
// *defined*, and the two differ exactly when a function is called from
// somewhere other than the file it came from — measured, `zsh -f -c '.
// ./lib.zsh<newline>f'` reports the shell and not `./lib.zsh`, so reusing
// File here would have named the definition for every sourced library in a
// script.
//
// Top level is **empty**, not a one-element array: `${#functrace}` is 0 with
// nothing called, the same place `${#funcstack}` is 0. So is the `eval` gap
// funcstackNames records above — real zsh pushes an `(eval)` frame and this
// shell has none — and it shows here as one *missing element* rather than as
// a wrong one, which is the same deliberate difference and not a second.
func functraceEntries(r *interp.Runner) []string {
	frames := r.CallStack()
	out := make([]string, 0, len(frames))
	for i, f := range frames {
		if !namesAUnit(f) {
			continue
		}
		if enteredFromAFunctionBody(frames, i) {
			out = append(out, f.OuterFunc+":"+strconv.Itoa(f.Line-f.OuterFuncLine))
			continue
		}
		out = append(out, enteredFromFile(r, frames, i)+":"+strconv.Itoa(f.Line))
	}
	return out
}

// enteredFromAFunctionBody reports whether this frame was entered from a line
// a function body holds, which is what decides between the two ways a call
// site is written.
//
// The **frame below** answers, not [interp.Frame.OuterFunc] on its own, and
// that is the same seam Runner.locationIsInsideAFunctionBody works on
// (#2037): a function that sources a file is still the innermost *function*
// while the file runs, so a call the file makes carries the function's name
// in OuterFunc while being nowhere inside its body. Measured on zsh 5.9.2 —
// `w(){ . ./src.zsh; }` with `src.zsh` calling `h` on its own line 4 answers
// `./src.zsh:4` for `h`, and OuterFunc alone wrote `w:3`, an offset into a
// body three lines long.
//
// So OuterFunc says *which* function and the frame below says *whether*,
// which is why both are read.
func enteredFromAFunctionBody(frames []interp.Frame, i int) bool {
	return frames[i].OuterFunc != "" && i+1 < len(frames) && frames[i+1].IsFunction()
}

// enteredFromFile is the file a call was made in, for a call not made from
// inside a function body.
//
// The frame below is the one that was reading it, whatever kind of frame that
// is — a sourced file, a startup file, the script itself — and its File is
// what a diagnostic raised there would name.
//
// With nothing below, the call was made at the top level of a shell given
// `-c` or standard input, where there is no file at all — and zsh names
// *itself* two different ways there depending on what was entered. Measured
// 2026-09-25 on zsh 5.9.2 through a symlink called `./myzsh`, so `$0` and the
// fixed name are different strings and the rows cannot be confused:
//
//	./myzsh -f -c 'f(){ … }<newline>f'        ./myzsh:2
//	./myzsh -f -c '<newline>. ./src.zsh'      zsh:2
//
// A **function** entered from there is located at `$0`, and a **sourced
// file** entered from there at the shell's own fixed name — which is the one
// a diagnostic uses on that route, `zsh:1: command not found` from the same
// binary under the same symlink. `$funcfiletrace` splits identically, so this
// is the shell's answer for "where am I" on the `-c` route rather than
// anything about this parameter.
func enteredFromFile(r *interp.Runner, frames []interp.Frame, i int) string {
	if i+1 < len(frames) {
		return frames[i+1].File
	}
	if !frames[i].IsFunction() && r.Diagnostics != nil && r.Diagnostics.SelfName != "" {
		return r.Diagnostics.SelfName
	}
	return r.Name
}
