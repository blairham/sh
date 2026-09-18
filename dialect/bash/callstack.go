// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// The three arrays bash keeps in step about where a script is.
//
//	BASH_SOURCE   the file each frame was read from
//	FUNCNAME      the function each frame is in
//	BASH_LINENO   the line each frame was called from
//
// `${BASH_SOURCE[0]}` is how a script finds its own directory, which is the
// first thing a great many of them do — bats stops on it before printing
// anything at all.
//
// The stack is the core's and the names are bash's. zsh spells the same
// state `funcstack` and `funcfiletrace`, ksh93 has `.sh.fun`, and dash has
// none of it, which is why nothing under interp/ says any of these words.
func registerCallStack(r *interp.Runner) {
	r.SetDynamicArray("BASH_SOURCE", func(r *interp.Runner) []string {
		frames := r.CallStack()
		out := make([]string, 0, len(frames))
		for _, f := range frames {
			out = append(out, f.File)
		}
		return out
	})

	r.SetDynamicArray("FUNCNAME", func(r *interp.Runner) []string {
		if !r.InCall() {
			// Absent at the top level rather than empty, which is a
			// different thing to a script testing it: bash documents this as
			// existing only inside a function.
			return nil
		}
		frames := r.CallStack()
		out := make([]string, 0, len(frames))
		for _, f := range frames {
			if f.Name == "" {
				// The script's own frame, which bash calls `main`. Only
				// there when the shell was given a file: `-c` reports the
				// function alone.
				out = append(out, "main")
				continue
			}
			out = append(out, f.Name)
		}
		return out
	})

	// The other two, which are a *record* rather than a view of the stack:
	// they are kept only while extended debugging is on, and what is in them
	// is what was in them when each call was entered. See
	// interp/callarguments.go, and Runner.SetRecordsCallArguments for what
	// turning the record on does to the frame it is turned on in.
	//
	// BASH_ARGC is one count per call, innermost first; BASH_ARGV is every
	// argument of every call in one list, and it is a **stack** — the
	// innermost call's last argument is element 0. Measured on bash 5.3.15,
	// 2026-09-14: `g(){ …; }; f(){ g x y z; }; f a b` under `shopt -s
	// extdebug` answers `3 2 0` and `z y x b a`, where the trailing `0` is
	// the top level's own arguments under `-c` with no operands (#2476).
	r.SetDynamicArray("BASH_ARGC", func(r *interp.Runner) []string {
		frames := r.CallArguments()
		out := make([]string, 0, len(frames))
		for _, args := range frames {
			out = append(out, strconv.Itoa(len(args)))
		}
		return out
	})

	r.SetDynamicArray("BASH_ARGV", func(r *interp.Runner) []string {
		// Never nil: the parameter **exists** with no calls recorded, where
		// FUNCNAME does not, and a listing tells the two apart by exactly
		// that. Measured 2026-09-18 on bash 5.3.20 at the top level,
		// `declare -a BASH_ARGV=()` against `declare -a FUNCNAME` with no
		// `=`. See declarationOf's produced-array branch.
		if flat := r.CallArgumentsFlat(); flat != nil {
			return flat
		}
		return []string{}
	})

	// The other two, which are a *record* rather than a view of the stack:
	// they are kept only while extended debugging is on, and what is in them
	// is what was in them when each call was entered. See
	// interp/callarguments.go, and Runner.SetRecordsCallArguments for what
	// turning the record on does to the frame it is turned on in.
	//
	// BASH_ARGC is one count per call, innermost first; BASH_ARGV is every
	// argument of every call in one list, and it is a **stack** — the
	// innermost call's last argument is element 0. Measured on bash 5.3.15,
	// 2026-09-14: `g(){ …; }; f(){ g x y z; }; f a b` under `shopt -s
	// extdebug` answers `3 2 0` and `z y x b a`, where the trailing `0` is
	// the top level's own arguments under `-c` with no operands (#2476).
	r.SetDynamicArray("BASH_ARGC", func(r *interp.Runner) []string {
		frames := r.CallArguments()
		out := make([]string, 0, len(frames))
		for _, args := range frames {
			out = append(out, strconv.Itoa(len(args)))
		}
		return out
	})

	r.SetDynamicArray("BASH_ARGV", func(r *interp.Runner) []string {
		// Never nil: the parameter **exists** with no calls recorded, where
		// FUNCNAME does not, and a listing tells the two apart by exactly
		// that. Measured 2026-09-18 on bash 5.3.20 at the top level,
		// `declare -a BASH_ARGV=()` against `declare -a FUNCNAME` with no
		// `=`. See declarationOf's produced-array branch.
		if flat := r.CallArgumentsFlat(); flat != nil {
			return flat
		}
		return []string{}
	})

	// Four of the five refuse `unset`, and FUNCNAME is the one that does not
	// — see Runner.RefuseUnset for the measurement.
	for _, name := range []string{"BASH_SOURCE", "BASH_LINENO", "BASH_ARGV", "BASH_ARGC"} {
		r.RefuseUnset(name)
	}

	// And how the five list back, which a produced parameter has to be told:
	// `declare -p FUNCNAME` was `FUNCNAME: not found` at 1 from a name the
	// same shell answers `${FUNCNAME[0]}` for, so the listing and the
	// expansion disagreed about whether the name existed (#3099).
	//
	// No letters to state — the `-a` a listing writes comes from the elements
	// being an array — so the declaration is empty and its whole job is to
	// put the name into a listing at all. Measured 2026-09-18 on bash 5.3.20:
	// `declare -a BASH_SOURCE=([0]="p3.sh")` from a script, `declare -a
	// FUNCNAME` with no `=` at the top level where the parameter is absent
	// rather than empty, and `declare -a BASH_ARGV=()` for one that is empty.
	for _, name := range []string{
		"BASH_SOURCE", "FUNCNAME", "BASH_LINENO", "BASH_ARGC", "BASH_ARGV",
	} {
		r.SetDynamicDeclaration(name, interp.ProducedDeclaration{Array: true})
	}

	r.SetDynamicArray("BASH_LINENO", func(r *interp.Runner) []string {
		frames := r.CallStack()
		out := make([]string, 0, len(frames))
		for _, f := range frames {
			// Nothing called the script, so its own frame reports 0 — which
			// is what the zero Line already holds.
			out = append(out, strconv.Itoa(f.Line))
		}
		return out
	})
}
