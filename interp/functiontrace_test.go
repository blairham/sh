// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate half of the trace mark, graded without any dialect in front
// of it — dialect/zsh/functiontrace_test.go is the column's own rows, and a
// case here passing proves nothing about those.
//
// What this file is for is the two things the seam decides and the letters do
// not: that a shell naming **no** trace letters is untouched by any of it,
// and that the letters are read from Semantics rather than spelled here.

// traceSem is the permissive base with the two letter sets answered and the
// trace restored at a call, which is one column's answer and not the default.
func traceSem() Semantics {
	s := permissive()
	s.DeclareOptions = "aAfFgilprxtT"
	s.FunctionsOptions = "mtT"
	s.UnfunctionOptions = "m"
	s.UnsetOptions = "vfm"
	s.BadOptionToSpecialBuiltinFatal = No
	s.FunctionTraceLetters = "tT"
	s.FunctionTraceLettersBoundToTheBody = "T"
	s.FunctionCallRestoresTheTrace = Yes
	return s
}

func withTraceLetters(s Semantics) func(*Runner) {
	return func(r *Runner) {
		withSem(s)(r)
		r.Register("functions", FunctionsBuiltin())
	}
}

// A shell that names no trace letters is untouched: the letters are refused
// as they were, nothing is recorded, and a call changes no option.
//
// This is the row that keeps the feature out of the three columns that have
// not got it, and it is the one a table keyed on a letter rather than on the
// field would fail.
func TestAShellWithNoTraceLettersIsUntouched(t *testing.T) {
	s := traceSem()
	s.FunctionTraceLetters, s.FunctionTraceLettersBoundToTheBody = "", ""
	s.FunctionCallRestoresTheTrace = No
	out, _ := run(t, "f(){ echo A; }\nfunctions -t f\nf\n", withTraceLetters(s))
	if strings.Contains(out, "+") {
		t.Errorf("a shell naming no trace letters traced anyway: %q", out)
	}
	// And the option a call leaves behind is the caller's, not a restored
	// one, where the axis says so.
	out, _ = run(t, "f(){ set -x; }\nf\necho after\nset +x\n", withTraceLetters(s))
	if !strings.Contains(out, "+ echo after") {
		t.Errorf("a shell that does not restore the trace lost it anyway: %q", out)
	}
}

// The letters come from Semantics, so a shell spelling different ones gets
// the same feature under them.
func TestTheTraceLettersAreTheDialectsOwn(t *testing.T) {
	s := traceSem()
	s.DeclareOptions, s.FunctionsOptions = "aAfFgilprxvV", "mvV"
	s.FunctionTraceLetters, s.FunctionTraceLettersBoundToTheBody = "vV", "V"
	out, _ := run(t, "g(){ echo G; }\nf(){ g; }\nfunctions -v f\nf\n", withTraceLetters(s))
	if !strings.Contains(out, "+ echo G") {
		t.Errorf("the dialect's own letter did not trace: %q", out)
	}
	out, _ = run(t, "g(){ echo G; }\nf(){ g; }\nfunctions -V f\nf\n", withTraceLetters(s))
	if strings.Contains(out, "+ echo G") {
		t.Errorf("the dialect's own bounded letter reached the callee: %q", out)
	}
	// And the letters it no longer spells are refused rather than silently
	// meaning what they used to.
	_, st := run(t, "f(){ :; }\nfunctions -t f\n", withTraceLetters(s))
	if st == 0 {
		t.Errorf("a letter this dialect does not spell was taken, status %d", st)
	}
}

// The restore is the trace and not the options: a call puts `xtrace` back and
// leaves everything else where the body left it.
func TestTheRestoreIsTheTraceAlone(t *testing.T) {
	out, _ := run(t, "f(){ set -x; }\nf\necho after\n", withTraceLetters(traceSem()))
	if strings.Contains(out, "+ echo after") {
		t.Errorf("the trace outlived the call that turned it on: %q", out)
	}
	// The other half, and it is the one that says "the trace alone" rather
	// than "options are local to a call": a different option turned on in
	// the same place is still on afterwards. `$-` is read rather than a
	// builtin's listing so that the probe needs nothing on PATH — a case
	// whose command is missing prints nothing and passes.
	const flag = "f(){ set -u; }\nf\ncase $- in *u*) echo KEPT;; *) echo LOST;; esac\n"
	out, _ = run(t, flag, withTraceLetters(traceSem()))
	if strings.TrimSpace(out) != "KEPT" {
		t.Errorf("a call restored an option that is not the trace: %q, want KEPT", out)
	}
}

// A mark is on the body and not on the name: a definition replaces it and a
// removal takes it away.
func TestTheMarkBelongsToTheBody(t *testing.T) {
	for _, src := range []string{
		"f(){ echo A; }\nfunctions -t f\nf(){ echo B; }\nf\n",
		"f(){ echo A; }\nfunctions -t f\nunset -f f\nf(){ echo A; }\nf\n",
	} {
		out, _ := run(t, src, withTraceLetters(traceSem()))
		if strings.Contains(out, "+") {
			t.Errorf("%q kept its mark across a redefinition: %q", src, out)
		}
	}
}
