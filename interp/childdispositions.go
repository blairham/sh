// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/blairham/sh/internal/sigbook"
)

// What a command this shell starts inherits of its signal dispositions.
//
// A child is born with one thing from its parent's signal handling, and it is
// the ignores: exec resets every handled signal to its default and leaves an
// ignored one ignored. A real shell gets the rest right by forking — what a
// subshell or a background job sets, it sets in a process of its own, and
// that process's children inherit exactly that. A Runner has no fork of its
// own: a subshell is a clone and a background body is a goroutine, and the
// only process is the one every Runner in the program shares. So a child here
// inherits whatever the *process* holds at the moment it is started, and that
// is the right answer only where the shell at the top is the one asking.
//
// The dispositions are therefore set where a fork would inherit them: on the
// process, for the length of each start. That is process state, which this
// package may not change on its own say-so, so the change is the front end's
// hook, StartWithDispositions, and where it is nil the child starts with the
// signals the process has. Every measurement below is unanimous across the
// panel — bash 5.3.20, bash 3.2.57, ksh93u+ and zsh 5.9.2 and dash — so none
// of it is an axis:
//
//   - **An asynchronous list in a shell without job control** runs with INT
//     and QUIT ignored, which is the standard's rule. Measured 2026-10-02
//     with `sleep 1 & sleep 0.2; kill -SIG $!; sleep 0.2; kill -0 $!` under
//     `-c`: every shell leaves the job alive for both, and `jobs` in bash
//     still lists it Running (#5414). The body's own handling is not this: a
//     signal aimed at a background body reaches its inbox, which INT and QUIT
//     never end — see endsABodyByDefault.
//   - **A subshell's `trap ''`** reaches the commands it starts. Measured
//     2026-10-30, `(trap '' INT; /bin/sh -c 'kill -INT $$; echo survived';
//     echo st=$?); echo out` prints `survived`, `st=0` and `out` in every
//     shell, and the same with USR1; here the child was killed, because the
//     subshell's ignore was in its own table and nowhere a child could see it
//     (#7049).
//   - **A subshell's `trap -` or handler** gives its commands the default
//     back, under a shell that ignores the signal. Measured the same day,
//     `trap '' USR1; (trap - USR1; /bin/sh -c 'kill -USR1 $$; echo
//     survived'; echo st=$?)` has the child killed in every shell — 158, and
//     ksh93's 286 — and so does the same with `trap 'echo h' USR1` inside
//     the parentheses. Here the child survived, on the process's ignore.
//   - **`exec` hands its ignores to the program that replaces the shell**,
//     which is `nohup` spelled in shell. Measured 2026-10-30, `trap '' INT;
//     exec /bin/sh -c 'kill -INT $$; echo survived'` prints `survived` in
//     every shell, and the same with USR1; here the replacement was killed,
//     because the shell's end hands the process back the dispositions it
//     borrowed just before the execve (#7050). The tail of a `-c` string the
//     shell becomes rather than forks is the same road.
//   - **A shebang-less script's ignore reaches its commands, and nothing
//     else.** It is a second Runner in this process, and it used to be handed
//     SetSignalDisposition so that its commands would inherit — which made
//     its ignore the process's for the length of the script, so a signal that
//     arrived meanwhile was dropped by the kernel before the caller's trap
//     could hear it. Measured 2026-10-30, a script holding `trap '' USR1;
//     /bin/sleep 1; echo inner`, run under `trap 'echo caught' USR1` with a
//     USR1 from another process 0.3s in: `inner`, `caught`, `after` in every
//     shell, and no `caught` here (#7051). It keeps its ignore to itself now,
//     and its commands inherit it through the window like a subshell's.
//
// # The window, and what it costs
//
// The window is as short as the start: the umask goes onto the process in the
// same way (see holdMaskForFork), and every start takes spawnDispositions so
// that nothing started beside one inherits what it put there.
//
// What it costs is every subscription the process had for a signal it
// ignores: signal.Ignore is the only way to make one inheritable, and it
// takes them all. So the end of the window puts back what this module had —
// every Runner's own subscriptions and the interactive session's — through
// internal/sigbook, which is the book of who asked for what; the hook puts
// back the front end's own. An arrival inside the window is dropped, which is
// what a real shell's child-to-be would have done with it too.
//
// # What is not handed over
//
// A signal the front end cannot take away again cleanly is left as the
// process has it, and the hook says which: see the driver's
// startWithDispositions.

// spawnDispositions orders the starts that change the process's dispositions
// against every other start: a program started in the window would inherit
// the ignore. A start that changes nothing shares the lock; one that changes
// something takes it alone.
var spawnDispositions sync.RWMutex

// startWithDispositions runs start — a process start, or a replacement of
// the process — with the dispositions this runner's children are owed.
func (r *Runner) startWithDispositions(start func() error) error {
	var ignore, reset []syscall.Signal
	if r.StartWithDispositions != nil {
		ignore, reset = r.owedDispositions()
	}
	if len(ignore) == 0 && len(reset) == 0 {
		spawnDispositions.RLock()
		defer spawnDispositions.RUnlock()
		return start()
	}
	spawnDispositions.Lock()
	defer spawnDispositions.Unlock()
	// Asked under the lock, because a window beside this one is exactly a
	// moment the process holds an ignore it is about to give back.
	if !processDiffers(ignore, reset) {
		return start()
	}
	err := r.StartWithDispositions(ignore, reset, start)
	// The ignore undid every subscription the process had for those, and the
	// ones this module made go back now. A reset needs nothing put back: the
	// process was ignoring the signal before the window and ignores it again
	// after, and an ignored signal has no subscribers to lose.
	back := make([]os.Signal, len(ignore))
	for i, sig := range ignore {
		back[i] = sig
	}
	sigbook.PutBack(back...)
	return err
}

// processDiffers reports whether the process is not already holding what a
// child is owed: a signal to ignore that it handles, or a signal to reset
// that it ignores.
func processDiffers(ignore, reset []syscall.Signal) bool {
	for _, sig := range ignore {
		if !signal.Ignored(sig) {
			return true
		}
	}
	for _, sig := range reset {
		if signal.Ignored(sig) {
			return true
		}
	}
	return false
}

// owedDispositions is what a command this runner starts is owed: the signals
// it begins ignoring, and the signals it begins at their default although the
// process may be ignoring them.
//
// The ignores are every entry with an empty action in the table this runner
// answers for — its own in a subshell, the shared one at the top — and an
// asynchronous list's INT and QUIT. For the shell at the top of a binary these
// are already the process's, and the hook finds nothing to change.
//
// The resets are for a subshell alone: a signal the shell at the top ignores,
// which this subshell's table does not. A subshell inherits every ignore at
// its entry (see inheritTraps), so one that is missing was taken off by the
// subshell or by one between it and the top — or the subshell was started
// before the shell ignored the signal, which is the same answer from the other
// side: a forked job keeps the dispositions it was forked with. Only an ignore
// the shell itself put on the process is reset; one the process was born with
// is not this shell's to give back, and a borrow ledger that says so is what
// tells the two apart.
func (r *Runner) owedDispositions() (ignore, reset []syscall.Signal) {
	if r.asyncIgnoresInterrupts {
		ignore = append(ignore, syscall.SIGINT, syscall.SIGQUIT)
	}
	s := r.signals
	if s == nil {
		return ignore, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	table := r.traps
	if table == nil {
		table = s.traps
	}
	for name, action := range table {
		if action != "" {
			continue
		}
		if sig := trapConditionSignal(name); sig != 0 && catchableSignal(sig) {
			ignore = append(ignore, sig)
		}
	}
	if r.traps == nil {
		return ignore, nil
	}
	for name, action := range s.traps {
		if action != "" {
			continue
		}
		if own, ok := r.traps[name]; ok && own == "" {
			continue
		}
		sig := trapConditionSignal(name)
		if sig == 0 || !catchableSignal(sig) {
			continue
		}
		if wasIgnored, borrowed := s.borrowed[sig]; !borrowed || wasIgnored {
			continue
		}
		if r.asyncIgnoresInterrupts && (sig == syscall.SIGINT || sig == syscall.SIGQUIT) {
			// The list's own rule wins over the subshell's table: the two
			// keyboard signals are ignored for what it starts either way.
			continue
		}
		reset = append(reset, sig)
	}
	return ignore, reset
}
