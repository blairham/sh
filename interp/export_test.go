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

// PipeDirForTest is the directory this shell made to hold the regular file a
// `=(cmd)` writes, or "" if it never needed one.
//
// Exported because a shell that removes its own scratch leaves nothing to look
// for afterwards, and the tests about where that scratch goes and that it is
// taken away again need to name it. The reading and writing forms had one too
// until #2893, and expand to `/dev/fd/N` now; what stands in for this where
// nothing opens the path is PipesMadeForTest below.
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

// PipesMadeForTest is how many process substitutions this shell tree has made,
// of any of the three spellings.
//
// The evidence a substitution *happened* where nothing else is: a `<(:)` in a
// pattern, in a condition, or in a parameter operand expands to a path that
// nothing opens, so the inner command is never started and not one Action or
// Event reaches a test. The path used to be a named pipe in a directory of the
// shell's own and PipeDirForTest was that evidence; since #2893 the path is
// `/dev/fd/N` and there is no directory unless a `=(cmd)` wrote a file, so the
// count is what is left that every spelling answers.
//
// The counter is the one that numbers `=(cmd)`'s files, which is why it is in
// the box rather than on the Runner: one box per shell *tree*, so a subshell's
// substitutions are counted with its parent's.
func (r *Runner) PipesMadeForTest() uint64 {
	if r.procSubHome == nil {
		return 0
	}
	return r.procSubHome.seq.Load()
}

// SetControlWord reads a `--name` word the `set` builtin takes on its own
// account, by unique prefix. Exported so the prefix rule can be asked
// directly: the behavior it gates is a dialect's, and a test that could only
// reach it through one would be measuring two things at once.
var SetControlWord = setControlWord

// ValueBackslashMarkForTest and ValueBackslashRanOutOfValueForTest are the two
// marks a value's backslash is written as in the escaped form.
//
// Exported for one test, and the smallest possible thing to export for it: the
// doubled mark is spelled as a literal because a Go constant cannot be built
// from another, so nothing but a test holds the two together. See
// valueBackslashRanOutOfValue.
const (
	ValueBackslashMarkForTest          = valueBackslashMark
	ValueBackslashRanOutOfValueForTest = valueBackslashRanOutOfValue
)
