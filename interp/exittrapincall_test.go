// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An `exit` written inside a function fires the EXIT trap from inside the
// call in one reading of the axis and after the unwinding in the other.
//
// A `local` is the probe, because it is the one every column can answer: the
// name of the running function is readable in only two of the six, and in a
// third the thing that looks like it reads the same value whether the call
// ended or returned — see Semantics.ExitTrapRunsInsideTheExitingCall, which
// carries the panel and that control.
func TestTheExitTrapAndTheCallThatExited(t *testing.T) {
	const src = `v=global
g() { local v=inner; trap 'echo "v=$v"' EXIT; exit 0; }
g`
	for _, c := range []struct {
		name   string
		inside Answer
		want   string
	}{
		{"the call is still standing", Yes, "v=inner"},
		{"or has been unwound first", No, "v=global"},
	} {
		t.Run(c.name, func(t *testing.T) {
			inside := c.inside
			out, st := run(t, src, func(r *Runner) {
				s := *r.Semantics
				s.ExitTrapRunsInsideTheExitingCall = inside
				r.Semantics = &s
			})
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("output %q, want %q", got, c.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}

// The control that says the axis is about `exit` and not about the trap: a
// function left by falling off its end has returned before the shell ends, so
// there is no call left to keep up and both answers read the global.
func TestTheExitTrapAfterAFunctionReturnedNormally(t *testing.T) {
	const src = `v=global
g() { local v=inner; trap 'echo "v=$v"' EXIT; }
g`
	for _, inside := range []Answer{Yes, No} {
		a := inside
		out, st := run(t, src, func(r *Runner) {
			s := *r.Semantics
			s.ExitTrapRunsInsideTheExitingCall = a
			r.Semantics = &s
		})
		if got := strings.TrimSpace(out); got != "v=global" {
			t.Errorf("with the axis %v: output %q, want %q", a, got, "v=global")
		}
		if st != 0 {
			t.Errorf("with the axis %v: status %d, want 0", a, st)
		}
	}
}

// The body still runs once, and the status `exit` asked for is still the
// shell's — the axis moves where the trap is fired from and nothing else.
func TestTheEarlyExitTrapRunsOnceAndKeepsTheStatus(t *testing.T) {
	const src = `g() { trap 'echo T' EXIT; exit 7; }
g`
	out, st := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ExitTrapRunsInsideTheExitingCall = Yes
		r.Semantics = &s
	})
	if got := strings.TrimSpace(out); got != "T" {
		t.Errorf("output %q, want a single T", got)
	}
	if st != 7 {
		t.Errorf("status %d, want 7", st)
	}
}

// A status the body exits with wins over the one `exit` asked for, the same
// way it does when the trap fires from the end of the shell.
func TestTheEarlyExitTrapMayChooseTheStatus(t *testing.T) {
	const src = `g() { trap 'exit 4' EXIT; exit 7; }
g`
	_, st := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ExitTrapRunsInsideTheExitingCall = Yes
		r.Semantics = &s
	})
	if st != 4 {
		t.Errorf("status %d, want 4", st)
	}
}

// At the top level the two readings cannot be told apart, so the axis is not
// asked at all: an unanswered one is a refusal, and refusing `exit` in a
// shell that has a trap would refuse the ordinary shape.
func TestTheExitTrapOutsideACallDoesNotAskTheAxis(t *testing.T) {
	const src = `trap 'echo T' EXIT; exit 0`
	out, st := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ExitTrapRunsInsideTheExitingCall = Unspecified
		r.Semantics = &s
	})
	if got := strings.TrimSpace(out); got != "T" {
		t.Errorf("output %q, want T with nothing refused", got)
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
}
