// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a `kill` that names the shell itself with the child-state condition
// runs the handler — see Semantics.SelfAimedChildSignalRunsTheTrap, which
// carries the panel.
//
// Not a timing question. The condition stands for "a child of this shell
// changed state" in most of the panel, and a signal the script sent is not
// that, so the handler that answers a reaped child answers nothing; the
// columns with no such notion take the number like any other signal's.
//
// The probe is the short one on purpose. A longer one with a `sleep` and an
// external command in it has real children in it, and the handler runs for
// *those* wherever the column runs one at all — which is how this rode along
// unnoticed beside a timing row until the children were taken out (#4756).
func selfAimedChildRun(t *testing.T, a Answer) (string, int) {
	t.Helper()
	src := "trap 'echo C' CHLD\nkill -CHLD $$\necho a\n:\necho b\n"
	return run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.SelfAimedChildSignalRunsTheTrap = a
		r.Semantics = &s
	})
}

func TestASelfAimedChildSignalIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		want string
	}{
		{"the handler runs", Yes, "C\na\nb"},
		{"the condition is a child's, not a signal's", No, "a\nb"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := selfAimedChildRun(t, tc.a)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if status != 0 {
				t.Errorf("status = %d, want 0 — the send succeeded either way", status)
			}
		})
	}
}

// An axis unanswered is a refusal by name and not a quiet reading of either
// side, which is what keeps a dialect that never measured it out of the
// majority by accident.
func TestASelfAimedChildSignalUnansweredIsRefusedByName(t *testing.T) {
	out, _ := selfAimedChildRun(t, Unspecified)
	if !strings.Contains(out, "aimed at the shell itself") {
		t.Errorf("got %q, want the axis named", out)
	}
}

// The control the row rests on, and the one a fix that simply silenced the
// condition would fail: a **real** child still raises it under either answer.
func TestARealChildStillRaisesTheConditionUnderEitherAnswer(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		src := "n=0\ntrap 'n=$((n+1))' CHLD\n/bin/echo x >/dev/null\n:\n:\nprintf '%s' \"$n\"\n"
		out, _ := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.SelfAimedChildSignalRunsTheTrap = a
			r.Semantics = &s
		})
		if got := strings.TrimSpace(out); got != "1" {
			t.Errorf("%v: counted %q, want 1", a, got)
		}
	}
}

// And the second control: the rule is one condition rather than a class, so
// every other signal a script aims at the shell still runs its handler in
// every column.
func TestAnotherSelfAimedSignalStillRunsItsHandler(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		src := "trap 'echo U' USR1\nkill -USR1 $$\necho a\n"
		out, _ := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.SelfAimedChildSignalRunsTheTrap = a
			r.Semantics = &s
		})
		if got := strings.TrimSpace(out); got != "U\na" {
			t.Errorf("%v: got %q, want %q", a, got, "U\na")
		}
	}
}
