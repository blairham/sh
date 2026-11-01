// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// A signal that arrives again while it is already waiting — after it arrived
// and before its handler had a turn (#7166).
//
// The rule has one noun in it, and the noun is *waiting*: not the number of
// signals sent, not the one `kill` that sent them, and not where they came
// from. Measured 2026-11-01 under `-c`, each shell in a process group of its
// own (`perl -e 'setpgrp(0,0); exec @ARGV'`), `trap "echo T" USR1` in front
// of each row:
//
//	                                                bash 5.3.20, 3.2.57,
//	                                                ksh93u+, dash, ash    zsh 5.9.2
//	kill -USR1 $$ $$; echo after                    T after               T T after
//	kill -USR1 0 $$; echo after                     T after               T T after
//	kill -USR1 0; kill -USR1 0; echo after          T T after             T T after
//	two /bin/kill -USR1 $$, 0.2 s apart, from a
//	  background job, during one /bin/sleep 1       T after               T T after
//	the same, the second during a second /bin/sleep T T after             T T after
//
// The last two rows are the pair that holds the sender fixed: the same
// external program, the same signal, the same two sends — and what moves the
// answer is only whether the shell had a turn to run the handler between
// them. And the first row against the fourth holds the arrival fixed while
// changing the sender from one builtin to two processes, which moves nothing.
// The second row is the first since #7163, which answers the shell's own copy
// of a group kill at the send. BusyBox ash is 1.38.0 in `busybox:latest`.
//
// Two different signals waiting both run, in every column. USR1, USR2 and
// USR1 again, sent by `/bin/kill` during one `/bin/sleep`, run `U1 U2` in
// bash, dash and ash and `U2 U1` in ksh93 — the order is
// Semantics.WaitingHandlersRun — and `U1 U2 U1` in zsh. With a turn between
// them, as `kill -USR1 $$; kill -USR2 $$ $$ $$; kill -USR1 $$ $$ $$` gives,
// they are `U1 U2 U1` in the five, and zsh runs all seven.
//
// A forked body answers as the shell does: `( trap "echo T" USR1; kill -USR1
// <own pid> <own pid>; echo after )` is `T after` in bash 5.3 with
// `$BASHPID`, and `T T after` in zsh with `$sysparams[pid]`.
//
// **A child's state change is not one of these.** CHLD is raised here once
// per child the shell reaped (Runner.childReaped), and bash and ksh93 run
// their CHLD handler once per child too: three background children ending
// during one `/bin/sleep` are `C C C C` in bash 5.3 — the fourth is the
// sleep — and `C C C` in ksh93. dash and ash run it once there, which this
// does not model.

// waitingSignalAgainAxis names the axis in a refusal.
const waitingSignalAgainAxis = "how often a signal that arrived again while it was waiting runs its handler"

// turnsTaken is the conditions of one batch whose handler has had its turn,
// where the dialect runs a condition waiting more than once a single time.
// Nil runs every arrival.
type turnsTaken map[string]bool

// again reports whether an arrival repeats one that already had its turn in
// this batch, and records that this one has.
func (t turnsTaken) again(name string) bool {
	if t == nil || name == "CHLD" {
		return false
	}
	if t[name] {
		return true
	}
	t[name] = true
	return false
}

// arrivingAgainRunsOnce reads Semantics.WaitingSignalArrivingAgainRunsOnce for
// one batch of waiting arrivals, and answers what to skip: a turnsTaken where
// the arrivals of a condition in it more than once run its handler once, nil
// where every arrival runs. handled reports whether a condition has a handler
// that runs something.
//
// Asked only where the batch holds one such condition twice, the one place
// the answers part. The bool is false where the axis has no answer, and
// nothing is then run.
func (r *Runner) arrivingAgainRunsOnce(batch []string, handled func(string) bool) (turnsTaken, bool) {
	if !repeatsAHandledSignal(batch, handled) {
		return nil, true
	}
	switch a := r.sem().WaitingSignalArrivingAgainRunsOnce; a {
	case Yes:
		return turnsTaken{}, true
	case No:
		return nil, true
	default:
		r.ask(a, waitingSignalAgainAxis)
		return nil, false
	}
}

// repeatsAHandledSignal reports whether a batch holds one condition with a
// handler more than once, a child's state change aside.
func repeatsAHandledSignal(batch []string, handled func(string) bool) bool {
	for i, name := range batch {
		if name == "CHLD" || !slices.Contains(batch[i+1:], name) {
			continue
		}
		if handled(name) {
			return true
		}
	}
	return false
}
