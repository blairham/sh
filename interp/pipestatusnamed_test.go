// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"slices"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The record is a *produced parameter*, and the three unions that answer what
// this shell has did not know it.
//
// Tests name the axis and never a shell; interp/pipestatus.go has the
// measurement. The fault it is written for is two readings of one question
// disagreeing inside one runner: the name expanded and `${+name}` answered 1,
// and a description of the same name said the shell did not have it (#4865).
//
// `Dynamic`, `DynamicArrays` and `DynamicAssocs` are the other three
// producers and each is a map a dialect fills; this one is a field, because
// the record belongs to the core and only its *name* is the dialect's. That
// is the whole reason it was missed, so the controls below are the other
// three answering correctly in the same runner.
func recordedRunner(t *testing.T, src, name string, tune func(*Semantics)) *Runner {
	t.Helper()
	var captured *Runner
	run(t, src, func(r *Runner) {
		named(name, tune)(r)
		// A producer of each of the other three kinds, so that "this runner
		// can describe a produced parameter at all" is answered in the same
		// run as "it can describe this one".
		r.SetDynamic("SCAL", func(*Runner) string { return "s" })
		r.SetDynamicArray("ARR", func(*Runner) []string { return []string{"a"} })
		captured = r
	})
	if captured == nil {
		t.Fatal("the runner was never handed to setup, so nothing below is asked of a real shell")
	}
	return captured
}

func TestTheProducedPipelineStatusIsAParameterTheShellHas(t *testing.T) {
	r := recordedRunner(t, `false | true`, "P", nil)

	if !r.ParameterIsNamed("P") {
		t.Error("ParameterIsNamed(P) = false, want true")
	}
	a, ok := r.ParameterAttributes("P")
	if !ok {
		t.Fatal("ParameterAttributes(P) reports no such parameter")
	}
	if a.Kind != ArrayParameter {
		t.Errorf("ParameterAttributes(P).Kind = %v, want ArrayParameter", a.Kind)
	}
	if !a.Provided {
		t.Errorf("ParameterAttributes(P) = %+v, want it provided by the shell", a)
	}
	if !r.DynamicParameter("P") {
		t.Error("DynamicParameter(P) = false, want true")
	}
	if names := r.ParameterNames(); !slices.Contains(names, "P") {
		t.Errorf("ParameterNames() = %v, want P among them", names)
	}

	// The controls, in the same runner: the three producers that are maps
	// were already right, which is what says this is one union's gap and not
	// a description that cannot see a producer.
	for _, name := range []string{"SCAL", "ARR"} {
		if _, ok := r.ParameterAttributes(name); !ok {
			t.Errorf("ParameterAttributes(%s) reports no such parameter, so the control is broken", name)
		}
	}
	// And the other control: a name nothing produces is still absent.
	if _, ok := r.ParameterAttributes("NOSUCH"); ok {
		t.Error("ParameterAttributes(NOSUCH) reports a parameter, so a positive here proves nothing")
	}
}

// Without a dialect to name the record there is no parameter, which is what
// keeps the answer above from being "any name at all".
func TestAnUnnamedPipelineStatusIsNotAParameter(t *testing.T) {
	r := recordedRunner(t, `false | true`, "", nil)
	for _, name := range []string{"P", "PIPESTATUS", "pipestatus"} {
		if r.ParameterIsNamed(name) || r.DynamicParameter(name) {
			t.Errorf("%s is a parameter of a shell with no name for the record", name)
		}
		if _, ok := r.ParameterAttributes(name); ok {
			t.Errorf("ParameterAttributes(%s) answers in a shell with no name for the record", name)
		}
	}
}

// A removed name is not described, and the record is no exception under the
// answer that ends it.
//
// **Only that answer is asserted here, and the omission is the point.** The
// other one keeps the producer alive across the removal, and the three unions
// above still answer "no such parameter" for it — each opens with the
// `removed` check, in front of anything that could ask a producer. That is
// right for every other producer in this engine, which is measured rather than
// assumed: a removed dynamic scalar and a removed dynamic array both stop
// being parameters under either answer, and so does an ordinary variable. It
// is wrong for this one name under one answer, and it is #4877 — left alone
// here because the removal rule is load-bearing for every other name and
// moving it is not what #4865 is about.
func TestARemovedRecordIsNotDescribedUnderTheAnswerThatEndsIt(t *testing.T) {
	r := recordedRunner(t, `false | true; unset P`, "P", func(s *Semantics) {
		s.UnsetEndsTheProducedPipelineStatus = Yes
	})
	if _, ok := r.ParameterAttributes("P"); ok {
		t.Error("after unset, ParameterAttributes(P) answers, want no such parameter")
	}
	if r.ParameterIsNamed("P") {
		t.Error("after unset, ParameterIsNamed(P) = true, want false")
	}
	if slices.Contains(r.ParameterNames(), "P") {
		t.Error("after unset, P is among ParameterNames(), want it gone")
	}
	// The control: with nothing removed, the same runner describes it — so
	// the three answers above are the removal and not a shell that cannot
	// describe the record at all.
	live := recordedRunner(t, `false | true`, "P", func(s *Semantics) {
		s.UnsetEndsTheProducedPipelineStatus = Yes
	})
	if _, ok := live.ParameterAttributes("P"); !ok {
		t.Error("without the unset, ParameterAttributes(P) reports no such parameter")
	}
	// And the neighbors, so that "a removed producer stops being a
	// parameter" is stated as the engine's rule rather than as this name's.
	for _, name := range []string{"SCAL", "ARR"} {
		gone := recordedRunner(t, `unset `+name, "P", nil)
		if _, ok := gone.ParameterAttributes(name); ok {
			t.Errorf("after unset, ParameterAttributes(%s) answers, want no such parameter", name)
		}
	}
}
