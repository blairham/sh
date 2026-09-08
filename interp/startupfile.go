// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"

	"github.com/blairham/sh/syntax"
)

// RunStartupFile runs a shell's own startup file — the run-commands file, the
// login profile, `$ENV`, `$BASH_ENV` — as the sourced script it is.
//
// It exists because a startup file *is* one, and ours was not. The front end
// parsed the file itself, for the sake of a diagnostic that names the file the
// way a file is named, and then handed the statements to [Runner.Run] — which
// is the entry point for a *script's own top level*. So there was nothing for
// a `return` to return from, and the refusal that belongs to a script's top
// level fired inside a person's `~/.bashrc`:
//
//	bash: line 2: return: can only `return' from a function or sourced script
//
// Measured across the panel with `echo BEFORE; return 3; echo AFTER` as the
// whole file: all six columns accept it, stop reading the file there, and say
// nothing. Nobody diagnoses. `return` in an rc is how a real `.bashrc` bails
// out early, most often as `[ -z "$PS1" ] && return`, so a refusal here both
// writes a line into every startup and leaves the file running past the point
// it meant to stop (#1422).
//
// The status is where the panel splits, and only over the *argument*. `return`
// with no argument means the status before it everywhere; `(exit 5)` as the
// last line of a startup file leaves 5 everywhere. But `return 3` at the top
// of one leaves 3 in dash, ksh93 and zsh and leaves bash with whatever the
// command before it left. See Semantics.StartupFileReturnCarriesItsArgument,
// which has the grid.
//
// The parse is still the caller's. A startup file that will not parse is
// reported as a file — the path and the line, not `.`: two different questions
// with two different answers, and this one is about control flow.
func (r *Runner) RunStartupFile(ctx context.Context, f *syntax.File) (int, error) {
	// The frame a `return` returns from. A count rather than a flag, and
	// raised the same way `.` raises it, because a startup file may source
	// another and each of them is its own boundary.
	r.sourceDepth++
	defer func() { r.sourceDepth-- }()
	err := r.RunPart(ctx, f)
	if r.ctl == controlReturn {
		// Caught, so the shell goes on to the next startup file and then to
		// the prompt rather than staying in a returning state. Exactly what
		// `.` does at the end of a sourced file, and for the same reason.
		r.ctl = controlNone
		if !r.ask(r.sem().StartupFileReturnCarriesItsArgument, "the status a `return` in a startup file leaves") {
			// bash: the argument is dropped and the shell's status is
			// whatever the last command before the `return` left. Measured:
			// an rc of `return 3` leaves 0 at the first prompt and one of
			// `false; return 3` leaves 1.
			//
			// Read from the field the RETURN trap reads, which is the same
			// moment asked about for a different reason — the trap's action
			// sees `$?` as the `return` found it. One field rather than two,
			// because a `return` that set one and not the other would be
			// answering the same question twice and drifting.
			r.status = r.returnSeenStatus
		}
	}
	return r.status, err
}
