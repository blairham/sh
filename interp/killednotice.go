// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"os/exec"
	"strconv"
	"syscall"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// quietSignal is a death no shell in the panel remarks on.
//
// Two of them, and both for the same reason: they are how a command is
// *supposed* to end. ^C is how a person stops something they started, and a
// broken pipe is how `yes | head` ends the thing writing into it. A shell
// that announced these would be announcing them constantly, and all four
// stay quiet. Every other signal is reported by the three that report at all.
func quietSignal(sig syscall.Signal) bool {
	return sig == syscall.SIGINT || sig == syscall.SIGPIPE
}

// reportKilled says that a signal ended a command, which is the only way a
// script hears about it: the status carries the number but nothing in the
// output says a signal was involved at all.
//
// Found by the run sweep on /usr/bin/wish, which the harness's timeout kills.
// Every shell in the panel but zsh said so and we said nothing, with output
// otherwise identical to the byte — so only a comparison that reads stderr
// could see it.
func (r *Runner) reportKilled(sig syscall.Signal, pid int) {
	if sig == syscall.SIGINT {
		// ^C reaching a child is the one death that ends the script in one
		// dialect. Asked before the quiet signals below, because an
		// interrupt is one of them: the shell that stops here says nothing
		// while doing it.
		status := r.status
		if r.ask(r.sem().ChildInterruptEndsTheScript, "an interrupt that ended a child ending the script") {
			// Not an exit with 130: the shell that does this *dies of the
			// interrupt itself*, so a parent sees a process killed by a
			// signal rather than one that exited. The golden record is what
			// said so — it recorded an exit code of -1, which is what
			// os/exec reports for a process a signal ended, where an exit
			// with 130 would have recorded 130.
			//
			// The same road `kill` takes when a script signals its own
			// shell: the status is set here because the driver may not get
			// the chance, and the dying is the driver's to do.
			r.signalDeath("INT", sig)
			return
		}
		if r.unspecified {
			r.status, r.unspecified = status, false
		}
	}
	if quietSignal(sig) {
		return
	}
	status := r.status
	if r.midPipeline && !r.ask(r.sem().ReportsAnyKilledPipelineElement, "a signal ending a pipeline element that is not the last") {
		// Asked before the question of whether this shell reports at all,
		// because a shell that says nothing never reaches either — and the
		// refusal must not become the status here for the same reason it
		// must not below.
		r.status, r.unspecified = status, false
		return
	}
	if !r.ask(r.sem().ReportsACommandKilledBySignal, "a command killed by a signal being reported") {
		// A refusal here is a refusal to *say* something, and the status is
		// not this question's to touch: `$?` after a killed command is what
		// the signal made it whether or not the shell remarked on it. The
		// axis has already been named on stderr by the asking.
		r.status, r.unspecified = status, false
		return
	}
	dg := r.diag()
	if sig == syscall.SIGTERM && dg.KilledCommandNoticeBareForTerminate != "" {
		// One signal, in one shell, written with neither the location nor
		// the process id — just the words and the command.
		//
		// Almost certainly a regression rather than a decision: bash 3.2
		// writes the full prefix for SIGTERM as it does for every other
		// signal, and no other shell in the panel treats it apart. It is
		// reproduced because this dialect is bash 5.3 and that is what bash
		// 5.3 does; if it is fixed upstream the drift check is what will
		// say so.
		r.errf("%s\n", Wording(dg.KilledCommandNoticeBareForTerminate, "%-27[1]s%[2]s",
			r.signalDescription(sig), r.killedCommandText()))
		return
	}
	// Three verbs, and the dialects use one, two and all three of them.
	notice := Wording(dg.KilledCommandNotice, "%5[1]d %-27[2]s%[3]s",
		pid, r.signalDescription(sig), r.killedCommandText())
	if dg.KilledCommandNoticeUnprefixed {
		r.errf("%s\n", notice)
		return
	}
	r.diagf("%s\n", notice)
}

// killedCommandText is the command written back out.
//
// Printed from the tree rather than quoted from the source, which is what the
// one shell that shows it does: `  cmd   x  >/dev/null  # note` comes back as
// `cmd x > /dev/null`, with the runs of spaces collapsed, the space put in
// after the operator and the comment gone. So this needs no record of the
// text — Stmt.Text is kept only for a background statement, and this is the
// evidence that the narrowness is right rather than an oversight.
func (r *Runner) killedCommandText() string {
	if r.killed == nil {
		return ""
	}
	return syntax.PrintCommand(r.killed)
}

// signalDescription is the shell's words for the signal.
//
// Two sources, because ksh93 carries its own table and says `Memory fault`
// where the machine says `Segmentation fault`. The other two that report use
// what the host calls it, which is why this is not one table written here.
func (r *Runner) signalDescription(sig syscall.Signal) string {
	if own, ok := r.diag().SignalDescriptions[sig]; ok {
		return own
	}
	return hostSignalDescription(sig)
}

// hostSignalDescription is what this machine calls a signal.
//
// The Go runtime's own table is the machine's: measured against every signal
// this shell could be made to report, its words are the platform's words with
// a small letter at the front — `segmentation fault` where the C library says
// `Segmentation fault`, on all nineteen. So this is that table with the first
// letter raised, rather than a copy of it written out here that would go
// stale against the host it is supposed to describe.
func hostSignalDescription(sig syscall.Signal) string {
	name := sig.String()
	if r, size := utf8.DecodeRuneInString(name); size > 0 {
		name = string(unicode.ToUpper(r)) + name[size:]
	}
	if !signalDescriptionCarriesItsNumber {
		return name
	}
	return name + ": " + strconv.Itoa(int(sig))
}

// killedBy reports the signal that ended a command, from the error a plain
// wait returns.
//
// The other path through here — a shell watching its own children, which is
// what job control needs — is told the signal directly and never sees an
// error at all.
func killedBy(err error) (syscall.Signal, bool) {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return 0, false
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return 0, false
	}
	return ws.Signal(), true
}
