// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// An assignment prefix in front of a *produced* parameter — one whose value
// comes from a producer rather than from the variable table — is taken back
// the way a prefix to any other name is.
//
// The route is what makes it its own file. An assignment to a produced name
// never reaches Vars: it becomes a message in Runner.assigned and, where the
// producer has a writer, a call to that writer. The take-back put the four
// stores back and told the producer nothing, so the seeding outlived the
// command it was written in front of (#2713).
//
// Named for the seam and not for a shell: the parameters this bites are the
// ones a dialect registers through SetDynamic, and every dialect with one has
// the hole.

func producedPrefixRunner(t *testing.T, out, errs *strings.Builder, sem *Semantics) *Runner {
	t.Helper()
	dg := Diagnostics{Location: LocationTightLine, BuiltinLocation: LocationTightLine}
	return newTestRunner(t, &Runner{
		Stdout: out, Stderr: errs, Diagnostics: &dg, Semantics: sem, Name: "testsh",
	})
}

func runProducedPrefix(t *testing.T, r *Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
}

// A producer that counts from whatever it was last assigned, which is the
// shape the parameters in the tree have: the message is read on each answer
// rather than acted on when it arrives.
func countsFromWhatItWasAssigned(r *Runner) string {
	if v, ok := r.Assigned("counter"); ok {
		return "seeded:" + v
	}
	return "unseeded"
}

func TestAPrefixToAProducedNameIsTakenBackAfterABuiltin(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	r := producedPrefixRunner(t, &out, &errs, &sem)
	r.SetDynamic("counter", countsFromWhatItWasAssigned)
	runProducedPrefix(t, r, `echo "in=[$counter]"
counter=100 echo "run"
echo "after=[$counter]"`)
	want := "in=[unseeded]\nrun\nafter=[unseeded]\n"
	if out.String() != want {
		t.Errorf("got %q, want %q — the message to the producer is the prefix too", out.String(), want)
	}
}

// And the prefix is visible *to* the command it stands in front of, which is
// the control: a take-back that ran too early would pass the test above and
// make the prefix do nothing at all.
func TestAPrefixToAProducedNameReachesTheCommandItPrefixes(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	r := producedPrefixRunner(t, &out, &errs, &sem)
	r.SetDynamic("counter", countsFromWhatItWasAssigned)
	r.Register("show", func(rr *Runner, _ context.Context, _ []string) int {
		_, _ = fmt.Fprintf(rr.Out(), "saw=[%s]\n", countsFromWhatItWasAssigned(rr))
		return 0
	})
	runProducedPrefix(t, r, `counter=7 show
echo "after=[$counter]"`)
	want := "saw=[seeded:7]\nafter=[unseeded]\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// A name the shell had already seeded goes back to *that* message rather than
// to none, which is what makes the record a tri-state rather than a flag.
func TestAPrefixToAnAlreadySeededProducedNameGoesBackToWhatItHeld(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	r := producedPrefixRunner(t, &out, &errs, &sem)
	r.SetDynamic("counter", countsFromWhatItWasAssigned)
	runProducedPrefix(t, r, `counter=3
counter=100 echo "run"
echo "after=[$counter]"`)
	want := "run\nafter=[seeded:3]\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// The function route asks an axis rather than restoring outright — a prefix in
// front of a function outlives the call in one reading — and the comparison
// that decides whether to ask it has to look at the message too. Comparing the
// four stores alone reported that nothing had moved, so the axis was never
// asked and the seeding stood under *both* readings.
func TestAPrefixToAProducedNameIsTakenBackAfterAFunctionWhereTheAxisSaysSo(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = No
	// The export answer is pinned to the reading that records `false`, so
	// the *only* thing that has moved about the name is the message to its
	// producer. Under the other reading the name gains the attribute for the
	// call, and that difference alone is enough to send the take-back
	// through — which would let a comparison that never looked at the
	// message pass this row for the wrong reason.
	sem.PrefixToAFunctionIsExported = No
	r := producedPrefixRunner(t, &out, &errs, &sem)
	r.SetDynamic("counter", countsFromWhatItWasAssigned)
	runProducedPrefix(t, r, `f(){ :; }
counter=100 f
echo "after=[$counter]"`)
	if out.String() != "after=[unseeded]\n" {
		t.Errorf("got %q, want the prefix taken back", out.String())
	}
}

// And stands where the other reading is chosen, which is the row that says the
// comparison reaches the axis rather than restoring on its own.
func TestAPrefixToAProducedNameStandsAfterAFunctionWhereTheAxisSaysSo(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	sem.AssignmentPrefixPersistsAfterAFunction = Yes
	sem.PrefixToAFunctionIsExported = No
	r := producedPrefixRunner(t, &out, &errs, &sem)
	r.SetDynamic("counter", countsFromWhatItWasAssigned)
	runProducedPrefix(t, r, `f(){ :; }
counter=100 f
echo "after=[$counter]"`)
	if out.String() != "after=[seeded:100]\n" {
		t.Errorf("got %q, want the prefix to persist", out.String())
	}
}

// A producer with a **writer** is told, and not merely un-recorded. The
// message is delivered when it arrives there — that is what a writer is for —
// so a take-back that only put Runner.assigned back would leave whatever the
// writer did standing.
func TestAPrefixToAProducedNameWithAWriterPutsTheProducerBack(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	r := producedPrefixRunner(t, &out, &errs, &sem)
	held := "original"
	r.SetDynamic("cell", func(*Runner) string { return held })
	r.SetDynamicWriter("cell", func(_ *Runner, value string) { held = value })
	runProducedPrefix(t, r, `cell=moved echo "run"
echo "after=[$cell]"`)
	want := "run\nafter=[original]\n"
	if out.String() != want {
		t.Errorf("got %q, want %q — the writer has to be sent the old value back", out.String(), want)
	}
	if held != "original" {
		t.Errorf("the producer's own state = %q, want it put back", held)
	}
}

// And the writer is reached while the command runs, which is the control on
// the row above.
func TestAPrefixToAProducedNameWithAWriterReachesTheCommand(t *testing.T) {
	var out, errs strings.Builder
	sem := permissive()
	r := producedPrefixRunner(t, &out, &errs, &sem)
	held := "original"
	r.SetDynamic("cell", func(*Runner) string { return held })
	r.SetDynamicWriter("cell", func(_ *Runner, value string) { held = value })
	r.Register("show", func(rr *Runner, _ context.Context, _ []string) int {
		_, _ = fmt.Fprintf(rr.Out(), "saw=[%s]\n", held)
		return 0
	})
	runProducedPrefix(t, r, `cell=moved show`)
	if out.String() != "saw=[moved]\n" {
		t.Errorf("got %q, want the writer reached for the command", out.String())
	}
}
