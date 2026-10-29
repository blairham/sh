// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// timedReadOn is base with a `read -t` it can reach and
// Semantics.ReadTimeoutWaitEndedByATrappedSignal unanswered, so a read that
// asks it is refused and one that does not is not. Semantics.ReadAbandonedWhen
// is unanswered too: a read that goes on through a trapped signal has no
// business asking when one gives it up. bounded is
// ReadTimeoutBoundsReadability, which decides what the deadline covers and
// not whether this is asked.
func timedReadOn(base Semantics, bounded Answer) Semantics {
	s := base
	s.ReadIsAbandonedByATrappedSignal = No
	s.ReadAbandonedWhen = ReadAbandonmentUnspecified
	s.ReadTimeoutWaitEndedByATrappedSignal = Unspecified
	s.ReadOptions = "rt:u:"
	s.ReadTimeoutBoundsReadability = bounded
	s.ReadTimeoutKeepsWhatArrived = No
	return s
}

// signalLandsInTheFirstWait is the shape the question is about: a fifo opened
// for reading alone with nothing in, USR1 trapped and sent a second in, and a
// line written a second after that. first is written before the read.
//
// The tests that send it run their rows in parallel with each other and with no
// other test in the package, so that a signal another test raises through the
// kernel cannot reach a Runner here (#7040). The second is a margin for a
// loaded runner under the race detector: a signal that lands before the read
// is waiting is not one this asks about.
func signalLandsInTheFirstWait(first string) string {
	return `mkfifo p; exec 4<>p 3<p; ` + first + `l=old; trap 'echo T' USR1; ` +
		`(sleep 1; kill -USR1 $$; sleep 1; printf 'late\nend\n' >p) & ` +
		`read -r -t 10 l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`
}

// TestAnOrdinaryTimedReadAsksNothingAboutASignalEndingItsWait: the question
// is put only when a trapped signal lands while a timed read waits for its
// first byte. A timed read with no trap, one whose byte is there or arrives in
// time, one whose byte came before the signal did, and one begun with a signal
// already waiting — its line there, or arriving after — ask nothing, under the
// core vector, which answers nothing about it, given only the letter and what
// its deadline bounds. The last two rows are zsh's, bash 5.3's and bash 3.2's
// answer as well, measured 2026-10-29.
func TestAnOrdinaryTimedReadAsksNothingAboutASignalEndingItsWait(t *testing.T) {
	// The core answers none of what the scripts pass on the way to the read
	// either — whether the descriptors `exec` parks reach `mkfifo`, the jobs a
	// command substitution sees, the status and parse of a trap body — so
	// those are POSIX's, which are not the question, and the jobs are kept.
	core, posix := CoreSemantics(), PosixSemantics()
	core.ExecOpenedFdReachesACommand = posix.ExecOpenedFdReachesACommand
	core.SubshellJobTable = SubshellJobsKept
	core.SignalHandlerSeesEarlierStatus = posix.SignalHandlerSeesEarlierStatus
	core.TrapBodyRunsWhatParsed = posix.TrapBodyRunsWhatParsed
	for _, bounded := range []Answer{Yes, No} {
		sem := timedReadOn(core, bounded)
		for _, tc := range []struct{ name, src, want string }{
			{"no trap", `mkfifo p; exec 4<>p 3<p; (sleep 0.2; echo hi >p) & ` +
				`read -r -t 5 l <&3; echo "st=$? [$l]"; wait`, "st=0 [hi]\n"},
			{"a trap, the line there", `mkfifo p; exec 4<>p 3<p; trap 'echo T' USR1; echo hi >p; ` +
				`read -r -t 5 l <&3; echo "st=$? [$l]"`, "st=0 [hi]\n"},
			{"a trap, the line arriving", `mkfifo p; exec 4<>p 3<p; trap 'echo T' USR1; ` +
				`(sleep 0.2; echo hi >p) & read -r -t 5 l <&3; echo "st=$? [$l]"; wait`, "st=0 [hi]\n"},
			{
				"a byte before the signal", signalLandsInTheFirstWait(`printf ab >p; `),
				"T\nst=0 l=[ablate]\nnext [end]\n",
			},
			{
				"a signal waiting, the line there", `mkfifo p; exec 4<>p 3<p; trap 'echo T' USR1; echo hi >p; ` +
					`read -r -t 5 l <&3 2>"$(kill -USR1 $$)/dev/null"; echo "st=$? [$l]"`,
				"T\nst=0 [hi]\n",
			},
			{
				"a signal before the read", `mkfifo p; exec 4<>p 3<p; trap 'echo T' USR1; ` +
					`(sleep 0.5; echo hi >p) & read -r -t 5 l <&3 2>"$(kill -USR1 $$)/dev/null"; echo "st=$? [$l]"; wait`,
				"T\nst=0 [hi]\n",
			},
		} {
			t.Run(bounded.String()+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				out, st := run(t, tc.src, withSem(sem))
				if st != 0 || out != tc.want {
					t.Errorf("out=%q st=%d, want %q and no refusal", out, st, tc.want)
				}
			})
		}
	}
}

// TestASignalInATimedReadsFirstWaitIsAskedWhereItLands is the other half: a
// trapped signal landing in that wait is where the dialects part, and an
// unanswered axis is refused there, by name, at 2. The handler still runs,
// and nothing was read, so the line written afterwards is the next read's.
func TestASignalInATimedReadsFirstWaitIsAskedWhereItLands(t *testing.T) {
	out, _ := run(t, signalLandsInTheFirstWait(""), withSem(timedReadOn(testSemantics(), No)))
	if !strings.Contains(out, "a trapped signal ending a `read -t` still waiting for its first byte: "+shellsDisagree) ||
		!strings.Contains(out, "T\nst=2 l=[old]\nnext [late]\n") {
		t.Errorf("got %q, want the refusal naming the question, the handler, 2 and the line left", out)
	}
}

// TestEachAnswerToASignalInATimedReadsFirstWait runs both answers, under both
// readings of what the deadline bounds. Yes ends the read as the deadline
// would — the timeout's status, the name untouched, the line the next read's;
// no reads on and takes it. With a byte in before the signal, both read on.
func TestEachAnswerToASignalInATimedReadsFirstWait(t *testing.T) {
	for _, tc := range []struct {
		answer            Answer
		nothingIn, byteIn string
	}{
		{Yes, "T\nst=1 l=[old]\nnext [late]\n", "T\nst=0 l=[ablate]\nnext [end]\n"},
		{No, "T\nst=0 l=[late]\nnext [end]\n", "T\nst=0 l=[ablate]\nnext [end]\n"},
	} {
		for _, bounded := range []Answer{Yes, No} {
			t.Run(tc.answer.String()+"/bounded "+bounded.String(), func(t *testing.T) {
				t.Parallel()
				sem := timedReadOn(testSemantics(), bounded)
				sem.ReadTimeoutWaitEndedByATrappedSignal = tc.answer
				if out, _ := run(t, signalLandsInTheFirstWait(""), withSem(sem)); out != tc.nothingIn {
					t.Errorf("nothing in: got %q, want %q", out, tc.nothingIn)
				}
				if out, _ := run(t, signalLandsInTheFirstWait(`printf ab >p; `), withSem(sem)); out != tc.byteIn {
					t.Errorf("a byte in: got %q, want %q", out, tc.byteIn)
				}
			})
		}
	}
}

// TestATimedReadGivenUpForAnySignalAsksNothingAboutItsFirstWait: where every
// read is given up for a trapped signal, that is what ends a timed one too, by
// Semantics.ReadAbandonedWhen — at the end of the input, the name cleared —
// and the question about the first wait is never put.
func TestATimedReadGivenUpForAnySignalAsksNothingAboutItsFirstWait(t *testing.T) {
	sem := timedReadOn(testSemantics(), No)
	sem.ReadIsAbandonedByATrappedSignal = Yes
	sem.ReadAbandonedWhen = ReadAbandonedOnlyDuringAWait
	out, st := run(t, signalLandsInTheFirstWait(""), withSem(sem))
	if want := "T\nst=1 l=[]\nnext [late]\n"; st != 0 || out != want {
		t.Errorf("out=%q st=%d, want %q and no refusal", out, st, want)
	}
}
