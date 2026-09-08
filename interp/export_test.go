// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// This file exports internals to the external test package, which is the
// standard way to let tests reach them without widening the public API. The
// tests are external because a dialect package imports this one, so an
// in-package test could not import a dialect without a cycle.

// HasProcessGroups reports whether this platform has them, so a test can skip
// rather than assert something the platform cannot do.
const HasProcessGroups = hasProcessGroups

// PipeDirForTest is the directory this shell made to hold the named pipes of
// its process substitutions, or "" if it never needed one.
//
// Exported because it is the only evidence a substitution was performed in the
// cases where nothing opens the path. `case abc in <(:))` expands the word to
// a path and compares it; no reader ever appears, so the inner command is
// never started and not one Action or Event reaches a test. The fifo and the
// directory around it are the whole of what happened.
//
// Those tests used to glob the shell's TMPDIR for it after the run. That
// stopped working when Finish learned to remove the directory (#1284) — and it
// stopped *discriminating* in the direction that matters more: the case
// asserting that a refused condition never ran its substitution wanted the
// TMPDIR empty, which a shell that cleans up after itself leaves it either
// way. Naming the directory rather than counting what is in it survives the
// cleanup and says which shell made which, so a test can assert both that the
// shell made one and that it is gone.
//
// The name is kept after CleanUp on purpose: what a shell put where is a fact
// about the run, and forgetting it would leave nothing to assert against.
func (r *Runner) PipeDirForTest() string {
	if r.procSubHome == nil {
		return ""
	}
	return r.procSubHome.dir
}
