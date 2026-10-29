// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// What one run of a simple command keeps about itself while it runs.
//
// A prefix value is expanded once and kept, because two parties read it — the
// trace line and the route that stores it — and a substitution in it must run
// once. The record is found again by the *syntax.Assign it belongs to, which is
// the right key within one run of a command and the wrong one across two: a
// function that calls itself through `v=$n f` runs the same node again while
// the first run is still going, and the second run found the first one's
// record and stored its value rather than expanding `$n`. Measured 2026-10-29
// from script files under `env -i PATH=/usr/bin:/bin`:
//
//	f() { echo "in v=$v"; n=$((n-1)); [ "$n" -ge 0 ] && v=$n f; }; n=3; v=x f
//
//	bash 5.3.20, ksh93u+ 2012-08-01, zsh 5.9.2, dash   in v=x, v=2, v=1, v=0
//	this engine, bash and ksh                          in v=x, v=2, v=2, v=2
//	this engine, all four under `set -x`               the same, lines too
//
// bash and ksh record on every run, in the ordered walk they make ahead of the
// redirections; zsh and dash record a call's prefix only for the trace.
//
// The records were put back when a command ended, which is what keeps an outer
// command's across a command run in the middle of it (#6752). They were never
// set aside when one started, and what was put back was what stood when the
// capture was taken — partway through the command, after the two walks that
// run first had already recorded: the frozen-name walk and the walk in front
// of a declaration's operand. Those records outlived their command, so the
// next pass of a loop read them back without any recursion at all. Measured
// the same day:
//
//	for i in 1 2 3; do v=$(echo "sub $i" >&2) typeset x=1; done
//	                 ksh93u+, zsh 5.9.2: sub 1, sub 2, sub 3; this engine: sub 1
//	readonly r; for i in 1 2 3; do r=2 B=$i f; done
//	                 bash 5.3.20: f sees B=1, B=2, B=3; this engine: B=1 three times
//
// The same capture put back the speaker this command had just chosen rather
// than the one before it, so the next command's failed value was located as
// the last one's: in ksh93u+, `b=$((1)) echo hi` on line 1 moved line 2's
// `a=$((1/0)) typeset x=1` from `S: line 2:` to `S[2]:`. And the two fields
// beside the capture were cleared rather than put back: bash 5.3.20 writes
// `r: readonly variable` twice for `readonly r=1; r=2 B=${ r=3 /bin/echo i;
// } f` where an inner command clearing prefixCheckedFirst made it three, and
// zsh 5.9.2 writes `( q1 /bin/sh )` at the end of an element literal's trace
// line that a recursion through glob qualifier code in its own words had
// emptied.
//
// So every such record lives here, and Runner.simple sets the whole of it aside
// as a command starts and puts it back as the command ends. A record added here
// is scoped to the run of its command without anybody having to list it, which
// is the point of one struct rather than fields each place enumerates.
type commandRecords struct {
	// prefixCheckedFirst records that this command's assignment prefix has
	// already been checked for a frozen name, ahead of its values and its
	// redirections — the order Semantics.PrefixToAFrozenNameIsCheckedFirst
	// answers — so the dispatch routes skip the value of a frozen name and
	// the ordinary check does not report the same names a second time.
	prefixCheckedFirst bool
	// prefixSpeaker is who a value in the running command's assignment
	// prefix is located as if it will not expand, decided once per command
	// from the command itself. See interp/prefixfailurelocation.go.
	prefixSpeaker prefixValueSpeaker
	// prefixTraceAssigns and prefixTraceValues hold this command's
	// assignment-prefix values. `set -x` has to write the value it is about
	// to hand over and the route that applies it has to hand over the same
	// one, and the expansion may run a command substitution — so it happens
	// once and both read it. See Runner.expandPrefixTraceValues and
	// interp/prefixredirorder.go.
	//
	// Two parallel slices rather than a map keyed by the assignment, which
	// is what this was: a prefix is one, two or three assignments, so the
	// scan is shorter than hashing a pointer — and a map here is a *table*
	// on the Runner, which every clone then has to own or be excused from
	// sharing. TestACloneOwnsEveryTable says so, and it is right to: a
	// per-command scratch value is not a table and should not have to
	// pretend to be one.
	prefixTraceAssigns []*syntax.Assign
	prefixTraceValues  []string
	// prefixTracedEarly are the prefix entries already written by the
	// frozen-name walk, ahead of its refusal. See
	// Runner.expandThePrefixUpToTheFrozenName. A slice, on the terms
	// prefixTraceAssigns gives.
	prefixTracedEarly []*syntax.Assign
	// prefixTraceJoins is the third, and it holds what an **appending**
	// entry came to: the name's value at the moment this entry expanded,
	// with the expansion behind it. Recorded rather than derived again,
	// because the ordered walk in interp/prefixredirorder.go writes an
	// entry's own trace line *after* it has held that joined value under the
	// name — so asking Runner.prefixJoined a second time reads the join back
	// and appends the word to it twice: `v=14; v+=5 :` traced `+ v=1455`
	// where the one column that writes an append as its joined value writes
	// `+ v=145` (#4431). A plain assignment's entry holds its own value, so
	// the slice is the same length as the two above and is read by index
	// with them.
	prefixTraceJoins []string
	// prefixGlobMatches holds what this command's assignment prefixes
	// matched, for the one shell that reads a prefix's value as a pattern —
	// see interp/prefixglob.go. Per-command scratch and not a table, for the
	// reasons the slices above give.
	prefixGlobMatches []prefixGlobMatch
	// prefixSubscripts is what this command's prefix walk read of each
	// subscripted entry's subscript, in turn, for the line that writes it and
	// the store that takes it — see interp/prefixsubscriptinturn.go.
	// Per-command scratch beside the slices above, for their reasons.
	prefixSubscripts []*prefixSubscriptRead
	// openTraceLiteral is the element literal whose words the open trace line
	// is waiting for. See Runner.openTraceTakesTheLiteral.
	openTraceLiteral *syntax.Assign
}
