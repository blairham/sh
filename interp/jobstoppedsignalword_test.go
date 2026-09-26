// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// The state word of a stopped job can be the *stopping signal's* own.
//
// One dialect has four words where this shell had one: ^Z is the plain word
// and SIGSTOP, SIGTTIN and SIGTTOU each have their own — see
// Diagnostics.JobStoppedBySignal for the measurement (#4527). The rule is a
// lookup rather than a wording, because the words are the job-control meaning
// of the signal rather than the host's name for it.
//
// The first row is what keeps the change honest: ^Z must still reach the plain
// word with the map in place, so a dialect cannot buy the other three by
// giving up the one it already had. The last two are the mutant — a shell that
// writes one word for every stop answers them with the wrong string.
//
// **Compared whole rather than searched for.** The plain word is a prefix of
// every other one here, exactly as it is in the shell this was measured on, so
// a `strings.Contains` would pass the first row for a shell that answered it
// with `suspended (signal)` — which is the one mistake this change could make.
func TestAStoppedJobsStateWordCanBeTheSignalsOwn(t *testing.T) {
	worded := map[syscall.Signal]string{
		syscall.SIGSTOP: "suspended (signal)",
		syscall.SIGTTIN: "suspended (tty input)",
		syscall.SIGTTOU: "suspended (tty output)",
	}
	for _, tc := range []struct {
		name  string
		words map[syscall.Signal]string
		sig   syscall.Signal
		want  string
	}{
		{"^Z keeps the plain word", worded, syscall.SIGTSTP, "suspended"},
		{"an explicit stop has its own", worded, syscall.SIGSTOP, "suspended (signal)"},
		{"reading the terminal has its own", worded, syscall.SIGTTIN, "suspended (tty input)"},
		{"writing to the terminal has its own", worded, syscall.SIGTTOU, "suspended (tty output)"},
		{"a dialect with one word for every stop", nil, syscall.SIGSTOP, "suspended"},
		{"and one for ^Z too", nil, syscall.SIGTSTP, "suspended"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			dg := Diagnostics{JobStopped: "suspended", JobStoppedBySignal: tc.words}
			r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg})
			r.addStoppedJob(1, []string{"sleep", "30"}, tc.sig)

			if got := r.jobState(r.jobs[0], false); got != tc.want {
				t.Errorf("the state column is %q, want %q", got, tc.want)
			}
			// And the listing really carries it, so the word is not only
			// right in the accessor the row above reads.
			sem.JobsShowBackgroundCommand = Yes
			if line := r.jobLine(0, r.jobs[0], true); !strings.Contains(line, tc.want) {
				t.Errorf("line = %q, want the state %q in it", line, tc.want)
			}
		})
	}
}

// And the mapped wording still takes the verb the plain one takes.
//
// JobStopped carries the signal's *number* for the dialect that prints it —
// `Suspended: 18` — and a second table of wordings that quietly dropped the
// verb would leave such a dialect unable to word a stop it names. The number
// is not written out, for the reason TestTheLineOfAStoppedJob gives: it is the
// platform's and not the shell's.
func TestAStoppedJobsSignalWordStillTakesTheNumber(t *testing.T) {
	sem := CoreSemantics()
	dg := Diagnostics{
		JobStopped: "Suspended: %[1]d",
		JobStoppedBySignal: map[syscall.Signal]string{
			syscall.SIGTTIN: "Stopped (tty input): %[1]d",
		},
	}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg})
	r.addStoppedJob(1, []string{"sleep", "30"}, syscall.SIGTTIN)

	want := "Stopped (tty input): " + strconv.Itoa(int(syscall.SIGTTIN))
	if got := r.jobState(r.jobs[0], false); got != want {
		t.Errorf("the state column is %q, want %q", got, want)
	}
}
