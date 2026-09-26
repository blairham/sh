// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The observer a dialect installs to be told about every definition — see
// Runner.AtFunctionDefinition. An extension point and not an axis, so this
// names the seam and no shell.
//
// **Every** definition, including one that adds no name: a redefinition is
// what a mark keyed on the function has to see, and a difference between the
// name sets around a piece of code cannot.
func TestAtFunctionDefinitionIsToldAboutEveryDefinition(t *testing.T) {
	var seen []string
	src := "f() { :; }\ng() { :; }\nf() { echo again; }\nf\n"
	out, st := run(t, src, func(r *Runner) {
		r.AtFunctionDefinition(func(_ *Runner, name string) {
			seen = append(seen, name)
		})
	})
	if st != 0 || out != "again\n" {
		t.Fatalf("out %q status %d, want the redefined body to have run", out, st)
	}
	if got := strings.Join(seen, ","); got != "f,g,f" {
		t.Errorf("observed %q, want %q — the redefinition is a definition too", got, "f,g,f")
	}
}

// The name arrives before the body can be called, which is what makes a mark
// taken here good for the first call as well as for every later one.
func TestAtFunctionDefinitionRunsBeforeTheFirstCall(t *testing.T) {
	marked := map[string]bool{}
	src := "f() { :; }\nf\n"
	calledWhileMarked := false
	out, st := run(t, src, func(r *Runner) {
		r.AtFunctionDefinition(func(_ *Runner, name string) { marked[name] = true })
		r.AtEveryFunctionCall(func(r *Runner) func() {
			calledWhileMarked = marked[r.RunningFunction()]
			return nil
		})
	})
	if st != 0 || out != "" {
		t.Fatalf("out %q status %d, want a quiet run", out, st)
	}
	if !calledWhileMarked {
		t.Errorf("the call ran with no mark, so a definition-time record misses the first call")
	}
}

// A definition made through the builtin seam is a definition, so a dialect
// that defines functions in Go is told about those too.
func TestAtFunctionDefinitionCoversTheBuiltinSeam(t *testing.T) {
	var seen []string
	_, _ = run(t, "echo hi\n", func(r *Runner) {
		r.AtFunctionDefinition(func(_ *Runner, name string) { seen = append(seen, name) })
		if !r.DefineFunction("made", "echo made") {
			t.Fatal("DefineFunction refused a body it should have taken")
		}
	})
	if got := strings.Join(seen, ","); got != "made" {
		t.Errorf("observed %q, want %q", got, "made")
	}
}

// RunningFunction is the innermost body and is empty outside one, which is
// the reading a hook at a call's own boundary needs.
func TestRunningFunctionIsTheInnermostBody(t *testing.T) {
	var at []string
	src := "inner() { :; }\nouter() { inner; }\nouter\n"
	_, _ = run(t, src, func(r *Runner) {
		r.AtEveryFunctionCall(func(r *Runner) func() {
			at = append(at, r.RunningFunction())
			return func() { at = append(at, "<-"+r.RunningFunction()) }
		})
	})
	if got := strings.Join(at, ","); got != "outer,inner,<-inner,<-outer" {
		t.Errorf("observed %q, want the innermost name at each boundary", got)
	}
}
