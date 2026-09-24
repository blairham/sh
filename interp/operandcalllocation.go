// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// operandLocatedUnderTheCall is the name a refusal takes when the operand
// assignment that raised it ran *after* its declaration builtin had returned.
//
// The shape is a container letter over an array literal — `readonly -a a=(1)`
// — where the letter has to be recorded before the literal is stored, so the
// store is deliberately left until the builtin is finished. By then nothing is
// speaking: r.inBuiltin has been put back to whatever was running outside, and
// at the top level that is nothing at all. The dialect that carries
// Diagnostics.LiteralOperandAfterADeclarationIsLocatedUnderTheCall answers
// with the call the shell is inside instead, which is what its own store does
// with an empty command name.
//
// Second return says whether to use it, and it is false at the top level
// rather than returning the script's name: measured, the same command outside
// a function answers with no name in front of the variable at all.
//
// A function only. A sourced file is a frame too, and the dialect measured
// answers `.` there — the word the sourcing was written with, which is the
// sourcing builtin's name rather than the frame's. This shell's frames keep
// the operand and not the word, so that case is left alone rather than
// answered with something else; see the field's own note.
func (r *Runner) operandLocatedUnderTheCall() (string, bool) {
	if !r.diag().LiteralOperandAfterADeclarationIsLocatedUnderTheCall {
		return "", false
	}
	if len(r.frames) == 0 {
		return "", false
	}
	f := r.frames[len(r.frames)-1]
	if !f.IsFunction() {
		return "", false
	}
	return f.Name, true
}
