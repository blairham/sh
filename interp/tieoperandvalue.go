// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether one of a tie's operands was written as an **array literal**.
//
// `-T`'s three positions take three different kinds of thing, and two of the
// refusals for an operand holding the wrong kind are about a literal: an array
// on the scalar half is `first argument of tie must be scalar`, and one in the
// separator's place is `third argument of tie must be join character`. Neither
// can be read off the operand's text, because the parser lifts a literal out of
// the command and the utility is handed the **bare name** — `typeset -T A a=(1)
// b=(2)` arrives as `A`, `a`, `b`, with nothing in the third word to say it had
// a value.
//
// **The position is the key, not the name**, and that is measured rather than
// tidy. `typeset -T A a=(1) a` is taken at 0 in the reference and
// `typeset -T A a a=(1)` is refused, and the two are the same three names:
// what differs is which position the literal was written at. A rule that asked
// "is there a literal named `a`" would answer the same for both and be wrong
// about one of them. Runner.arrayOperands already records each literal's index
// in the command's word list — it is what puts the operand back where it was
// written, see interp/operandwrittenorder.go — so this asks that record.
//
// The operands of a declaration are the tail of its word list, which is what
// declarationArgvLen is for: the utility's own word and its letters stand in
// front of them, and neither is in `args`.
func (r *Runner) tieOperandIsAnArrayLiteral(args []string, i int) bool {
	if i >= len(args) || r.declarationArgvLen == 0 {
		return false
	}
	at := r.declarationArgvLen - len(args) + i
	for _, op := range r.arrayOperands {
		if op.at == at {
			return true
		}
	}
	return false
}
