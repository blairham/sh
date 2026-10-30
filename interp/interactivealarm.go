// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path"
	"syscall"
)

// InteractiveAlarmAnswer is what an untrapped SIGALRM does to an interactive
// shell. See Semantics.InteractiveAlarm.
type InteractiveAlarmAnswer int

const (
	// InteractiveAlarmUnspecified is no answer, and is read as the default
	// action, which is what this package did before the question was asked.
	InteractiveAlarmUnspecified InteractiveAlarmAnswer = iota
	// InteractiveAlarmKills is bash, dash and BusyBox ash: the shell dies of
	// it, as a shell that is not interactive does in every column.
	InteractiveAlarmKills
	// InteractiveAlarmIsIgnored is ksh93: nothing happens at all, and that
	// holds inside `( … )` too.
	InteractiveAlarmIsIgnored
	// InteractiveAlarmTimesOut is zsh: the shell says `timeout` and exits at
	// the signal's number, running its EXIT trap on the way, until a
	// `trap - ALRM` hands the signal back to its default action.
	InteractiveAlarmTimesOut
)

func (a InteractiveAlarmAnswer) String() string {
	switch a {
	case InteractiveAlarmKills:
		return "kills"
	case InteractiveAlarmIsIgnored:
		return "is ignored"
	case InteractiveAlarmTimesOut:
		return "times out"
	}
	return "unspecified"
}

// alarmTimesOut reports whether an untrapped SIGALRM ends this shell as a
// timeout rather than as a death: interactive, in the dialect that answers
// so, and with nothing having reset the signal since.
func (r *Runner) alarmTimesOut() bool {
	return r.Interactive && r.sem().InteractiveAlarm == InteractiveAlarmTimesOut &&
		!r.defaultRestored("ALRM")
}

// alarmTimeout ends the shell the way an interactive zsh ends on an alarm it
// has no trap for: it says so, located as any other diagnostic is, and leaves
// at the signal's number as `exit` would, so the EXIT trap still runs and an
// `exit` inside it still names the status. See Semantics.InteractiveAlarm.
func (r *Runner) alarmTimeout(sig syscall.Signal) {
	// The shell's own sentence and not the builtin's that sent the signal:
	// measured, `kill -ALRM $$` writes `zsh:1: timeout` and not a `kill:`
	// beside the line, the same as a signal from outside.
	inBuiltin := r.inBuiltin
	r.inBuiltin = ""
	r.diagf("timeout\n")
	r.inBuiltin = inBuiltin
	r.status = int(sig)
	r.captureLeavingContexts(false)
	r.stopTheShell()
}

// TimesOutOnAnAlarm hands a front end that owns the process the question an
// untrapped SIGALRM from another process asks: does this shell end on it as a
// timeout, and if so, with what to say and what status. The shape is
// UntrappedHangupExit's and for its reason — it is asked from a goroutine of
// the front end's own, so it closes over the answers as they stood when it was
// taken and over the shared signal state, never over the Runner's fields. Take
// it again once Interactive is set.
//
// Read and not asked: an unanswered axis reads as no, which leaves the signal
// to end the shell by its default action as before.
//
// What it says is located by the shell's name alone, which is what zsh writes
// at a prompt — `zsh: timeout` — and the front end ends the process itself:
// a timeout from outside runs no EXIT trap here, where zsh runs one.
func (r *Runner) TimesOutOnAnAlarm() func() (say string, status int, timesOut bool) {
	s := r.sigs()
	timesOut := r.Interactive && r.sem().InteractiveAlarm == InteractiveAlarmTimesOut
	say := path.Base(r.invokedAs()) + ": timeout\n"
	return func() (string, int, bool) {
		if !timesOut {
			return "", 0, false
		}
		s.mu.Lock()
		reset := s.defaultRestored["ALRM"]
		s.mu.Unlock()
		if reset {
			return "", 0, false
		}
		return say, int(syscall.SIGALRM), true
	}
}
