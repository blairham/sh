// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// named runs with the pipeline-status record exposed under a name, which is
// all a dialect supplies.
func named(name string, tune func(*Semantics)) func(*Runner) {
	return func(r *Runner) {
		r.SetPipelineStatus(name)
		if tune != nil {
			s := *r.Semantics
			tune(&s)
			r.Semantics = &s
		}
	}
}

// What the record holds. `$?` reports the last element only, which is the
// whole reason the others are worth keeping.
func TestThePipelineStatusRecordsEveryElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"three elements", `false | true | false; echo "${P[@]}"`, "1 0 1"},
		{"a command on its own records one", `false; echo "${P[@]}"`, "1"},
		// The `if` is the command that just ran; the pipeline inside it is
		// gone by the time the clause finishes.
		{"a compound command records its own", `if false | true; then :; fi; echo "${P[@]}"`, "0"},
		// `!` inverts what the pipeline reports, not what its elements did.
		{"negation does not reach it", `! false | true; echo "${P[@]}"`, "1 0"},
		{"count", `true | true | true; echo "${#P[@]}"`, "3"},
		{"one element by index", `false | true; echo "${P[0]}${P[1]}"`, "10"},
		{"a later pipeline replaces it", `false | true; true | false; echo "${P[@]}"`, "0 1"},
		{"inside a function", `f() { false | true; echo "${P[@]}"; }; f`, "1 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, named("P", nil))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// Without a dialect to name it, there is no record at all — nothing could read
// one, so nothing is kept.
func TestNoNameMeansNoRecord(t *testing.T) {
	out, _ := run(t, `false | true; echo "[${PIPESTATUS[@]}]"`, nil)
	if strings.TrimSpace(out) != "[]" {
		t.Errorf("got %q, want an ordinary absent variable", out)
	}
}

// Whether an assignment with no command name counts as a command.
func TestAssignmentUpdatesPipelineStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		{Yes, "0"},
		{No, "1 0"},
	} {
		out, _ := run(t, `false | true; x=1; echo "${P[@]}"`, named("P", func(s *Semantics) {
			s.AssignmentUpdatesPipelineStatus = tc.a
		}))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: got %q, want %q", tc.a, strings.TrimSpace(out), tc.want)
		}
	}
	// Every other shape of command updates it whatever the answer, so nothing
	// is asked for those.
	for _, a := range []Answer{Yes, No, Unspecified} {
		out, _ := run(t, `false | true; :; echo "${P[@]}"`, named("P", func(s *Semantics) {
			s.AssignmentUpdatesPipelineStatus = a
		}))
		if strings.TrimSpace(out) != "0" {
			t.Errorf("%v: a builtin gave %q, want 0 with no dialect needed", a, strings.TrimSpace(out))
		}
	}
}

// Whether `unset` is permanent. This is the opposite of what a produced
// *scalar* does, where unset ends it everywhere, which is why it is asked
// rather than inherited from the rule Dynamic follows.
func TestUnsetEndsTheProducedPipelineStatusIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		a    Answer
		want string
	}{
		{Yes, "[]"},
		{No, "[1 0]"},
	} {
		out, _ := run(t, `unset P; false | true; echo "[${P[@]}]"`, named("P", func(s *Semantics) {
			s.UnsetEndsTheProducedPipelineStatus = tc.a
		}))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%v: got %q, want %q", tc.a, strings.TrimSpace(out), tc.want)
		}
	}
}

// What a plain `$a` gives when `a` is an array. A rule about arrays rather
// than about this record, measured here because this is the array every shell
// that has one builds without being asked.
func TestArrayScalarIsTheWholeArrayIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		{"joined", Yes, `a=(x y z); echo "[$a]"`, "[x y z]"},
		{"first element", No, `a=(x y z); echo "[$a]"`, "[x]"},
		{"joined, on the record", Yes, `false | true; echo "[$P]"`, "[1 0]"},
		{"first element, on the record", No, `false | true; echo "[$P]"`, "[1]"},
		// With none or one element the two agree, so no dialect is needed —
		// and building an array is not a question about how it flattens.
		{"one element needs no dialect", Unspecified, `a=(x); echo "[$a]"`, "[x]"},
		{"building a long one needs no dialect", Unspecified, `a=(x y z); echo "[${a[@]}]"`, "[x y z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, named("P", func(s *Semantics) {
				s.ArrayScalarIsTheWholeArray = tc.a
			}))
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("got %q, want %q", strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A shell with no name for the record must not be asked how to maintain one.
// Without the guard, the bare core refused every `x=1` over a difference
// nothing in that shell could observe.
func TestNoNameAsksNoAxis(t *testing.T) {
	f, err := syntax.Parse(`false | true; x=1; echo done`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	// The strict core: every axis unanswered, so anything asked is reported.
	s := PosixSemantics()
	r := &Runner{Stdout: &out, Stderr: &out, Semantics: &s}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "done\n" {
		t.Errorf("got %q, want no dialect to be needed", got)
	}
}
