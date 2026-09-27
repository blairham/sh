// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// `$signals`: the names this shell's `trap` takes, as an array a script can
// walk.
//
// It had no parameter here at all — `${+signals}` was 0 — so a script that
// installs a handler by iterating the roster iterated nothing. Split out of
// #4866's ledger as #4906, and the one name of that ledger's remainder whose
// **answer this shell already held**: the middle of the array is the bare
// `kill -l` roster, and measured 2026-09-27 in one run this shell's is the
// reference's byte for byte —
//
//	HUP INT QUIT ILL TRAP ABRT EMT FPE KILL BUS SEGV SYS PIPE ALRM TERM URG
//	STOP TSTP CONT CHLD TTIN TTOU IO XCPU XFSZ VTALRM PROF WINCH INFO USR1 USR2
//
// on both sides. So what was missing was a **name for a roster that was
// already right**, which is why this is a parameter and not a second table.
// [interp.Runner.SignalNames] is the accessor that keeps it one table; a
// literal list beside `kill -l` is exactly the shape this tree keeps finding
// as two copies that drift.
//
// # The three parts, measured
//
// 2026-09-27 against `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, `-f` from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME:
//
//	${(t)signals}    array
//	${#signals}      34
//	$signals[1]      EXIT
//	$signals[2]      HUP      — signal 1
//	$signals[32]     USR2     — signal 31, the last the kernel here takes
//	$signals[33]     ZERR
//	$signals[34]     DEBUG
//	typeset -p       typeset -a signals=( EXIT HUP … USR2 ZERR DEBUG )
//
// `EXIT` first, the kernel's signals in the kernel's numbering, then the two
// **pseudo-signals** — neither of which is a kernel signal and both of which
// this shell's `trap` already takes: measured in the same run,
// `trap 'print z' ZERR; false` writes `z` here and there alike. The core may
// name a platform fact and may not name a shell, so those two are added here
// rather than by the accessor.
//
// # The type word says it is an ordinary array
//
// `array`, with neither `special` nor `readonly` — so this is *not* the
// produced-and-frozen seam `$ARGC`, `$status` and `$PPID` use. It is a plain
// array laid down at startup, and a script that writes over it keeps what it
// wrote: measured, `signals=(x y)` is accepted and `${(t)signals}` is still
// `array` afterwards. That is also why it takes no
// [interp.Runner.MarkShellOwnParameter] — the word the reference writes has
// no `special` in it.
//
// # A test asserting this platform's list would pass on one runner and fail
// on the other
//
// The roster is the machine's: this kernel takes 31 signals and names every
// one of them, and the Linux runner's does neither. The assertion that is
// true on both is the one the paragraph above makes — grade `$signals`
// against **the same shell's own `kill -l`, in the same run** — which is also
// the check that would catch the two rosters drifting apart, and which a
// literal would not.
func registerTheSignalNames(r *interp.Runner) {
	names := r.SignalNames()
	signals := make([]string, 0, len(names)+3)
	// `EXIT` is position one and is not a signal: `trap … EXIT` is the shell
	// ending, so the kernel's numbering starts at position two and
	// `$signals[n+1]` is signal `n` wherever this kernel names every number
	// it takes. See interp.Runner.SignalNames, which does not promise that
	// and says why.
	signals = append(signals, "EXIT")
	signals = append(signals, names...)
	// The two this shell traps that no kernel sends: a command's failure and
	// every command. They are the dialect's because the core's table is the
	// platform's.
	signals = append(signals, "ZERR", "DEBUG")
	r.SetArray("signals", signals)
}
