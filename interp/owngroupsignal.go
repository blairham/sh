// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

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
// Only a trapped signal with a handler, and only at the top level. An ignored
// signal is ignored on arrival and a fatal or stopping one acts on the whole
// group, the shell included, as the kernel delivers it — which is what every
// reference does, and agreed already. A subshell is a pretend child that
// shares the process, so its copy is the top-level shell's to have and is left
// to arrive as it always did.
func (r *Runner) signalTheShellsOwnGroup(pid int, name string, sig syscall.Signal) error {
	if r.inSubshell {
		return r.killProcess(pid, sig)
	}
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
	err := r.killProcess(pid, sig)
	if !answered {
		return err
	}
	if err != nil {
		// Nothing was sent — the gate refused it, or the kernel did — so no
		// copy is coming for the credit to cover.
		s.mu.Lock()
		s.groupCopies[copyName]--
		s.mu.Unlock()
		return err
	}
	if r.selfAimedSignalIsTheCondition(key) {
		r.selfSignaled(key)
	}
	return nil
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
