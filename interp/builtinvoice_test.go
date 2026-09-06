// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Two things a registered builtin needs and could not reach: a diagnostic
// that is not its own, and whether the shell *produces* a parameter.
//
// Both are here because a dialect builtin found them missing, and both are
// about a distinction the calling builtin cannot make for itself: the
// builtin's name in a location is this package's to write, and what counts as
// a produced parameter is this package's to know.

func voiceRunner(t *testing.T, out, errs *strings.Builder, names bool) *Runner {
	t.Helper()
	dg := Diagnostics{
		Location:               LocationTightLine,
		BuiltinLocation:        LocationTightLine,
		NamesBuiltinInLocation: names,
	}
	return newTestRunner(t, &Runner{
		Stdout: out, Stderr: errs, Diagnostics: &dg, Name: "testsh",
	})
}

// The dialect that puts a builtin's name in the location leaves it out for a
// complaint the builtin did not make. One command writes both kinds, and the
// location is the only thing that says which — so the two are asserted
// side by side, as whole lines.
func TestADiagnosticThatIsNotTheBuiltinsOwnLeavesItsNameOut(t *testing.T) {
	var out, errs strings.Builder
	r := voiceRunner(t, &out, &errs, true)
	r.Register("loader", func(rr *Runner, _ context.Context, _ []string) int {
		rr.Diagnosef("bad option: -q\n")
		rr.DiagnoseAsTheShellf("failed to load module `m'\n")
		return 1
	})
	f, err := syntax.Parse("loader\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "testsh:loader:1: bad option: -q\ntestsh:1: failed to load module `m'\n"
	if errs.String() != want {
		t.Errorf("diagnostics = %q, want %q", errs.String(), want)
	}
}

// And the name goes back afterwards: the second message must not silence the
// third. This is what a saved-and-restored field is for, and a nil restore
// would look right in the test above and wrong here.
func TestTheBuiltinsNameComesBackAfterALoudersDiagnostic(t *testing.T) {
	var out, errs strings.Builder
	r := voiceRunner(t, &out, &errs, true)
	r.Register("loader", func(rr *Runner, _ context.Context, _ []string) int {
		rr.DiagnoseAsTheShellf("first\n")
		rr.Diagnosef("second\n")
		return 0
	})
	f, err := syntax.Parse("loader\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "testsh:1: first\ntestsh:loader:1: second\n"
	if errs.String() != want {
		t.Errorf("diagnostics = %q, want %q", errs.String(), want)
	}
}

// In a dialect that does not name the builtin in the location, the two are
// the same message — which is the point: nothing here invents a distinction
// the dialect does not draw.
func TestADialectThatNamesNoBuiltinWritesBothTheSameWay(t *testing.T) {
	var out, errs strings.Builder
	r := voiceRunner(t, &out, &errs, false)
	r.Register("loader", func(rr *Runner, _ context.Context, _ []string) int {
		rr.Diagnosef("one\n")
		rr.DiagnoseAsTheShellf("two\n")
		return 0
	})
	f, err := syntax.Parse("loader\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "testsh:1: one\ntestsh:1: two\n"
	if errs.String() != want {
		t.Errorf("diagnostics = %q, want %q", errs.String(), want)
	}
}

// A produced parameter is one the shell generates on being read, and a
// variable a script assigned is not one however it is spelled. The
// distinction is the whole reason this exists: a builtin asking whether the
// shell *provides* `options` must not be fooled by a script's `options=(a)`.
func TestAProducedParameterIsNotAVariableAScriptSet(t *testing.T) {
	var out, errs strings.Builder
	r := voiceRunner(t, &out, &errs, true)
	r.SetDynamic("MADE", func(*Runner) string { return "x" })
	r.SetDynamicArray("MADEARR", func(*Runner) []string { return []string{"x"} })
	r.Register("ask", func(rr *Runner, _ context.Context, args []string) int {
		for _, name := range args {
			if rr.DynamicParameter(name) {
				rr.Diagnosef("%s produced\n", name)
				continue
			}
			rr.Diagnosef("%s not produced\n", name)
		}
		return 0
	})
	f, err := syntax.Parse("assigned=1; arr=(a b); ask MADE MADEARR assigned arr never\n", syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := "testsh:ask:1: MADE produced\ntestsh:ask:1: MADEARR produced\n" +
		"testsh:ask:1: assigned not produced\ntestsh:ask:1: arr not produced\n" +
		"testsh:ask:1: never not produced\n"
	if errs.String() != want {
		t.Errorf("answers = %q, want %q", errs.String(), want)
	}
}
