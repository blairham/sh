// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"syscall"
)

// A `kill` aimed at **the shell's own process group** — `kill -USR1 0`, `kill
// -USR1 -$$` where the shell leads its group — reaches the shell as well as
// everything else in the group, and the shell's copy is answered at the send,
// exactly as one aimed at `$$` is.
//
// It still goes to the kernel, and has to: the other members of the group are
// real processes, and a reference's children do receive it — measured
// 2026-10-29, `trap 'echo T' USR1; /bin/sleep 1 & /bin/sleep 0.3; kill -USR1 0;
// wait $!` is 158 for the child in all five. What the kernel cannot be left to
// do is tell the shell. Its copy comes back through os/signal on a goroutine of
// its own, so the handler ran whenever that goroutine was scheduled: a command
// late, and under load not at all, which is the race selfSignaled was written
// to take out of the picture (#7043). Measured 2026-10-31, each shell in a
// process group of its own, against bash 5.3.20, bash 3.2.57, ksh93u+
// 2012-08-01, zsh 5.9.2 and dash:
//
//	trap 'echo T' USR1; kill -USR1 0;   echo after   T, after in all five
//	trap 'echo T' USR1; kill -USR1 -$$; echo after   T, after in all five
//	…the same with HUP and TERM trapped               the handler first, likewise
//	this shell, until this change                     after, then T — or no T
//
// So the shell's own arrival is recorded where the send succeeds, through the
// same selfSignaled a `kill $$` reaches, and the runtime's copy is dropped when
// it arrives — see signalState.groupCopies. A shell cannot tell which of the
// two a signal came from, and neither do the references: the rows a `kill $$`
// was measured on hold for the group too, which is why the same two questions
// are asked of it. A `CHLD` runs the handler only where the dialect's
// self-aimed one does (dash writes `C` for `kill -CHLD 0` and nobody else
// does), and a `WINCH` waits for input where the dialect's does (zsh writes no
// `W` for `kill -WINCH 0` under `-c`, and everybody else writes it first).
//
// Only a trapped signal with a handler, at the top level. An ignored signal is
// ignored on arrival and a fatal or stopping one acts on the whole group, the
// shell included, as the kernel delivers it — which is what every reference
// does, and agreed already. A subshell sending it is the other half of the
// rule, and has two copies to answer rather than one: see
// signalTheGroupFromASubshell.
func (r *Runner) signalTheShellsOwnGroup(pid int, name string, sig syscall.Signal) error {
	key := trapKey(name, sig)
	// The name the runtime's copy will be read back under, which is the one
	// signalState.forwarded computes — a nameless number's decimal included.
	copyName, named := signalName(sig)
	s := r.sigs()
	s.mu.Lock()
	body, trapped := s.traps[key]
	answered := trapped && body != "" && named && catchableSignal(sig)
	if answered {
		// Before the send rather than after it. The kernel may hand the copy
		// over before kill(2) has even returned, and another goroutine's
		// `wait` reads the same channel; a credit given afterwards could
		// arrive behind the copy it was meant to cover.
		if s.groupCopies == nil {
			s.groupCopies = map[string]int{}
		}
		s.groupCopies[copyName]++
	}
	s.mu.Unlock()
	if err := r.killProcess(pid, sig); err != nil {
		if answered {
			// Nothing was sent — the gate refused it, or the kernel did — so
			// no copy is coming for the credit to cover.
			s.mu.Lock()
			s.groupCopies[copyName]--
			s.mu.Unlock()
		}
		return err
	}
	if r.inSubshell {
		r.signalTheGroupFromASubshell(name, sig, trapped)
		return nil
	}
	if answered && r.selfAimedSignalIsTheCondition(key) {
		r.selfSignaled(key)
	}
	return nil
}

// A subshell that signals **the process group it shares with the shell** —
// `( kill -USR1 0 )`, or `-$$` where the shell leads its group — is a member
// of that group itself, so the signal reaches the subshell as well as the
// shell above it. Each answers its own copy from its own table: the subshell
// has the shell's handled signals back at their defaults, so it dies of one
// the shell traps, and the shell runs its handler once the subshell is over.
// Measured 2026-11-01, each shell in a process group of its own, against bash
// 5.3.20, bash 3.2.57, ksh93u+ 2012-08-01, zsh 5.9.2 and dash:
//
//	trap 'echo T' USR1; (kill -USR1 0; echo in); echo out=$?
//	        T, out=158 in all five (ksh93 out=286); bash and dash write the
//	        death notice first; here, before: in, then T and out=0 in
//	        either order
//	…the same with `kill -USR1 -$$`                     the same
//	(kill -USR1 0; echo in); echo out=$?                nothing, and 158:
//	        the whole group dies; here, before: in, and a race
//	trap 'echo T' USR1; (trap 'echo S' USR1; kill -USR1 0; echo in)
//	        S, in, T, out=0 — the subshell's own handler, and it runs on
//	        (ksh93 crashes); here, before: in, out=0, and T or not
//	trap 'echo T' USR1; { kill -USR1 0; echo in; }      T, in: a group is
//	        the shell itself, and was already right
//
// So the shell's copy is answered as `kill -SIG $$` from the subshell is
// answered — a handler queued for the shell, a death recorded for it — and the
// runtime's copy is dropped by the credit taken above, so the handler runs
// once and before the next command rather than whenever os/signal is
// scheduled. The subshell's copy is answered as `kill -SIG $BASHPID` is, by
// signalThisBody against the subshell's own table. Not a dialect split: the
// one column whose answer differs is ksh93, whose `( … )` runs in the shell's
// own process, so the two copies are one and the axes that already say so —
// Semantics.SubshellEndsWhenItSendsTheShellATrappedSignal and
// SubshellRunsOnAfterSignalingTheShell — end the subshell at the shell's
// answer, and there is no second copy left to settle.
//
// An untrapped stop is the shell's copy left to the kernel, as at the top
// level: it stops the process, which is every member here, and a second stop
// raised beside it would stop the shell again once something had continued it.
//
// **And every body around the subshell has a copy too** (#7245): the
// parentheses, substitutions and pipeline elements a real shell forked on
// the way down to it are members of the same group, with the same defaults.
// Measured 2026-11-01 the same way:
//
//	trap 'echo T' USR1; ( (kill -USR1 0; echo in2); echo in1 ); echo out=$?
//	        T, out=158 in all five (ksh93 out=286), and the same with a
//	        third level; bash 5.3 writes its one notice bare, for the outer
//	        parentheses; here, before: in1, T, out=0
//	trap 'echo T' USR1; ( trap 'echo S' USR1; (kill -USR1 0; echo in2); echo in1 )
//	        S, in1, T, out=0 — the body that traps it runs on; here,
//	        before: in1, T
//	trap 'echo T' USR1; : | { (kill -USR1 0; echo in2); echo in1; }
//	        T, out=158 where the element is forked (bash, dash); T, in1,
//	        out=0 where it is the shell (zsh, ksh93)
//	( (kill -USR1 0; echo in2); echo in1 ); echo out=$?
//	        nothing, killed by USR1; here, before: in1
//
// So each running body around the sender is left the signal as a copy of it
// leaves one at the body's own number, and settles it the same way, through
// signalThisBody at its next moment: innermost outward, because each is
// waiting for the one inside it. A body that dies of it says nothing about
// the child that died with it — see the parentheses in compound.go — so the
// one notice is the shell's. The walk is over the bodies' processes and not
// their anchors, which exist only where a front end supplied ProcessAnchor.

// signalTheGroupFromASubshell answers the shell's copy, the subshell's own
// copy and the copy of every body around it, of a signal the subshell has
// just sent the group they share. trapped is whether the shell has an entry
// for it.
func (r *Runner) signalTheGroupFromASubshell(name string, sig syscall.Signal, trapped bool) {
	was := r.ctl
	if trapped || !signalStops(sig) {
		_ = r.sendSignal(os.Getpid(), name, sig)
	}
	if r.ctl != was {
		// The shell's copy was this subshell's own, in the dialect that runs
		// it in the shell's process, and it has ended it.
		return
	}
	_ = r.signalThisBody(name, sig)
	// Not reached in the dialect whose `( … )` is the shell's own process:
	// that copy has ended the subshell above, and the rule that did it ends
	// every unforked subshell around it as well.
	for p := range r.bodiesAround {
		r.signalTheBodyAround(p, name, sig)
	}
}

// dyingOfASubshellsSignal reports that a subshell has sent the shell a fatal
// signal it has still to die of.
func (r *Runner) dyingOfASubshellsSignal() bool {
	s := r.signals
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.diedSig != 0
}

// aimedAtTheShellsOwnGroup reports whether a `kill` operand names the process
// group this shell is in: 0, which always does, or the negative of the
// group's own number.
//
// Not -1. That is every process the sender may signal, and on both platforms
// this builds for kill(2) leaves the sender itself out of it, so the shell
// has no copy coming.
func aimedAtTheShellsOwnGroup(pid int) bool {
	switch {
	case pid == 0:
		return true
	case pid < -1:
		return -pid == syscall.Getpgrp()
	default:
		return false
	}
}
