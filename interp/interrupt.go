// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "syscall"

// Interrupting what the shell is running.
//
// A shell at a prompt runs two quite different things, and ^C has to reach
// both. An external command is a process of its own, in a process group the
// terminal has been handed to, so the interrupt goes to *it* and the shell
// learns about it from the wait. A loop, an `if`, a function body — anything
// the shell executes itself — is not a process at all: the shell holds the
// terminal while it runs, the interrupt arrives as a signal at the shell, and
// only the front end that installed a handler can see it.
//
// Both have to end the same way, and until this existed neither did. `while :;
// do echo tick; sleep 1; done` typed at the prompt kept ticking through ^C and
// through ^Z: the `sleep` died or stopped, the loop went round again, and the
// only way out was to close the terminal. That is the worst failure a daily
// driver can have, because the recovery costs the session.
//
// Measured through a pseudo-terminal on 2026-09-05, and unanimous between bash
// 5.3.15 and zsh 5.9.2: an interrupt gives up **the whole typed line** —
// a loop, an `if`, a function body and the commands after it on the same line
// — and reports 128 plus SIGINT. `sleep 10; echo after` interrupted never
// prints `after` in either shell.
//
// A stop is the other half and is not the same rule. Measured in bash: ^Z
// during `sleep 10; echo after` prints `after`, so a stop does not give up the
// line — and ^Z inside a loop *does* end the loop, with the commands after it
// still running. So a stop breaks out of the loops it is inside and nothing
// else. zsh answers this one completely differently: it suspends the enclosing
// construct itself as a job, which needs a fork this engine does not have, and
// is recorded in docs/spec/semantics.md rather than approximated here.

// takeInterrupt asks the front end whether the person has interrupted the
// shell since it was last asked, and gives up the line if so.
//
// The question is the front end's because the answer is: interp installs no
// signal handlers — a library that did would be taking them from the program
// around it — so a signal aimed at this process is something only the binary
// can hear. See Runner.TakeInterrupt.
//
// Asked at the top of every command, which is the only place a loop of
// builtins can be stopped: nothing else in `while :; do echo tick; done` ever
// blocks or returns to the interpreter.
func (r *Runner) takeInterrupt() bool {
	if r.TakeInterrupt == nil || !r.Interactive || r.bg != nil {
		// A background job runs on a clone of this Runner and would otherwise
		// race the foreground for the same answer, taking an interrupt meant
		// for the command the person is watching.
		return false
	}
	if !r.TakeInterrupt() {
		return false
	}
	r.abandonForInterrupt()
	return true
}

// abandonForInterrupt gives up the line the shell is running because it was
// interrupted.
//
// controlAbandon is exactly the shape measured: it unwinds past loops,
// functions, groups and subshells alike, and is consumed at the top-level
// statement loop, which also drops the rest of the line it was on. That is
// `sleep 10; echo after` not printing `after`, and it is the same mechanism a
// refused assignment to a readonly name already uses.
func (r *Runner) abandonForInterrupt() {
	r.diedOfSig = syscall.SIGINT
	r.status = r.signalDeathStatus(syscall.SIGINT)
	r.ctl, r.abandonLine = controlAbandon, r.line
}

// breakLoopsForAStop ends the loops the stopped command was inside.
//
// Measured in bash: ^Z inside `while :; do echo tick; sleep 1; done; echo
// after` stops the `sleep`, ends the loop and prints `after`. So it is a
// break and not a give-up — the line goes on — and it reaches every enclosing
// loop, which is what the recorded depth is for. Outside a loop it is nothing
// at all: `sleep 10; echo after` suspended prints `after`, so a stop on its
// own does not interrupt anything.
//
// Reusing `break`'s own machinery rather than a control value of its own: the
// count is exactly what `break 2` means, and loopControl already unwinds it
// one loop at a time and clears it at the outermost.
func (r *Runner) breakLoopsForAStop() {
	if !r.Interactive || r.loopDepth == 0 {
		return
	}
	r.ctl, r.ctlDepth = controlBreak, r.loopDepth
}

// enteringLoop records that a loop's body is running, and hands back what ends
// it. The depth is dynamic — a loop that calls a function that loops is two —
// because what a stop has to break out of is what it is *inside* at the time.
func (r *Runner) enteringLoop() func() {
	r.loopDepth++
	return func() { r.loopDepth-- }
}
