// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.ReadHandlersRun: when the handler of a trapped signal that arrives
// during a `read` the signal does not end runs (#7031).

// handlersOn is the test vector with reads that go on through a trapped
// signal, and the moment their handlers run set to moment. A handler's
// `return` there ends the read with no name touched, which is bash's and
// ksh93's answer; TestAHandlersReturnInTheMiddleOfAReadAsksTheDialect is the
// test about the other. Two different
// conditions can wait together in some rows, so the order they run in is
// answered too, lowest signal number first, and so are the jobs a command
// substitution sees, which the rows that raise a signal from one pass on the
// way to the read.
func handlersOn(moment ReadHandlerMoment) Semantics {
	s := testSemantics()
	s.ReadIsAbandonedByATrappedSignal = No
	s.ReadHandlersRun = moment
	s.ReadLeftByAHandlersReturnAssignsWhatArrived = No
	s.WaitingHandlersRun = WaitingHandlersRunUpTheSignalNumbers
	s.SubshellJobTable = SubshellJobsKept
	return s
}

// landsInAWait has `ab` in a fifo and the read waiting for the rest when USR1
// lands, then `W`, then `cd`. The job starts its clock only once the shell has
// written `ab` and is about to read — it says so through q — and the second
// before the signal is a margin for the read to have taken `ab` and be
// waiting. The handler shows `$?`, which `false` left, and the name.
const landsInAWait = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo "T st=$? l=[$l]"' USR1; ` +
	`(read y <q; sleep 1; kill -USR1 $$; sleep 1; echo W; echo cd >p) & ` +
	`printf ab >p; echo >q; false; read -r l <&3; echo "st=$? l=[$l]"; wait`

// waitingAsItBegins raises USR1 from a command substitution in the read's own
// redirection, so it is already waiting as the read begins — nothing about
// that is a race — and the line comes a second later, after `W`.
const waitingAsItBegins = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'echo "T l=[$l]"' USR1; ` +
	`(read y <q; sleep 1; echo W; echo cd >p) & ` +
	`echo >q; read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo "st=$? l=[$l]"; wait`

// TestEachAnswerRunsAReadsHandlerWhereItSays: after the read, the handler sees
// the line read and the read's own status. In the middle of it — at a signal
// landing in a wait, in two answers, and as the read begins, in one — it sees
// the name as it was and `$?` as the read found it, and the read then goes on
// to the end of its line.
func TestEachAnswerRunsAReadsHandlerWhereItSays(t *testing.T) {
	for _, tc := range []struct {
		moment          ReadHandlerMoment
		inAWait, before string
	}{
		{
			ReadHandlersRunAfterTheRead,
			"W\nT st=0 l=[abcd]\nst=0 l=[abcd]\n",
			"W\nT l=[cd]\nst=0 l=[cd]\n",
		},
		{
			ReadHandlersRunWhenAWaitIsInterrupted,
			"T st=1 l=[old]\nW\nst=0 l=[abcd]\n",
			"W\nT l=[cd]\nst=0 l=[cd]\n",
		},
		{
			ReadHandlersRunAsSoonAsTheyArrive,
			"T st=1 l=[old]\nW\nst=0 l=[abcd]\n",
			"T l=[old]\nW\nst=0 l=[cd]\n",
		},
	} {
		t.Run(tc.moment.String(), func(t *testing.T) {
			t.Parallel()
			if out, st := run(t, landsInAWait, withSem(handlersOn(tc.moment))); out != tc.inAWait || st != 0 {
				t.Errorf("in a wait: got %q at %d, want %q at 0", out, st, tc.inAWait)
			}
			if out, st := run(t, waitingAsItBegins, withSem(handlersOn(tc.moment))); out != tc.before || st != 0 {
				t.Errorf("waiting as it begins: got %q at %d, want %q at 0", out, st, tc.before)
			}
		})
	}
}

// TestASignalLandingInAWaitRunsTheHandlersAlreadyWaitingToo: where a read
// stops for a signal that lands in a wait, every handler waiting runs then,
// as one batch — the one whose signal came before the read began as well.
// Measured 2026-10-31: bash 5.3.20 and 3.2.57 with INT raised in the read's
// own redirection and USR1 landing mid-read run both before the line ends.
func TestASignalLandingInAWaitRunsTheHandlersAlreadyWaitingToo(t *testing.T) {
	src := `mkfifo p q; exec 4<>p 3<p; trap 'echo H' HUP; trap 'echo U' USR1; ` +
		`(read y <q; sleep 1; kill -USR1 $$; sleep 1; echo W; echo cd >p) & ` +
		`printf ab >p; echo >q; read -r l <&3 5>"$(kill -HUP $$)/dev/null"; echo "l=[$l]"; wait`
	for _, tc := range []struct {
		moment ReadHandlerMoment
		want   string
	}{
		{ReadHandlersRunWhenAWaitIsInterrupted, "H\nU\nW\nl=[abcd]\n"},
		{ReadHandlersRunAsSoonAsTheyArrive, "H\nU\nW\nl=[abcd]\n"},
		{ReadHandlersRunAfterTheRead, "W\nH\nU\nl=[abcd]\n"},
	} {
		t.Run(tc.moment.String(), func(t *testing.T) {
			t.Parallel()
			if out, st := run(t, src, withSem(handlersOn(tc.moment))); out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}

// TestAHandlerThatLeavesEndsTheReadWithNoNameTouched: a handler run in the
// middle of a read that exits the shell, or returns from the function the
// read is in, ends the read there. No name is assigned, the status is the
// handler's, and what the read had not taken is the next reader's. Measured
// 2026-10-31: after `exit 3` an EXIT trap reads the name as it was, at 3, in
// bash 5.3, zsh and ksh93; `return 5` returns 5 in bash 5.3 and ksh93.
func TestAHandlerThatLeavesEndsTheReadWithNoNameTouched(t *testing.T) {
	sem := handlersOn(ReadHandlersRunAsSoonAsTheyArrive)
	out, st := run(t, `mkfifo p; exec 4<>p 3<p; l=old; trap 'echo "E st=$? l=[$l]"' EXIT; trap 'exit 3' USR1; `+
		`printf 'ab\n' >p; read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo not reached`, withSem(sem))
	if want := "E st=3 l=[old]\n"; out != want || st != 3 {
		t.Errorf("exit: got %q at %d, want %q at 3", out, st, want)
	}
	out, st = run(t, `mkfifo p; exec 4<>p 3<p; l=old; trap 'return 5' USR1; `+
		`f() { read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo in; }; `+
		`printf 'ab\ncd\n' >p; f; echo "out st=$? l=[$l]"; read -r m <&3; echo "m=[$m]"`, withSem(sem))
	if want := "out st=5 l=[old]\nm=[ab]\n"; out != want || st != 0 {
		t.Errorf("return: got %q at %d, want %q at 0", out, st, want)
	}
}

// TestAHandlerInTheMiddleOfAReadSeesTheStatusAsItStands: the read has no
// status yet, so even where a handler is shown the status from before the
// command (Semantics.SignalHandlerSeesEarlierStatus) one run mid-read sees
// `$?` as the read found it — here, what a command substitution in its own
// redirection left, not what came before the read. Measured 2026-10-31: zsh
// 5.9.2's handler for USR1 landing mid-read reads 0 there after `false`.
func TestAHandlerInTheMiddleOfAReadSeesTheStatusAsItStands(t *testing.T) {
	sem := handlersOn(ReadHandlersRunAsSoonAsTheyArrive)
	sem.SignalHandlerSeesEarlierStatus = Yes
	src := `mkfifo p q; exec 4<>p 3<p; trap 'echo "T st=$?"' USR1; ` +
		`(read y <q; sleep 1; kill -USR1 $$; sleep 1; echo cd >p) & ` +
		`printf ab >p; echo >q; false; read -r l <&3 5>"$(exit 0)/dev/null"; echo "st=$? l=[$l]"; wait`
	if out, st := run(t, src, withSem(sem)); out != "T st=0\nst=0 l=[abcd]\n" || st != 0 {
		t.Errorf("got %q at %d, want %q at 0", out, st, "T st=0\nst=0 l=[abcd]\n")
	}
}

// TestTheReadHandlerMomentIsAskedOnlyWhereItDecides: unanswered, the axis is
// refused by name where a trapped signal is waiting during a read that goes
// on — at 2, no name touched, and the handler run once the builtin has
// returned — and nowhere else: not where nothing arrives, not for an ignored
// signal, not on a regular file, which never makes a read wait, and not where
// the signal gives the read up.
func TestTheReadHandlerMomentIsAskedOnlyWhereItDecides(t *testing.T) {
	const asked = "when the handler of a signal that arrives during a `read` runs: " + shellsDisagree
	out, _ := run(t, `mkfifo p; exec 4<>p 3<p; l=old; trap 'echo T' USR1; printf 'ab\n' >p; `+
		`read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo "st=$? l=[$l]"`,
		withSem(handlersOn(ReadHandlerMomentUnspecified)))
	if !strings.Contains(out, asked) || !strings.HasSuffix(out, "T\nst=2 l=[old]\n") {
		t.Errorf("got %q, want the refusal naming the question, then the handler, 2 and l untouched", out)
	}
	givenUp := handlersOn(ReadHandlerMomentUnspecified)
	givenUp.ReadIsAbandonedByATrappedSignal = Yes
	givenUp.ReadAbandonedWhen = ReadAbandonedAtTheNextByte
	for _, tc := range []struct {
		name, src, want string
		sem             Semantics
	}{
		{
			"nothing arrives",
			`mkfifo p; exec 4<>p 3<p; trap 'echo T' USR1; printf 'hi\n' >p; read -r l <&3; echo "[$l]"`,
			"[hi]\n", handlersOn(ReadHandlerMomentUnspecified),
		},
		{
			"an ignored signal",
			`mkfifo p; exec 4<>p 3<p; trap 'echo T' HUP; trap '' USR1; printf 'hi\n' >p; ` +
				`read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo "[$l]"`,
			"[hi]\n", handlersOn(ReadHandlerMomentUnspecified),
		},
		{
			"a regular file",
			`trap 'echo T' USR1; echo hi >f; read -r l <f 5>"$(kill -USR1 $$)/dev/null"; echo "[$l]"`,
			"T\n[hi]\n", handlersOn(ReadHandlerMomentUnspecified),
		},
		{
			"a read the signal gives up",
			`mkfifo p; exec 4<>p 3<p; l=old; trap 'echo T' USR1; printf 'hi\n' >p; ` +
				`read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo "st=$? l=[$l]"`,
			"T\nst=1 l=[]\n", givenUp,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := run(t, tc.src, withSem(tc.sem)); out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0 and nothing asked", out, st, tc.want)
			}
		})
	}
}

// Semantics.ReadLeftByAHandlersReturnAssignsWhatArrived: a handler run in the
// middle of a read that returns from the function the read is in (#7176).

// aReturnInAWait has `ab` in a fifo and the read, in f, waiting for the rest
// when USR1 lands and its handler returns 5; `cd` comes a second later.
const aReturnInAWait = `mkfifo p q; exec 4<>p 3<p; l=old; trap 'return 5' USR1; ` +
	`f() { read -r l <&3; echo in; }; ` +
	`(read y <q; sleep 1; kill -USR1 $$; sleep 1; echo cd >p) & ` +
	`printf ab >p; echo >q; f; echo "out st=$? l=[$l]"; read -r m <&3; echo "m=[$m]"; wait`

// aReturnAsItBegins has USR1 waiting as the read begins, raised in its own
// redirection, so the handler returns before a byte is taken.
const aReturnAsItBegins = `mkfifo p; exec 4<>p 3<p; l=old; trap 'return 5' USR1; ` +
	`f() { read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo in; }; ` +
	`printf 'ab\ncd\n' >p; f; echo "out st=$? l=[$l]"; read -r m <&3; echo "m=[$m]"`

// TestAHandlersReturnInTheMiddleOfAReadAsksTheDialect: yes ends the read as at
// the end of its input — what arrived assigned, the name cleared where nothing
// had, and the read's 1 the function's status in place of the `return`'s 5 —
// and no ends it with no name touched at 5. Either way the rest of the line is
// the next reader's. Unanswered, it is refused by name at 2 with no name
// touched.
func TestAHandlersReturnInTheMiddleOfAReadAsksTheDialect(t *testing.T) {
	const asked = "a `read` a handler's `return` left in the middle assigning what arrived: " + shellsDisagree
	for _, tc := range []struct {
		answer              Answer
		inAWait, asItBegins string
	}{
		{Yes, "out st=1 l=[ab]\nm=[cd]\n", "out st=1 l=[]\nm=[ab]\n"},
		{No, "out st=5 l=[old]\nm=[cd]\n", "out st=5 l=[old]\nm=[ab]\n"},
		{Unspecified, "out st=2 l=[old]\nm=[cd]\n", "out st=2 l=[old]\nm=[ab]\n"},
	} {
		t.Run(tc.answer.String(), func(t *testing.T) {
			t.Parallel()
			sem := handlersOn(ReadHandlersRunAsSoonAsTheyArrive)
			sem.ReadLeftByAHandlersReturnAssignsWhatArrived = tc.answer
			for _, row := range []struct{ name, src, want string }{
				{"in a wait", aReturnInAWait, tc.inAWait},
				{"as it begins", aReturnAsItBegins, tc.asItBegins},
			} {
				out, st := run(t, row.src, withSem(sem))
				refused := strings.Contains(out, asked)
				if !strings.HasSuffix(out, row.want) || st != 0 || refused != (tc.answer == Unspecified) {
					t.Errorf("%s: got %q at %d, want %q at 0, refused only where unanswered", row.name, out, st, row.want)
				}
			}
		})
	}
}

// TestAHandlersReturnIsTheOnlyLeavingThatAsks: unanswered, the axis is asked
// of nothing but a `return` — not of an `exit`, which ends the read with no
// name touched in every column, and not of a handler that does not leave, after
// which the read goes on.
func TestAHandlersReturnIsTheOnlyLeavingThatAsks(t *testing.T) {
	sem := handlersOn(ReadHandlersRunAsSoonAsTheyArrive)
	sem.ReadLeftByAHandlersReturnAssignsWhatArrived = Unspecified
	out, st := run(t, `mkfifo p; exec 4<>p 3<p; l=old; trap 'echo "E st=$? l=[$l]"' EXIT; trap 'exit 3' USR1; `+
		`printf 'ab\n' >p; read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo not reached`, withSem(sem))
	if want := "E st=3 l=[old]\n"; out != want || st != 3 {
		t.Errorf("exit: got %q at %d, want %q at 3", out, st, want)
	}
	out, st = run(t, `mkfifo p; exec 4<>p 3<p; l=old; trap 'echo T' USR1; `+
		`f() { read -r l <&3 5>"$(kill -USR1 $$)/dev/null"; echo "in st=$? l=[$l]"; }; printf 'ab\n' >p; f`, withSem(sem))
	if want := "T\nin st=0 l=[ab]\n"; out != want || st != 0 {
		t.Errorf("a handler that stays: got %q at %d, want %q at 0", out, st, want)
	}
}
