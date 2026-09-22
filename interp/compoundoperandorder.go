// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Where a declaration utility's own `set -x` line stands against what its
// **compound-variable** operand does.
//
// `typeset c=(a=1 b=2)` is one command carrying one operand whose body is a
// list of declarations, and each of those traces itself as it is performed —
// which this shell already did. What it had wrong is the side of the command's
// own line they were written on.
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> x.sh` over
// a script file with `set -x` on line 1, standard input on the null device,
// AT&T ksh93u+ 2012-08-01 (`/bin/ksh`):
//
//	written                       	ksh93u+                              	ours, before
//	`typeset c=(a=1 b=2)`         	`+ c.a=1` `+ c.b=2` `+ typeset c`     	the command first
//	`typeset -C d=(p=3 q=(r=4))`  	`+ d.p=3` `+ d.q.r=4` `+ typeset -C d`	the command first
//	`typeset c=(a=1 b=$(echo two))`	`+ c.a=1` `+ echo two` `+ c.b=two` `+ typeset c`	the command first
//	`typeset c=(a=1 b=${c.a})`    	`+ c.a=1` `+ c.b=1` `+ typeset c`    	the command first
//	`typeset x=1 c=(a=1)`         	`+ x=1` `+ c.a=1` `+ typeset x c`    	`+ x=1` `+ typeset x c` `+ c.a=1`
//	`typeset e=(1 2) k=(a=1)`     	`+ e=( 1 2 )` `+ k.a=1` `+ typeset e k`	`+ e=( 1 2 )` `+ typeset e k` `+ k.a=1`
//	`typeset c[1]=(a=1)`          	`+ c[1].a=1` `+ typeset c`           	the command first
//	`typeset -A m=(p=1)`          	`+ m[0].p=1` `+ typeset -A m`        	the command first
//	`readonly c=(a=1)`            	`+ c.a=1` `+ readonly c`             	the command first
//
// The construct is one column's: a parenthesized body of *assignments* is a
// compound variable only where the grammar has one, which is the same place
// Runner.emptyListIsACompound asks. bash, zsh, dash and BusyBox ash have no
// second answer to disagree with, so this is a fact and not an axis.
//
// **The three rows the mixed operands supply say the lines do not simply swap
// sides.** A scalar operand's line and an array literal's are written in front
// of the command already, and the compound body's members belong *behind*
// those and still in front of the command word — which is the order the script
// wrote them in. So what moves is the command's own line, over the operand
// alone; everything written ahead of it stays where it was.
//
// # It is the work that is in front, not a line held and written early
//
// Two of the rows above say so, and neither can be satisfied by rendering:
// the substitution in `b=$(echo two)` runs there too, and `b=${c.a}` resolves
// to the member the same body stored a moment earlier. The body is performed
// in front of the command, and the trace is what that looks like.
//
// This shell performs it in Runner.assignOperands, which runs **after**
// callBuiltin so that the declaration decides the scope the members land in —
// the order interp/arrayoperandorder.go states and #3842 depends on. A
// compound body assigned in front of the shadow lands on the caller's cell,
// which is the defect #3824 was. So the store does not move.
//
// What moves is the **command's own line**: it is held at the point it would
// have been written and released once the operand has landed. That is the
// shape interp/declareprintoperand.go already has for a `-p` listing, and it
// is what makes the two orders agree — the listing is held too, so
//
//	set -x
//	typeset -p c=(a=1)
//
// is `+ c.a=1`, `+ typeset -p c` and then `typeset -C c=(a=1)` in ksh93u+ and
// here alike. Before #3891 held the listing, the utility's output stood
// between the command and its operand and no amount of moving the line could
// have matched; that is the blocker this issue recorded and it is gone.
//
// # A refused operand writes no line at all
//
//	typeset -r c=(z=9)
//	typeset c=(a=2)
//
// is `c: is read only` alone in ksh93u+ — not the member's line and not the
// command's. So the held line is **dropped** where the operand assignment was
// refused, which is the same claim seen from its failing side: a command whose
// operand never happened was never traced.
//
// # Two rows this does not reach, measured and left alone
//
// Where the utility writes something *before* the operand could land, ksh93
// has still performed the operand and this shell has not, so no ordering of
// the lines it does write can match:
//
//	typeset -x c=(a=1)	`+ c.a=1` `+ typeset -x c` then `only simple variables can be exported`
//	typeset -Q c=(a=1)	`+ c.a=1` `+ typeset -Q c` then `-Q: unknown option`
//
// Both are refusals raised inside the builtin, ahead of any store. The line
// this shell writes is the command's, and it keeps standing in front of the
// diagnostic rather than falling behind it: anything the utility itself
// writes releases the held line first. What is missing there is the member's
// line, which is missing because the member was never stored — a question for
// the store's position and not for the trace's.
//
// Two further rows were found beside these and are neither this issue's nor
// changed by it, because both are about *where the members go* rather than
// when: `typeset -n c=zz; typeset c=(a=1)` follows the reference in ksh93u+
// and writes `c.a=1` here, and a valueless `readonly c` does not refuse a
// later compound body there where it does here.

// heldTrace is a command's own `set -x` line, kept back until the compound
// operand standing in front of it has been performed.
//
// The rendered lines rather than the words, because what the line says is
// decided where it would have been written — the trace prefix is expanded
// there, and the operand about to run can change what a second rendering
// would say.
type heldTrace struct {
	lines []string
	held  bool
	// auto releases the line on the next thing the shell writes. On while
	// the utility runs, so its diagnostics stand behind the command they
	// belong to exactly as they do with nothing held; off while the operand
	// is assigned, which is the one thing that goes in front.
	auto bool
}

// holdsItsLineForACompoundOperand reports whether this command's own trace
// line waits for its operand — which is a command carrying a compound
// variable's body as one.
func (r *Runner) holdsItsLineForACompoundOperand() bool {
	for _, op := range r.arrayOperands {
		if op.assign.Members != nil {
			return true
		}
	}
	return false
}

// holdCommandTrace keeps a command's rendered line back, and reports that it
// took it.
func (r *Runner) holdCommandTrace(lines []string) bool {
	if !r.holdsItsLineForACompoundOperand() {
		return false
	}
	r.heldTrace = heldTrace{lines: lines, held: true, auto: true}
	return true
}

// flushHeldCommandTrace writes a line that was held, if one is still waiting.
//
// Cleared before it writes rather than after: the write goes through the same
// stream the release is armed on.
func (r *Runner) flushHeldCommandTrace() {
	held := r.heldTrace
	r.heldTrace = heldTrace{}
	if !held.held {
		return
	}
	r.awaitTraceTurn()
	defer r.releaseTraceTurn()
	for _, line := range held.lines {
		r.tracef("%s", line)
	}
}

// dropHeldCommandTrace throws a held line away, which is what a refused
// operand earns it.
func (r *Runner) dropHeldCommandTrace() {
	r.heldTrace = heldTrace{}
}

// releaseHeldTraceBeforeWriting is the hook the shell's two output helpers
// call: whatever is about to be written stands behind the command's own line.
func (r *Runner) releaseHeldTraceBeforeWriting() {
	if r.heldTrace.held && r.heldTrace.auto {
		r.flushHeldCommandTrace()
	}
}
