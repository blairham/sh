// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// unansweredWhen is this package's test vector in a dialect that gives a read
// up for a trapped signal and has not said when — so any read that asks
// Semantics.ReadAbandonedWhen is refused, and one that does not is not.
func unansweredWhen() Semantics {
	s := testSemantics()
	s.ReadIsAbandonedByATrappedSignal = Yes
	s.ReadAbandonedWhen = ReadAbandonmentUnspecified
	// The letters the rows use, and so that the timed row reaches the read
	// rather than the question of what its deadline bounds.
	s.ReadOptions = "rt:u:"
	s.ReadTimeoutBoundsReadability = No
	return s
}

const shellsDisagree = "the shells disagree here"

// TestAnOrdinaryReadAsksNothingAboutWhenASignalGivesItUp: the question is
// asked once a trapped signal has arrived during a read, and nowhere before
// that — not for a read with no trap, not for one with a trap that nothing
// arrives for, and not for one that waits a while for its line. Every reading
// waits for a byte and reads it alike until a signal comes.
func TestAnOrdinaryReadAsksNothingAboutWhenASignalGivesItUp(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"no trap", `mkfifo p; exec 3<>p; echo hi >p; read -r l <&3; echo "st=$? [$l]"`},
		{"a trap, nothing arriving", `mkfifo p; exec 3<>p; trap 'echo T' USR1; echo hi >p; ` +
			`read -r l <&3; echo "st=$? [$l]"`},
		{"a trap, the line arriving later", `mkfifo p; exec 3<>p; trap 'echo T' USR1; ` +
			`(sleep 0.2; echo hi >p) & read -r l <&3; echo "st=$? [$l]"; wait`},
		{"a trap, a timed read", `mkfifo p; exec 3<>p; trap 'echo T' USR1; ` +
			`(sleep 0.2; echo hi >p) & read -r -t 5 l <&3; echo "st=$? [$l]"; wait`},
		{"a trap, -u", `mkfifo p; exec 3<>p; trap 'echo T' USR1; ` +
			`(sleep 0.2; echo hi >p) & read -r -u 3 l; echo "st=$? [$l]"; wait`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(unansweredWhen()))
			if st != 0 || out != "st=0 [hi]\n" {
				t.Errorf("out=%q st=%d, want the line and no refusal", out, st)
			}
		})
	}
}

// TestTheCoreVectorAsksNothingOfAReadWithATrapSet: the core reads on through a
// trapped signal, so no read of it reaches the question at all — nor the
// watch. A regular file is the stream, since the core answers nothing about
// a fifo's `exec` either.
func TestTheCoreVectorAsksNothingOfAReadWithATrapSet(t *testing.T) {
	out, st := run(t, `echo hi >f; trap 'echo T' USR1; read -r l <f; echo "st=$? [$l]"`, withSem(CoreSemantics()))
	if st != 0 || out != "st=0 [hi]\n" {
		t.Errorf("out=%q st=%d, want the line and no refusal", out, st)
	}
}

// TestWhenASignalGivesUpAReadIsAskedWhereOneArrives is the other half: a
// trapped signal arriving during the read is where the readings part, and an
// unanswered axis is refused there, by name, at 2. The handler still runs.
func TestWhenASignalGivesUpAReadIsAskedWhereOneArrives(t *testing.T) {
	out, _ := run(t, `mkfifo p; exec 3<>p; trap 'echo T' USR1; `+
		`(sleep 0.2; kill -USR1 $$; sleep 0.3; echo late >p) & read -r l <&3; echo "st=$?"; wait`,
		withSem(unansweredWhen()))
	if !strings.Contains(out, "when a trapped signal gives up a `read`: "+shellsDisagree) ||
		!strings.Contains(out, "T\nst=2\n") {
		t.Errorf("got %q, want the refusal naming the question, the handler, and 2", out)
	}
}

// TestEachReadingGivesUpAReadWhereItSays runs the readings on the core vector
// over one stream — a line under way on a fifo, then a signal while the read
// waits for the rest — and over a fifo opened both ways, which is the one
// thing ReadAbandonedOnlyAtTheStartOfALine looks at.
func TestEachReadingGivesUpAReadWhereItSays(t *testing.T) {
	src := func(open string) string {
		return `mkfifo p; ` + open + `; trap 'echo T' USR1; printf ab >p; ` +
			`(sleep 0.2; kill -USR1 $$; sleep 0.3; printf 'cd\nend\n' >p) & ` +
			`read -r l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"; wait`
	}
	const gaveUp, readOn = "T\nst=1 l=[ab]\nnext [cd]\n", "T\nst=0 l=[abcd]\nnext [end]\n"
	for _, tc := range []struct {
		reading               ReadAbandonmentReading
		readingOnly, bothWays string
	}{
		{ReadAbandonedAtTheNextByte, gaveUp, gaveUp},
		{ReadAbandonedOnlyDuringAWait, gaveUp, gaveUp},
		{ReadAbandonedOnlyAtTheStartOfALine, readOn, gaveUp},
	} {
		t.Run(tc.reading.String(), func(t *testing.T) {
			sem := unansweredWhen()
			sem.ReadAbandonedWhen = tc.reading
			if out, _ := run(t, src(`exec 4<>p 3<p`), withSem(sem)); out != tc.readingOnly {
				t.Errorf("reading only: got %q, want %q", out, tc.readingOnly)
			}
			if out, _ := run(t, src(`exec 3<>p`), withSem(sem)); out != tc.bothWays {
				t.Errorf("both ways: got %q, want %q", out, tc.bothWays)
			}
		})
	}
}
