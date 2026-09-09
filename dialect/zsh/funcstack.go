// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

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
