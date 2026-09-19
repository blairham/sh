// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether `--` is taken behind a signal option as well as in front of
// everything (#3817).
//
// Two places, two questions. In front — `kill -- T` — every column reaches
// the target, and this shell already did. Behind the signal is the spelling a
// script writes when it has a signal to name *and* a negative target, and
// only three columns take it.
//
// Every row aims at process group 99999, which does not exist, so no row can
// deliver anything to anything even where the default signal stands.
func TestKillTakesEndOfOptionsAfterTheSignalAxis(t *testing.T) {
	const absent = "-99999"

	for _, tc := range []struct {
		name     string
		answer   Answer
		src      string
		contains string
		refused  string
		// marker says this row expects a `--` to reach the target list, so
		// the blanket check below does not apply to it.
		marker bool
	}{
		{
			// bash 5.3.20, zsh 5.9.2, ksh93u+: the marker is consumed and
			// the target is reached, so the complaint is the target's.
			name: "the marker is taken behind the signal", answer: Yes,
			src:      "kill -0 -- " + absent,
			contains: "no such process",
		},
		{
			// dash 0.5.12 behind `-0`, and BusyBox ash behind both: the
			// marker is read as a target and complained about as one.
			name: "the marker is a target", answer: No,
			src:      "kill -0 -- " + absent,
			contains: "--",
		},
		{
			// CONTROL. In front of everything the marker is taken in every
			// column, and this axis must not reach that reading — no signal
			// was named, so the question is not asked. If it were asked and
			// answered No, this row would start complaining about `--`.
			name: "the marker in front is not this question", answer: No,
			src:      "kill -- " + absent,
			contains: "no such process",
		},
		{
			// CONTROL, and it is answered Yes on purpose. A target that is
			// not the marker must be left alone even where the axis is
			// taken — otherwise the reading consumes the first word behind
			// the signal whatever it is, and `kill -0 1234` loses its
			// target. Answered No this row proves nothing, because the
			// branch it is guarding against is not entered.
			name: "an ordinary target is untouched where the axis is taken", answer: Yes,
			src:      "kill -0 " + absent,
			contains: "no such process",
		},
		{
			// CONTROL. Exactly one marker is consumed: bash 5.3.20 and
			// ksh93u+ both complain about the second and then reach the
			// target behind it. The first is taken by the reading in front,
			// which leaves no signal read — so this axis must not fire on
			// the second and turn it into a third marker nobody wrote.
			name: "only one marker is consumed", answer: Yes,
			src:      "kill -- -- " + absent,
			contains: "--",
			marker:   true,
		},
		{
			name: "unanswered", answer: Unspecified,
			src:     "kill -0 -- " + absent,
			refused: "end-of-options marker behind the signal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.KillTakesEndOfOptionsAfterTheSignal = tc.answer
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			want := tc.contains
			if tc.refused != "" {
				want = tc.refused
			}
			if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
				t.Fatalf("got %q, want it to carry %q", out, want)
			}
			// The marker must never survive into the target list where it
			// was taken: a row that reports both complaints has consumed
			// nothing.
			if tc.answer == Yes && !tc.marker && strings.Contains(out, "--") {
				t.Errorf("got %q, which still complains about the marker", out)
			}
		})
	}
}
