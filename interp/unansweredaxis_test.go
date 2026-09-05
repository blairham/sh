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

// refuseAnAxis runs a snippet that reaches an axis nothing answered and returns
// what the shell said about it.
//
// `test a == a` is the axis, chosen because it needs no files, no processes and
// no dialect: the permissive vector leaves TestAcceptsDoubleEqual unanswered
// like every other Unspecified field, which is exactly the state this is about.
func refuseAnAxis(t *testing.T, remedy string) string {
	t.Helper()
	f, err := syntax.Parse(`test a == a`, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	sem := permissive()
	sem.TestAcceptsDoubleEqual = Unspecified
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
		Name: "testsh", AxisRemedy: remedy,
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

// TestAnUnansweredAxisCarriesTheCallersRemedy is the contract AxisRemedy exists
// for. Refusing the axis is the substrate's central discipline and does not
// move; what it owed a person was a sentence about what to do next, and only
// the embedder can write that sentence, because it names a flag this package
// has never heard of.
func TestAnUnansweredAxisCarriesTheCallersRemedy(t *testing.T) {
	t.Parallel()
	const remedy = "say which shell you meant"
	got := refuseAnAxis(t, remedy)
	if !strings.Contains(got, "the shells disagree here and no dialect was chosen") {
		t.Fatalf("output = %q, want the axis refused", got)
	}
	if !strings.Contains(got, remedy) {
		t.Errorf("output = %q, want it to carry %q", got, remedy)
	}
}

// TestAnUnansweredAxisInventsNoRemedy is the other half, and the half that
// keeps this package honest: a Runner embedded in a program with no dialect
// flag has nothing true to advise, so it says nothing rather than pointing at
// an option its host does not have.
func TestAnUnansweredAxisInventsNoRemedy(t *testing.T) {
	t.Parallel()
	got := refuseAnAxis(t, "")
	if !strings.Contains(got, "the shells disagree here and no dialect was chosen") {
		t.Fatalf("output = %q, want the axis refused", got)
	}
	// The separator belongs to the remedy. Without one the sentence ends where
	// it always did, so nothing that reads these diagnostics has to change.
	if strings.Contains(got, "chosen;") {
		t.Errorf("output = %q: with no remedy there is nothing to separate", got)
	}
}

// TestEveryUnansweredAxisIsWordedTheSameWay is why the phrase became a function
// rather than staying a literal at each of the twenty-odd refusals. The
// sentence is the discipline said out loud; spelled by hand in twenty places it
// is twenty chances for one of them to drift, and a remedy appended by hand in
// twenty places is a remedy appended in nineteen.
func TestEveryUnansweredAxisIsWordedTheSameWay(t *testing.T) {
	t.Parallel()
	const remedy = "say which shell you meant"
	// Under the core vector every disputed axis is unanswered by
	// construction, which is the state this is about. Four refusals raised in
	// four different files: a builtin's operator, two listings and a name.
	for _, src := range []string{
		`test a == a`,
		`set`,
		`local`,
		`alias`,
	} {
		t.Run(src, func(t *testing.T) {
			t.Parallel()
			f, err := syntax.Parse(src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", src, err)
			}
			var buf bytes.Buffer
			sem := CoreSemantics()
			dg := CoreDiagnostics()
			r := newTestRunner(t, &Runner{
				Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg,
				Name: "testsh", AxisRemedy: remedy,
			})
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatalf("run %q: %v", src, err)
			}
			got := buf.String()
			if !strings.Contains(got, "no dialect was chosen; "+remedy) {
				t.Errorf("output = %q, want the remedy directly after the refusal", got)
			}
		})
	}
}
