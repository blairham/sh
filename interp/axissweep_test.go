// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// TestTheSweepHookIsNotInAShippedBuild.
//
// The axis sweep moves a field of the vector in a running shell, which is a
// seam into the interpreter's settings, so the shipped binary must not have
// it: a value a person could set would let anything that can reach the
// environment change what the shell *means*. The hook is behind a build tag
// for that reason, and this is the assertion that the tag is doing its job —
// the variable is set here, in an ordinary build, and the vector does not
// move.
//
// It is deliberately not a test of the sweep. What the hook does under
// -tags shaxissweep is exercised by running the sweep, which is the only
// thing that builds it.
func TestTheSweepHookIsNotInAShippedBuild(t *testing.T) {
	t.Setenv("SH_AXIS_MUTATION", "SplitParamExpansion=2")
	want := CoreSemantics()
	want.SplitParamExpansion = Yes
	r := &Runner{Semantics: &want}
	if got := r.sem().SplitParamExpansion; got != Yes {
		t.Fatalf("an environment variable moved an axis in an ordinary build: SplitParamExpansion is %v", got)
	}
	if got := (&Runner{}).sem().SplitParamExpansion; got != CoreSemantics().SplitParamExpansion {
		t.Fatalf("an environment variable moved the default vector: SplitParamExpansion is %v", got)
	}
}
