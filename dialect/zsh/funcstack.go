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

// traceEntries walks the stack once and writes one element per unit
// `$funcstack` names, so the three arrays that report on those units are the
// same length by construction rather than by three walks that agree today.
//
// `${#funcstack}` == `${#functrace}` == `${#funcfiletrace}` ==
// `${#funcsourcetrace}` is measured on zsh 5.9.2 in every shape the tests
// here cover, and it is the invariant that makes the set usable: a handler
// reads `$funcstack[1]` beside `$functrace[1]` and `$funcfiletrace[1]`, so an
// array that selected a different set of frames would line one up against the
// wrong other. Spelling the selection once is what the callers differ *after*
// — each says only how to write the element, never which frames there are.
func traceEntries(r *interp.Runner, element func(frames []interp.Frame, i int) string) []string {
	frames := r.CallStack()
	out := make([]string, 0, len(frames))
	for i, f := range frames {
		if !namesAUnit(f) {
			continue
		}
		out = append(out, element(frames, i))
	}
	return out
}

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
	return traceEntries(r, func(frames []interp.Frame, i int) string {
		if enteredFromAFunctionBody(frames, i) {
			f := frames[i]
			return f.OuterFunc + ":" + strconv.Itoa(f.Line-f.OuterFuncLine)
		}
		return callSiteFileAndLine(r, frames, i)
	})
}

// funcfiletraceEntries is `$funcfiletrace`: the caller's **file and absolute
// line**, for every unit `$funcstack` names, innermost first (#4470).
//
// It is `$functrace` with the branch taken out — the same walk over the same
// frames, written the one way in every case, where `$functrace` switches to
// `<function>:<offset into its body>` whenever the call was made inside a
// function body. That is the whole of the difference and it is why the
// parameter exists: a handler that wants a *location* cannot recover one from
// a name and an offset.
//
// Measured 2026-09-25 against zsh 5.9.2 under `-f`, `-c` with `f` defined on
// line 1 calling `g` on line 2 and `g` defined on line 4, called on line 5:
//
//	frame  functrace                   funcfiletrace
//	g      f:1                         zsh:2
//	f      /opt/homebrew/bin/zsh:5     /opt/homebrew/bin/zsh:5
//
// The two names the shell gives itself in that one run are not a typo and
// need no symlink to see: the *defining file* of a function written in a `-c`
// string is the fixed `zsh`, while the *call site* at the top level of `-c` is
// `$0`. unitFile is what keeps them apart — see enteredFromFile, which
// measured the same split for `$functrace`.
//
// Empty at the top level, and short by the same `eval` frame, for the reasons
// functraceEntries gives.
func funcfiletraceEntries(r *interp.Runner) []string {
	return traceEntries(r, func(frames []interp.Frame, i int) string {
		return callSiteFileAndLine(r, frames, i)
	})
}

// funcsourcetraceEntries is `$funcsourcetrace`: where the unit running in each
// frame was **defined**, `file:line`, innermost first (#4469).
//
// The third question about the same frames, and the only one of the three
// that asks nothing about the caller: `$funcstack` says what the shell is in,
// `$functrace` and `$funcfiletrace` say where each of those was entered from,
// and this says where each was written. It is zsh's `${BASH_SOURCE[0]}` — the
// "find the file I was loaded from" idiom — which is why tig's shipped zsh
// completion has no other way to reach its own directory.
//
// Measured 2026-09-25 against zsh 5.9.2 under `-f`, and two details are only
// visible if you go looking:
//
//   - **A sourced file's frame is `<file>:0`.** A file has no definition line
//     and zsh writes a literal nought — not 1, and not the line it was
//     sourced at. `. ./src.zsh` reports `./src.zsh:0` for that frame from
//     every depth, including a file sourced from inside another sourced file.
//     Nothing here writes that nought: [interp.Frame.FuncLine] is zero on a
//     frame that is not a function, which is the same answer arrived at
//     rather than a case.
//   - **The line is the line the declaration starts on**, not the brace's and
//     not the body's first. `f()` on line 2 with its `{` on line 3 is `:2`,
//     `function k {` on line 6 is `:6`, and a body of blank lines does not
//     move it. That is [interp.Frame.FuncLine], which is `fn.Pos().Line` —
//     the same line `$LINENO` counts a function's offsets from, so the two
//     cannot disagree.
//
// The file is the frame's **own**, which is what makes this the odd one out:
// the other two read the frame below. A function defined in a sourced library
// and called from the script names the library here and the script there,
// measured both ways.
//
// One route is off by one line and is named rather than papered over: a
// function `autoload` defined reports its line one too high, because
// Runner.defineFromText reads a body out of text by wrapping it in a
// synthetic `name() {` line that the parser then counts. Every other route is
// exact — a script, a sourced file, a `-c` string, a definition nested inside
// another function, either declaration spelling — and the *sourced* reading of
// the very same file is byte-identical to the reference, which is what says
// the fault is the wrapper and not this walk. It stood because everything else
// consumes the line as a subtrahend, `at - funcLine`, where the extra line
// cancels itself. See #4471, which owns the fix; this array is its first
// reader and not its cause.
func funcsourcetraceEntries(r *interp.Runner) []string {
	return traceEntries(r, func(frames []interp.Frame, i int) string {
		f := frames[i]
		return unitFile(r, f) + ":" + strconv.Itoa(f.FuncLine)
	})
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

// callSiteFileAndLine is where a frame was entered from, written as a file and
// an absolute line in it.
//
// The whole of `$funcfiletrace`'s element, and the branch `$functrace` takes
// for a call not made inside a function body — spelled once, because the two
// arrays agree on every element `$functrace` writes this way and a second
// copy would be free to stop agreeing.
func callSiteFileAndLine(r *interp.Runner, frames []interp.Frame, i int) string {
	return enteredFromFile(r, frames, i) + ":" + strconv.Itoa(frames[i].Line)
}

// enteredFromFile is the file a call was made in.
//
// The frame below is the one that was reading it, whatever kind of frame that
// is — a sourced file, a startup file, the script itself — and its File is
// what a diagnostic raised there would name. Read through unitFile, because a
// function defined at the top level of `-c` has no file and the core parks
// `$0` in the field; this shell calls that non-file `zsh`, measured.
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
// binary under the same symlink. `$funcfiletrace` splits identically, measured
// under the same symlink, so this is the shell's answer for "where am I" on
// the `-c` route rather than anything about either parameter.
func enteredFromFile(r *interp.Runner, frames []interp.Frame, i int) string {
	if i+1 < len(frames) {
		return unitFile(r, frames[i+1])
	}
	if !frames[i].IsFunction() {
		return shellSelfName(r)
	}
	return r.Name
}

// unitFile is the file a frame's unit was read out of, as this shell names it.
//
// [interp.Frame.File] is the answer except for a unit that came out of no file
// — a function defined at the top level of `-c` or of standard input — where
// the core parks the shell's `$0` in the field, that being the measured answer
// for the dialect whose `${BASH_SOURCE[@]}` reports it. This shell names that
// same non-file after *itself*, and the two are different strings in one
// ordinary run: `zsh -f -c 'f(){<newline>  g<newline>}<newline>g(){ … }
// <newline>f'` answers `$funcfiletrace` as `zsh:2` for the inner frame and
// `/opt/homebrew/bin/zsh:5` for the outer, from the same binary invoked by its
// own path. See [interp.Frame.NoFile].
func unitFile(r *interp.Runner, f interp.Frame) string {
	if f.NoFile {
		return shellSelfName(r)
	}
	return f.File
}

// shellSelfName is what this shell calls itself where there is no file to
// name, which is not `$0`: a diagnostic on the `-c` route says `zsh:1:
// command not found` from a binary reached as `./myzsh`. Spelled once because
// three readers ask it and `$0` is the wrong answer for all three.
func shellSelfName(r *interp.Runner) string {
	if r.Diagnostics != nil && r.Diagnostics.SelfName != "" {
		return r.Diagnostics.SelfName
	}
	return r.Name
}
