// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// A signal waiting with a handler, whose handler an earlier one of the same
// batch takes away before its turn comes (#7170).
//
// Two trapped signals that arrive while one command runs wait for it to end
// and then run one after another, in the order Semantics.WaitingHandlersRun
// gives them. The first to run can change the second's trap. A replacement
// is read at the second's turn and runs, in every column. A trap that is gone
// splits them. Measured 2026-11-01 under `-c`, the two signals sent by `{
// kill -USR2 $$; kill -HUP $$; } | /bin/cat` and HUP's handler written `echo
// HUP; <change>` (darwin numbers: USR2 31, URG 16, WINCH 28):
//
//	change                bash 5.3.20 and 3.2.57                    dash, ash
//	trap - USR2           HUP, two warnings, dies of USR2 (159)     HUP after
//	trap '' USR2          HUP, the first warning, after             HUP after
//	trap "echo NEW" USR2  HUP NEW after                             HUP NEW after
//	trap - WINCH          HUP, two warnings, after                  HUP after
//	trap - URG            HUP, two warnings, after                  HUP after
//
// with WINCH and URG trapped and sent in USR2's place. The warnings are
// Diagnostics.WaitingSignalLostItsTrap and WaitingSignalResent — `bad value
// in trap_list[31]: 0x0` and `signal handler is SIG_DFL, resending 31
// (SIGUSR2) to myself` — and the second is true to its word: what follows is
// whatever the signal does to a shell with no trap for it. USR2 kills it,
// with an EXIT trap run first (`trap 'echo "bye $?"' EXIT` reads the status
// the batch waited on, `bye 5` after `(/bin/cat; exit 5)`); WINCH and URG do
// nothing; QUIT, which bash ignores untrapped when not interactive, warns
// twice and goes on; TERM, PIPE and ALRM die. bash 5.3 under the name `sh`
// is bash.
//
// ksh93u+ and zsh 5.9.2 run nothing for it either, reached in the order each
// gives: for ksh93 USR2's handler resets HUP's (it runs USR2 first, `USR2
// after`, with the signals sent a tenth of a second apart during a
// `/bin/sleep`), and for zsh HUP's resets USR2's where HUP came first (`HUP
// after`, three runs).
//
// Two signals are not resent in bash, because bash keeps a handler of its own
// for them: INT runs nothing and says nothing, after `trap -` and `trap ''`
// alike; and CHLD ends bash 5.3 with a segmentation fault (139) either way,
// which is not modeled — nothing runs for it, which is every other column.
//
// Only a condition's first turn in the batch is asked about. One whose own
// handler resets its trap, and which arrived twice, runs once and goes on in
// bash as everywhere: `trap 'echo U; trap - USR2' USR2` with USR2 sent twice
// is `U after` in all of them.

// handledAsTheBatchIsTaken is the set of conditions in a batch with a handler
// that runs something, as the batch is taken: what a later turn holds to it.
// Nil for a batch of one, where no earlier handler can take anything away.
func (r *Runner) handledAsTheBatchIsTaken(batch []string) map[string]bool {
	if len(batch) < 2 {
		return nil
	}
	s := r.sigs()
	s.mu.Lock()
	defer s.mu.Unlock()
	var had map[string]bool
	for _, name := range batch {
		if s.traps[name] == "" {
			continue
		}
		if had == nil {
			had = map[string]bool{}
		}
		had[name] = true
	}
	return had
}

// waitingSignalLostItsTrap answers the turn of a condition that had a handler
// as its batch was taken and has none now — ignored says whether its trap now
// ignores it rather than being gone — and reports whether the shell is
// stopping. See Semantics.WaitingSignalWithoutItsTrapIsResent.
//
// Asked here and nowhere else: where the condition still has a handler, or
// never had one, every column runs what is there.
func (r *Runner) waitingSignalLostItsTrap(name string, ignored bool) bool {
	sig := trapConditionSignal(name)
	if sig == 0 || name == "INT" || name == "CHLD" {
		// Nothing, in every column. See the top of this file.
		return false
	}
	if !r.ask(r.sem().WaitingSignalWithoutItsTrapIsResent,
		"what a waiting signal does once an earlier handler took its trap away") {
		return false
	}
	d := r.diag()
	disposition := 0
	if ignored {
		disposition = 1
	}
	if w := d.WaitingSignalLostItsTrap; w != "" {
		r.diagf("%s\n", Wording(w, "", int(sig), disposition))
	}
	if ignored {
		return false
	}
	if w := d.WaitingSignalResent; w != "" {
		r.diagf("%s\n", Wording(w, "", int(sig), name))
	}
	return r.resendWithNoTrap(name, sig)
}

// resendWithNoTrap is what a signal the shell at the top sends itself with no
// trap for it does, taken where the shell stands, and reports whether the
// shell is stopping: a fatal signal it does not ignore untrapped ends it, a
// stopping one stops the process, and any other does nothing. It is the
// kill builtin's road for a signal nobody trapped, without what `kill` owes
// the operands after `$$` — there are none — so the death is taken here, as a
// held one is at the next command, and the EXIT trap reads the status the
// batch waited on.
func (r *Runner) resendWithNoTrap(name string, sig syscall.Signal) bool {
	switch {
	case fatalSignal(name, sig) && !r.untrappedSignalIgnored(name):
		shown := r.status
		r.signalDeath(name, sig)
		r.showTheExitTrap(shown, name, sig)
		return r.ctl != controlNone
	case signalStops(sig) && r.StopThisProcess != nil:
		_ = r.StopThisProcess(sig)
	}
	return false
}
