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
