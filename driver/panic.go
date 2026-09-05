// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"

	"github.com/blairham/sh/internal/panicguard"
)

// guard is what this shell catches an interpreter bug with.
//
// Every route a shell can be invoked by ends up running one, because a panic
// costs whatever the guard around it was holding: the whole of a script on the
// `-c` and file routes, which are not sessions and have nothing to keep going
// for; one startup file where a session is being built; and one typed line at
// a prompt, which repl holds because it is the only thing that knows where a
// line ends. interp itself keeps panicking, which is correct for a library —
// see internal/panicguard for the whole of the reasoning.
func (sh Shell) guard() panicguard.Guard {
	return panicguard.Guard{Name: sh.Name, Err: sh.Stderr, Trace: panicTrace()}
}

// panicTrace reports whether a caught panic should print its stack.
//
// Read here rather than in the guard, for the reason every other read of
// process state is made in this package: driver is the front end and *is* the
// process, while interp and repl are libraries an embedder links, where
// reaching for the environment is the leak the purity rule exists to stop. The
// answer travels down as a value, the same way the environment itself does.
func panicTrace() bool { return os.Getenv(panicguard.TraceVar) != "" }
