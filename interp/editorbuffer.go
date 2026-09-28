// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The **editor buffer stack**: the lines a script has put aside for the line
// editor to pick up, and the one thing `read -z` reads.
//
// It is not the shell's input. `read -z` never touches the stream — measured
// 2026-09-28 on zsh 5.9.2, `printf 'x\ny\n' | { read -z l; read m; }` leaves
// `l` empty and `m` holding `x`, so the `x` is still on the stream for the
// next reader. What the letter reads is this stack, and the only things that
// push onto it are `zle push-line` and `print -z`.
//
// The **storage is the dialect's** and only the pop is here, which is the same
// split [Runner.SetHistoryStore] makes for the history list and for the same
// reason: where the entries live is a question about the shell, a subshell has
// to get its own copy of them, and the dialect that has `print -z` already
// keeps such a list under a name no script can reach. Nothing in the core
// pushes, so nothing in the core needs to say where.
//
// A dialect with no such stack leaves the hook nil, and `read -z` is then a
// letter its `Semantics.ReadOptions` does not have — which is every dialect
// but one. Measured: `read -z v` is `invalid option` on bash 5.3.20, `unknown
// option` on ksh93u+ and `Illegal option -z` on dash, all at 2.

// SetEditorBufferPop hands the Runner the dialect's way of taking one entry
// off the editor buffer stack.
//
// The pop reports the entry and whether there was one, which is the whole of
// what `read -z` needs: an empty stack is not an error and not an empty entry
// either — it is a read that did not happen, and the status says so.
func (r *Runner) SetEditorBufferPop(pop func(*Runner) (string, bool)) {
	r.editorBufferPop = pop
}

// popEditorBuffer takes one entry, or reports that there was none.
//
// False for a dialect that never registered a stack, which is the same answer
// an empty one gives — a `read -z` in such a shell is a letter that dialect's
// option set does not accept, so the case is unreachable rather than wrong.
func (r *Runner) popEditorBuffer() (string, bool) {
	if r.editorBufferPop == nil {
		return "", false
	}
	return r.editorBufferPop(r)
}
