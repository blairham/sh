// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// killManyDiag is the wording these cases read the rows off, chosen so that
// each complaint is one line and the three are told apart by their text.
//
// KillUnknownSignalHint is set because one case needs a *second* line for a
// failure that is not an operand's — see the status case below, where it is
// the whole point.
func killManyDiag() Diagnostics {
	return Diagnostics{
		KillNotAPid:           "kill: illegal pid: %[1]s",
		KillNoSuchProcess:     "kill: no such process: %[1]s",
		KillInvalidSignal:     "kill: unknown signal: %[1]s",
		KillUnknownSignalHint: "kill: type kill -l for a list",
	}
}

func killManyErrLines(errs string) []string {
	return strings.Split(strings.TrimSuffix(errs, "\n"), "\n")
}

// Whether the operands behind a word that is not a pid are still operands
// (#4648).
//
// The axis is about the *malformed word* and nothing else. A well-formed pid
// that reaches nothing is a send that failed, and every column in the panel
// carries on past one of those — which is the control below, and is the pair
// that holds the noun fixed: same number of operands, same number of
// failures, one word readable and the other not.
func TestKillKeepsGoingPastAnOperandThatIsNotAPidAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    []string
		refused string
	}{
		{
			// bash 5.3.20, zsh 5.9.2 and BusyBox ash: every operand is
			// reported, in the order they were written.
			name: "every operand behind the bad one is reported", answer: Yes,
			src:  "kill a b c",
			want: []string{"illegal pid: a", "illegal pid: b", "illegal pid: c"},
		},
		{
			// ksh93u+ and dash 0.5.12: the first bad word ends the builtin.
			name: "the first bad operand ends it", answer: No,
			src:  "kill a b c",
			want: []string{"illegal pid: a"},
		},
		{
			// CONTROL, and the pair that holds the noun fixed. Two operands
			// and two failures again, but both words *are* pids — so this
			// axis must not reach them, and answering it No must not stop
			// the second from being reported. Nothing in the panel stops
			// here, this shell never did, and a rule keyed on "an operand
			// that failed" rather than "a word that is not a pid" would
			// silence the second line the moment it were answered No.
			name: "a pid that reaches nothing is not this question", answer: No,
			src:  "kill 999998 999999",
			want: []string{"no such process: 999998", "no such process: 999999"},
		},
		{
			// CONTROL, the other half of the same pair: answered Yes the
			// unreachable pids are still both reported, so the rows above
			// and below differ by the axis and by nothing else.
			name: "and it is not this question answered the other way", answer: Yes,
			src:  "kill 999998 999999",
			want: []string{"no such process: 999998", "no such process: 999999"},
		},
		{
			// CONTROL. One operand needs nobody's policy: there is nothing
			// behind it to carry on to, so the axis is never consulted and
			// an Unspecified vector must not refuse here. Answered Yes or
			// No this row proves nothing — Unspecified is what makes it a
			// test of *where the question is asked* rather than of the
			// answer.
			name: "a single operand does not ask the question", answer: Unspecified,
			src:  "kill a",
			want: []string{"illegal pid: a"},
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     "kill a b c",
			refused: "operands behind one that is not a pid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = tc.answer
			_, errs, st := killRun(t, tc.src, sem, killManyDiag())
			if tc.refused != "" {
				if !strings.Contains(errs, tc.refused) {
					t.Fatalf("stderr = %q, want a refusal naming %q", errs, tc.refused)
				}
				// A refusal is 2 and is the whole answer: the builtin must
				// not go on to report the operand as well, which would hand
				// back this dialect's status for a bad pid and bury the
				// refusal it has just written.
				if st != 2 {
					t.Errorf("status = %d, want 2 for an unanswered axis", st)
				}
				if strings.Contains(errs, "illegal pid") {
					t.Errorf("stderr = %q: the refusal was followed by an answer", errs)
				}
				return
			}
			got := killManyErrLines(errs)
			if len(got) != len(tc.want) {
				t.Fatalf("stderr = %q: %d lines, want %d", errs, len(got), len(tc.want))
			}
			for i, w := range tc.want {
				if !strings.Contains(got[i], w) {
					t.Errorf("line %d = %q, want it to carry %q", i, got[i], w)
				}
			}
		})
	}
}

// What the status counts, which is the *operands that failed* and not the
// diagnostics written.
//
// The two agree on nearly every row, which is what makes the distinction easy
// to get wrong: three bad operands are three lines and 3. They part where one
// failure writes two lines — a signal this shell cannot name, refused with a
// listing hint behind it, before any operand has been looked at. Two lines,
// and the count is 1 rather than 2, because nothing in the target list ever
// failed.
//
// Held fixed across the pair: the number of lines on stderr. Varied: whether
// those lines came from operands. If the status followed the lines, the two
// rows would report the same number.
func TestKillStatusCountsFailedOperandsAndNotDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		src    string
		lines  int
		status int
	}{
		{"three bad operands are three lines and 3", "kill a b c", 3, 3},
		{"four are four and 4", "kill a b c d", 4, 4},
		{"the same word written three times still counts three", "kill a a a", 3, 3},
		{
			// The separating row. Two lines, and no operand was reached.
			"a signal nobody can name is two lines and 1", "kill -NOPE a b", 2, 1,
		},
		{
			// Its pair, at the same number of lines.
			"two bad operands are two lines and 2", "kill a b", 2, 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillStatus = KillStatusFailureCount
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = Yes
			_, errs, st := killRun(t, tc.src, sem, killManyDiag())
			if got := len(killManyErrLines(errs)); got != tc.lines {
				t.Errorf("stderr = %q: %d lines, want %d", errs, got, tc.lines)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d (stderr %q)", st, tc.status, errs)
			}
		})
	}
}

// A target that works among ones that do not is still signaled, and the
// status counts only what failed.
//
// Signal 0 so that nothing is delivered to anything: the probe succeeds for a
// process that is there, which is what makes this a row about the *count*
// rather than about a shell that has just signaled itself.
func TestKillSignalsTheGoodTargetsAmongTheBadOnes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy KillStatusPolicy
		status int
	}{
		// zsh 5.9.2 and BusyBox ash: two operands failed, so 2.
		{"the count is of the failures alone", KillStatusFailureCount, 2},
		// bash 5.3.20: something was signaled, so 0.
		{"a success is the whole answer elsewhere", KillStatusAnySuccess, 0},
		// ksh93u+ and dash 0.5.12 never reach this, because they stop at
		// the first bad word — but the policy is still a value, and a
		// failure is a failure under it.
		{"and a failure is the whole answer elsewhere again", KillStatusAnyFailure, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := killSem()
			sem.KillStatus = tc.policy
			sem.KillKeepsGoingPastAnOperandThatIsNotAPid = Yes
			_, errs, st := killRun(t, "kill -0 a $$ b", sem, killManyDiag())
			if got := len(killManyErrLines(errs)); got != 2 {
				t.Fatalf("stderr = %q: %d lines, want the two bad operands", errs, got)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d (stderr %q)", st, tc.status, errs)
			}
		})
	}
}
