// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A builtin saying that its own write did not happen, so the shell's sentence
// for a failed write can be said.
//
// The shell has two of those sentences and neither belongs to the builtin: one
// is the dialect's `BuiltinWriteError`, and one is the sentence for a write to a
// stream **something else** closed — `write error: bad file descriptor`, with no
// builtin in the location, which is what `exec >&-; print foo` gets and what
// `print foo >&-` does not. Which of them is said, whether the command fails,
// and what a broken pipe does instead are all decided in builtinWriteStatus and
// are not this caller's business; all it has to do is not swallow the error.
//
// Exported for dialect builtins that write through a stream of their own rather
// than through Runner.printf. `echo` and `printf` are this package's and record
// it already, which is why `exec >&-; echo foo` said `write error: bad file
// descriptor` while the same line with `print` said nothing at all: one builtin
// out of three was handling the error itself and dropping it (#4436, the
// `'>&-' with attempt to use closed fd` chunk of A04redirect.ztst).
//
// A builtin that has its *own* sentence for the failure — `print -u3` on a
// descriptor open for reading is `bad mode on fd 3` — says that instead and does
// not call this: the reference writes one sentence there, not two.
func (r *Runner) BuiltinWriteFailed(err error) {
	if err != nil {
		r.writeFailed = err
	}
}
