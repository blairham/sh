// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// The editor buffer stack: what `print -z` pushes and `read -z` takes off.
//
// `print -z` was a no-op — "a line editor that is not running: the operands
// are consumed and nothing is written" — which is right about the *writing*
// and wrong about the stack. The stack is a script-visible thing with or
// without an editor, and the pair round-trips in a plain `zsh -f` script:
// measured 2026-09-28 on zsh 5.9.2 under `-f` from a script file,
//
//	print -z "buffered line"; read -z l   →  status 0, l is `buffered line`
//	read -z l                             →  status 1, l cleared
//
// so a `read -z` that could only ever answer 1 would have been the script-only
// half of a letter that has a script-only producer.
//
// # Last in, first out
//
// Measured, and it is the row that says this is a stack rather than a slot:
// `print -z one; print -z two` then two reads give `two` and then `one`, and a
// third answers 1.
//
// # What is pushed is one entry, joined with a space
//
// `print -z one two` pushes the single entry `one two`, and `-l` and `-N` do
// not change it — which the writing side of this builtin already had measured
// and is why the join is not repeated here. An operand list that joins to
// **nothing** pushes no entry at all: `print -z` with no operands and
// `print -z ""` both leave the stack as it was, so the next `read -z` answers
// 1. That is the one row that separates "pushed an empty entry" from "pushed
// nothing", and an empty entry would have read back at status 0.

// editorBufferStore is the stack itself, kept as an indexed array under a name
// no script can reach — the way `zmodload`, `zstyle` and `emulate` keep
// theirs, which is also what gives a subshell its own copy.
//
// Oldest first, so the newest entry is the last element and a pop is a trim
// off the end. The order in the array is the opposite of the order a `read -z`
// sees them in, which is what a stack is.
const editorBufferStore = ".zsh.editorbuffer"

// pushEditorBuffer puts one entry on the stack, and is what `print -z` does
// instead of writing.
//
// An entry with no bytes in it is not pushed, which is measured rather than an
// optimization — see the note above.
func pushEditorBuffer(r *interp.Runner, entry string) {
	if entry == "" {
		return
	}
	stored, _ := r.GetArray(editorBufferStore)
	r.SetArray(editorBufferStore, append(append([]string(nil), stored...), entry))
}

// popEditorBuffer takes the newest entry off, for `read -z`.
//
// The whole of what the core is given — see [interp.Runner.SetEditorBufferPop],
// which is where the split between the storage and the read is argued.
func popEditorBuffer(r *interp.Runner) (string, bool) {
	stored, ok := r.GetArray(editorBufferStore)
	if !ok || len(stored) == 0 {
		return "", false
	}
	last := len(stored) - 1
	entry := stored[last]
	r.SetArray(editorBufferStore, append([]string(nil), stored[:last]...))
	return entry, true
}

// registerEditorBuffer hands the core the pop half.
func registerEditorBuffer(r *interp.Runner) {
	r.SetEditorBufferPop(popEditorBuffer)
}
