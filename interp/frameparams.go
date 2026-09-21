// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Reading the positional parameters and `$0` in the frame a script has
// *selected*.
//
// The third thing a `.sh.level` selection moves, after the two location
// parameters (interp/callstack.go) and the variable scope
// (interp/framescope.go): each level answers with **that frame's** argument
// list, and `$0` names the function standing there.
//
// Measured on AT&T ksh93u+ 2012-08-01 (`/bin/ksh`, macOS 25.6), 2026-09-20,
// from a script file run with no arguments, with `function outer { inner a b
// c; }` and `outer 1 2 3`:
//
//	level   $1       $#   $*        $0
//	1       1        3    `1 2 3`   outer
//	0       unset    0    empty     the script's name
//	2       a        3    `a b c`   inner
//
// Level 0 is the discriminating row and it is why the script is run without
// arguments: a selection that did nothing would answer the callee's 3 there,
// and one that merely reached the *caller* would answer the caller's 3 as
// well. Running the same script with two arguments answers `s1` and 2 at
// level 0, which says it is the script's own list and not simply emptiness.
//
// `$0` follows the dialect's ordinary rule over a **truncated** stack rather
// than naming the frame at the level. Measured with `function A` calling
// `B()` calling `function C`: at level 2, which is `B`, `$0` is `A`. In the
// shell that has this selection `$0` names the innermost *keyword* function,
// and a `name()` frame is transparent to that walk whether or not a level has
// been selected — so cutting the stack at the selected frame and asking the
// usual question is the whole of it, where reading the frame's own name would
// have answered with the script's.
//
// **How it is done.** The positional list is not a chain and has no
// save-and-restore records the way the variable scopes do: `Runner.Params` is
// one slice, swapped for the arguments on the way into a call and put back on
// the way out. So the list of any frame but the innermost is knowable only at
// the moment the call is made, and it is written down there — see
// Frame.outerParams, stamped by callFuncAs. The frame standing at level n+1
// carries level n's list, which is also how level 0 is answered with no frame
// of its own.
//
// **Narrowed on purpose, the same way the variable half is.** Only *reads*
// move: `$1`, `$@`, `$*`, `$#`, a slice of either list, and `$0`. `set --`,
// `shift`, an assignment to a positional, `getopts`, and a `for` or `select`
// with no word list are left reading and writing the live list, because a
// write into another frame's arguments is a second question with its own
// unwinding and answering half of it would be worse than answering none. See
// #3952, which records the rest.

// selectedFrameIndex is the index in r.frames of the frame **above** the one
// a script selected — the one carrying the selected level's saved positional
// list, and the first frame the selection makes invisible.
//
// One index answers both readers here: the list is that frame's outerParams,
// and the stack `$0` is asked over is r.frames up to it.
func (r *Runner) selectedFrameIndex() (int, bool) {
	level, ok := r.divertingSelection()
	if !ok {
		return 0, false
	}
	// r.frames is outermost first and the function frames in it count up
	// from 1, so the frame wanted is the one standing at level+1.
	at := r.FunctionDepth()
	for i := len(r.frames) - 1; i >= 0; i-- {
		if !r.frames[i].IsFunction() {
			continue
		}
		if at == level+1 {
			return i, true
		}
		at--
	}
	return 0, false
}

// paramsInSelectedFrame is the positional list of the frame a script
// selected, and whether a selection is moving the list at all.
//
// False for every run that has not selected a frame, which is the whole of
// what the readers pay for it.
func (r *Runner) paramsInSelectedFrame() ([]string, bool) {
	i, ok := r.selectedFrameIndex()
	if !ok {
		return nil, false
	}
	return r.frames[i].outerParams, true
}

// params is the positional list a parameter expansion reads: the selected
// frame's where a selection is diverting, and the live one otherwise.
//
// Every read of `$1`, `$@`, `$*` and `$#` goes through this rather than
// through r.Params directly. The writes deliberately do not — see the note on
// narrowing at the top of this file.
func (r *Runner) params() []string {
	if p, ok := r.paramsInSelectedFrame(); ok {
		return p
	}
	return r.Params
}

// framesInSelectedFrame is the call stack as the selected frame sees it:
// everything from the selected frame outward, with the frames a selection
// hides cut off.
//
// It is r.frames when nothing is selected, so `$0` asks its ordinary question
// of its ordinary input and the selection is one slice expression rather than
// a second reading.
func (r *Runner) framesInSelectedFrame() []Frame {
	i, ok := r.selectedFrameIndex()
	if !ok {
		return r.frames
	}
	return r.frames[:i]
}
