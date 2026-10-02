// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// One dialect lists the text handed to `eval` among the units the shell is
// inside — `$funcstack` names it `(eval)` — and reports where each unit was
// entered from both as a place in the text around it and as a line of the
// file. Measured 2026-10-02 on zsh 5.9.2 (`-f`), from a script file:
//
//	eval on line 7, `f` called on its 3rd line   f's caller: (eval):3, file line 9
//	eval inside k, whose line 8 holds it          the eval's caller: k:1, file line 8
//	eval inside eval, both on line 3              (eval) (eval), both file line 3
//	q defined on the 2nd line of an eval on 13    q is defined at file line 14
//	unsetopt evallineno                           no (eval) entry at all
//
// The entry is not a frame, and that is deliberate: nothing else in this shell
// counts `eval` as a call — not `$0`, not a frame selection, not another
// dialect's `FUNCNAME` — so it is kept beside the stack and woven into one
// view of it rather than pushed onto it.

// evalUnit is one entry: the frame it stands for, and how many frames stood
// when it was entered, which is where in the stack it goes.
type evalUnit struct {
	depth int
	frame Frame
}

// enterEvalUnit records that the shell is entering text handed to `eval` that
// is a place of its own, and returns what leaves it. It reads the caller's
// position, so it is called before anything moves the location into the text.
func (r *Runner) enterEvalUnit() func() {
	f := Frame{
		Name: "(eval)", Eval: true,
		Line: r.line, AbsLine: r.fileLineOffset + r.line,
		OuterFunc: r.inFunc, OuterFuncLine: r.funcLine,
	}
	f.FuncLine, f.FuncAbsLine = f.Line, f.AbsLine
	// The text's file is the one the caller is reading, including a stand-in
	// for no file at all; at the top level of a shell given no file, it is
	// `$0`. Measured: `zsh -f -c 'p(){…}<newline>eval p'` reports the eval's
	// own entry as `$0:2`, while an eval inside a function defined at the top
	// level of `-c` reports the shell's fixed name, as that function does.
	switch {
	case len(r.frames) > 0:
		top := r.frames[len(r.frames)-1]
		f.File, f.NoFile = top.File, top.NoFile
	case r.scriptFile != "":
		f.File = r.scriptFile
	default:
		f.File = r.Name
	}
	r.evalUnits = append(r.evalUnits, evalUnit{depth: len(r.frames), frame: f})
	return func() { r.evalUnits = r.evalUnits[:len(r.evalUnits)-1] }
}

// CallStackWithEvals is CallStack with an entry for every text handed to
// `eval` that the shell is inside, each standing where it was entered:
// innermost first, the script last.
func (r *Runner) CallStackWithEvals() []Frame {
	if len(r.evalUnits) == 0 {
		return r.CallStack()
	}
	out := make([]Frame, 0, len(r.frames)+len(r.evalUnits)+1)
	e := len(r.evalUnits) - 1
	for i := len(r.frames); i >= 0; i-- {
		for e >= 0 && r.evalUnits[e].depth == i {
			out = append(out, r.evalUnits[e].frame)
			e--
		}
		if i > 0 {
			out = append(out, r.frames[i-1])
		}
	}
	if r.scriptFile != "" {
		out = append(out, Frame{File: r.scriptFile})
	}
	return out
}
