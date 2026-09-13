// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What an assignment means once `unset` has taken a produced parameter away.
// Two readings of identical text with no subset between them — see
// Semantics.AssignmentRestoresAnUnsetProducedParameter — so this names the
// axis and no shell.
//
// The producer counts, which is the discriminator the measurement uses: a
// stored value read twice is the same text twice, and a producer read twice is
// two different ones. A single read cannot tell them apart, and the assigned
// value is deliberately one nothing here would produce.

// counting registers a produced scalar that answers a different value on every
// read, with a writer that hears what a script assigns.
func counting(name string, tune func(*Semantics)) func(*Runner) {
	return func(r *Runner) {
		n := 0
		heard := ""
		r.SetDynamic(name, func(*Runner) string {
			n++
			return "made" + strconv.Itoa(n) + heard
		})
		r.SetDynamicWriter(name, func(_ *Runner, v string) { heard = "/" + v })
		if tune != nil {
			s := *r.Semantics
			tune(&s)
			r.Semantics = &s
		}
	}
}

func TestAssignmentRestoresAnUnsetProducedParameterIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		want string
	}{
		// The producer is back and answers both reads, each with a number
		// of its own — and it heard the assignment, which is what the
		// writer's mark says.
		{"restored", Yes, "[made1/9] [made2/9]"},
		// An ordinary name holding what was assigned, twice.
		{"an ordinary name", No, "[9] [9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, `unset X; X=9; a=$X; b=$X; echo "[$a] [$b]"`, counting("X", func(s *Semantics) {
				s.AssignmentRestoresAnUnsetProducedParameter = tc.a
			}))
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("%v: got %q, want %q", tc.a, got, tc.want)
			}
		})
	}
}

// The axis is asked about what `unset` left behind and about nothing else. An
// assignment to a parameter still standing is a message to its producer in
// every shell in the panel, so both answers have to leave that alone — and a
// test that only exercised the unset case could not tell a fix from a shell
// that had stopped producing altogether.
func TestAnAssignmentToAStandingProducedParameterIsStillAMessage(t *testing.T) {
	for _, a := range []Answer{Yes, No, Unspecified} {
		out, _ := run(t, `X=9; a=$X; b=$X; echo "[$a] [$b]"`, counting("X", func(s *Semantics) {
			s.AssignmentRestoresAnUnsetProducedParameter = a
		}))
		if want := "[made1/9] [made2/9]"; strings.TrimSpace(out) != want {
			t.Errorf("%v: got %q, want %q — the producer answers whether or not the axis is answered",
				a, strings.TrimSpace(out), want)
		}
	}
}

// An `unset` with no assignment after it is the same in both readings: the
// name is gone and reads as nothing.
func TestUnsetAloneLeavesTheNameEmptyUnderEitherAnswer(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, _ := run(t, `unset X; echo "[$X]" "${X-unset}"`, counting("X", func(s *Semantics) {
			s.AssignmentRestoresAnUnsetProducedParameter = a
		}))
		if want := "[] unset"; strings.TrimSpace(out) != want {
			t.Errorf("%v: got %q, want %q", a, strings.TrimSpace(out), want)
		}
	}
}

// The name a script has taken over is an ordinary one in every other respect
// too, which is what says the producer is gone rather than merely outvoted:
// it lists back, it is there for `-v`, and a second assignment lands on it.
func TestAnEndedProducerLeavesAnOrdinaryName(t *testing.T) {
	out, _ := run(t, `unset X; X=9; X=10; echo "[$X]" "${X+set}"`, counting("X", func(s *Semantics) {
		s.AssignmentRestoresAnUnsetProducedParameter = No
	}))
	if want := "[10] set"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q", strings.TrimSpace(out), want)
	}
}

// A subshell holds a copy of the tables, so a produced parameter it ends is
// still the parent's afterwards. The producer's counter is shared on purpose
// — it is the closure both runners see — so what this reads is which runner
// the *record* reached, not how many times anything was called.
func TestEndingAProducedParameterInASubshellDoesNotEndTheParents(t *testing.T) {
	out, _ := run(t, `(unset X; X=9; echo "in [$X]"); echo "out [${X#made}]"`, counting("X", func(s *Semantics) {
		s.AssignmentRestoresAnUnsetProducedParameter = No
	}))
	if want := "in [9]\nout [1]"; strings.TrimSpace(out) != want {
		t.Errorf("got %q, want %q — the parent's parameter is not the subshell's to end", strings.TrimSpace(out), want)
	}
}
