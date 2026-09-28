// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// bounded names the parameter that moves the bound and words the refusal the
// way the column that has both words it.
func bounded() func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		r.Semantics = &sem
		dg := Diagnostics{
			FunctionNestingLimit: "%[1]s: maximum function nesting level exceeded (%[2]d)",
		}
		r.Diagnostics = &dg
		r.SetFunctionNesting(FunctionNesting{Parameter: "FUNCNEST"})
	}
}

// boundedZshWise is the same parameter read the other column's way: a stored
// zero is a bound, and the refusal ends the script.
func boundedZshWise() func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		r.Semantics = &sem
		dg := Diagnostics{
			FunctionNestingLimit: "%[1]s: maximum nested function level reached; increase FUNCNEST?",
		}
		r.Diagnostics = &dg
		r.SetFunctionNesting(FunctionNesting{
			Parameter:            "FUNCNEST",
			ZeroIsABound:         true,
			RefusalEndsTheScript: true,
		})
	}
}

// TestAFunctionCallIsBoundedByTheParameter — a call the bound will not admit is
// reported, the input line goes with it, and the shell carries on at 1.
//
// Measured against bash 5.3.20, 2026-09-23, script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`. Nothing here bounded a call at all before
// #4174: a recursive function ran to the substrate's own ceiling of 256, so a
// script that set the parameter to hold a runaway short got no refusal and the
// wrong count in the variable it was watching.
func TestAFunctionCallIsBoundedByTheParameter(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The bound lets that many calls stand and refuses the next, so a
		// counter in the body reaches exactly the bound.
		{
			"the count reaches the bound",
			"f() { d=$((d+1)); f; return 7; }\nd=0; FUNCNEST=5; f\necho \"st=$? d=$d\"",
			"f: maximum function nesting level exceeded (5)\nst=1 d=5",
		},
		// The rest of the *line* goes with the refusal, which is what makes it
		// a give-up rather than a status: the `||` never runs and the next
		// line does.
		{
			"the line is given up",
			"f() { f; }\nFUNCNEST=2\nf || echo or\necho \"st=$?\"",
			"f: maximum function nesting level exceeded (2)\nst=1",
		},
		{
			"a loop around it is given up too",
			"f() { f; }\nFUNCNEST=2\nfor i in 1 2; do f; echo body; done\necho \"st=$?\"",
			"f: maximum function nesting level exceeded (2)\nst=1",
		},
		// The name is the callee's and not the caller's, which a chain of two
		// is what distinguishes: `g` calls `h` calls `g`, and it is the second
		// `g` that could not be entered.
		{
			"the callee is named",
			"g() { h; }\nh() { g; }\nFUNCNEST=2; g\necho \"st=$?\"",
			"g: maximum function nesting level exceeded (2)\nst=1",
		},
		// Only a positive integer is a bound. Each of these runs to the
		// function's own base case at its own status, measured on all four.
		{"zero is no bound", boundJunk("0"), "st=7 d=6"},
		{"empty is no bound", boundJunk(""), "st=7 d=6"},
		{"a word is no bound", boundJunk("abc"), "st=7 d=6"},
		{"a negative is no bound", boundJunk("-2"), "st=7 d=6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, bounded())
			// The location the shell puts in front of a diagnostic is not what
			// these rows are about, so the want is the tail of each line.
			got := strings.TrimSpace(strings.ReplaceAll(out, "sh: ", ""))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// boundJunk is a recursion with a base case of its own, so a value that is no
// bound shows as the function finishing rather than as a hang. The status is
// the outermost frame's `return 7` and not the base case's 9, which is what
// says the whole chain came back rather than being unwound.
func boundJunk(value string) string {
	return "f() { d=$((d+1)); if [ $d -gt 5 ]; then return 9; fi; f; return 7; }\n" +
		"d=0; FUNCNEST=" + value + "; f; echo \"st=$? d=$d\""
}

// TestNoParameterMeansNoBoundOfTheScripts — a dialect that names no parameter
// is a shell whose bound is its own, and a script writing that name moves
// nothing.
//
// The control for the suite above: without it every row there would pass in a
// shell that bounded calls at five whatever anybody said.
func TestNoParameterMeansNoBoundOfTheScripts(t *testing.T) {
	src := boundJunk("2")
	out, _ := run(t, src, func(r *Runner) {
		sem := permissive()
		r.Semantics = &sem
	})
	if got := strings.TrimSpace(out); got != "st=7 d=6" {
		t.Errorf("got %q, want the function's own base case", got)
	}
}

// TestZeroIsABoundWhereTheDialectSaysSo — the one row the two columns that
// read this parameter part over.
//
// Measured 2026-09-27, script files under `env -i PATH=/usr/bin:/bin`, the
// same recursion in both: zsh 5.9.2 refuses the *first* call at
// `FUNCNEST=0` where bash 5.3.20 runs to the function's own base case. The
// bash rows are the suite above, which is what makes this a pair rather
// than an assertion about one shell.
//
// A negative is the control and it is in here on purpose: both columns read
// it as no bound, so a change that made zero a bound by making every
// unreadable value one would fail this row.
func TestZeroIsABoundWhereTheDialectSaysSo(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"zero admits no call at all",
			boundJunk("0"),
			"f: maximum nested function level reached; increase FUNCNEST?",
		},
		// An empty value and a word reach this through the *integer*
		// attribute in the shell being modeled, which stores 0 for both
		// before the bound is ever read. Written here as the stored value,
		// because this is the core's question and the attribute is the
		// dialect's.
		{"a negative is still no bound", boundJunk("-2"), "st=7 d=6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, boundedZshWise())
			got := strings.TrimSpace(strings.ReplaceAll(out, "sh: ", ""))
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTheRefusalEndsTheScriptWhereTheDialectSaysSo — the fourth fact of
// #4905, and the one a parameter with the right letters would still get
// wrong.
//
// Measured 2026-09-27 in one run, `echo AFTER` on the line after the call:
// zsh 5.9.2 never reaches it and leaves at 1, where bash 5.3.20 runs it. The
// `||` is refused in both, which is what says this is the give-up growing
// rather than a different refusal.
func TestTheRefusalEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	src := "f() { f; }\nFUNCNEST=2\nf || echo or\necho AFTER"
	out, st := run(t, src, boundedZshWise())
	got := strings.TrimSpace(strings.ReplaceAll(out, "sh: ", ""))
	want := "f: maximum nested function level reached; increase FUNCNEST?"
	if got != want {
		t.Errorf("got %q, want %q — the next line must not run", got, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	// The control: the same source under the column that gives up the line
	// alone reaches the `echo`, so this test is not passing because the
	// runner stops at any refusal.
	out, _ = run(t, src, bounded())
	if !strings.Contains(out, "AFTER") {
		t.Errorf("got %q, want the next line to run where the dialect gives up the line alone", out)
	}
}
