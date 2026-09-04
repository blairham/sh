// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a shell is, in the sense a script means when it asks.
//
// `${BASH_SOURCE[0]}` is how a script finds its own directory, and it is one
// of the first things a great many of them do — which is why the answer has
// to be produced rather than stored. It changes with every function call and
// every sourced file, and a plain array set once would be right until the
// first `source` and silently wrong afterwards, which is the worst shape a
// bug can have here.
//
// The core keeps the stack and names none of it. Three shells expose it under
// three different sets of names, and a fourth exposes nothing at all, so
// naming is the dialect's the way every other wording is.

// Frame is one entry of the call stack.
type Frame struct {
	// File is the file being read in this frame: the one a function was
	// *defined* in, or the one that was sourced.
	File string

	// Name is the function's name, or "source" for a sourced file. Empty for
	// the script itself.
	Name string

	// Line is the line this frame was entered from, in the frame below it.
	Line int

	// serial numbers this frame among every frame ever pushed, so a
	// sibling entered later at the same depth is still a different frame —
	// which is the distinction the RETURN trap turns on.
	serial int
}

// CallStack is the frames a shell is currently inside, innermost first, with
// the script itself last.
//
// The script's own frame is there whenever there is a script, even at the top
// level with nothing called: a script that asks where it is expects an answer
// before it has done anything. A shell given `-c` or standard input has no
// such frame, because there is no file — and that is measured rather than
// assumed: bash inside a function called from `-c` reports one frame and not
// two, and reports nothing at all outside one.
func (r *Runner) CallStack() []Frame {
	out := make([]Frame, 0, len(r.frames)+1)
	for i := len(r.frames) - 1; i >= 0; i-- {
		out = append(out, r.frames[i])
	}
	if r.scriptFile != "" {
		out = append(out, Frame{File: r.scriptFile})
	}
	return out
}

// HasScriptFrame reports whether the bottom of the stack is a script.
//
// The dialect that names the bottom frame `main` names it only when it is
// there, so this is the question that decides rather than the file's text.
func (r *Runner) HasScriptFrame() bool { return r.scriptFile != "" }

// InCall reports whether anything has been called: a function entered or a
// file sourced.
//
// One dialect's name for the innermost function is absent at the top level
// rather than empty, which is a different thing to a script testing it.
func (r *Runner) InCall() bool { return len(r.frames) > 0 }

// SetScriptFile says which file the shell was given, for the bottom of the
// stack. The front end knows and the interpreter does not.
func (r *Runner) SetScriptFile(path string) { r.scriptFile = path }

// pushFrame enters a function or a sourced file.
func (r *Runner) pushFrame(f Frame) {
	f.Line = r.line
	r.frameSerial++
	f.serial = r.frameSerial
	if f.File == "" {
		// A function defined where nothing was read from a file — `-c`, or
		// standard input — belongs to whatever the shell calls itself, which
		// is what bash puts there: `bash -c 'f(){ …; }; f'` reports the
		// frame's file as `bash`.
		f.File = r.Name
	}
	r.frames = append(r.frames, f)
}

// popFrame leaves one.
func (r *Runner) popFrame() {
	if len(r.frames) > 0 {
		r.frames = r.frames[:len(r.frames)-1]
	}
}

// currentFile is the file being read now, which is what a function defined
// here will remember.
func (r *Runner) currentFile() string {
	if len(r.frames) > 0 {
		return r.frames[len(r.frames)-1].File
	}
	return r.scriptFile
}
