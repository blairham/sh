// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// operandGaveUpTheBuiltin reports whether the builtin part-way through an
// operand list has stopped: the shell is ending, or the *command* is being
// given up and the operands after this one are never reached.
//
// The second half is what the operand loops were missing. `controlAbandon` is
// consumed at the top-level statement loop rather than inside the command —
// which is the whole of what makes it "the rest of the line" rather than "the
// rest of the script" — so a loop watching only for `controlExit` ran on to
// its next operand after the give-up. Measured 2026-09-17, bash 5.3.20 and
// 3.2.57 alike:
//
//	a=(1 2 3); declare 'a[b c]'=v x=1 y=2   x and y are both unset
//	x=1; y=2; unset x 'a[b c]' y            x is gone and y is still 2
//
// So the operands *before* the bad one stand and the ones after it are never
// reached, which is the rule `read 'r[1/0]' b` already follows through the
// `refused` flag its own loop reads (#3494). These two followed it nowhere,
// and the declaration half arrived with the abandon itself (#3495, #3506).
//
// A predicate rather than five copies of the comparison, for the reason
// badSubscriptGivesUp is one door: the shape is what was wrong at each site,
// and a sixth site added tomorrow should not have to rediscover it.
func (r *Runner) operandGaveUpTheBuiltin() bool {
	return r.ctl == controlExit || r.ctl == controlAbandon
}
