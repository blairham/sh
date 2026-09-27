// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/internal/opened"

// TerminalName is the path of the terminal this shell holds, and false where
// it holds none or the kernel will not name it.
//
// The stream table and not the process's, for the reason
// [Runner.descriptorIsTerminal] gives: an embedded Runner may have been given
// a pipe while the program around it sits at a terminal, and the shell's
// terminal is the shell's. Runner.terminal is the same lookup `$COLUMNS`,
// `read -k` and [Runner.TerminalIdle] make, remembering included.
//
// The name comes from `internal/opened`, which is the package that already
// asks a descriptor what it is called — `F_GETPATH` on one kernel and
// `/proc/self/fd` on the other. The standard library has no `ttyname`, and a
// second implementation of the question beside the gate's is exactly the
// shape this tree keeps finding as two copies that drift.
//
// **False and the empty string are different answers** and a caller must not
// fold them: a kernel that declines to name a descriptor has not said the
// terminal is nowhere, and a dialect writes its own word for each.
func (r *Runner) TerminalName() (string, bool) {
	f, held := r.terminal()
	if !held {
		return "", false
	}
	return opened.Path(f)
}
