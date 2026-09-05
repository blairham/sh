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

// `unset -f` is quiet in two of the panel and not in the other two, and the
// two that speak are answering different questions.
//
// One judges the *name*: `1x` could never be a function name, and it says so
// whether or not a function exists. The other reports its *table*: it
// complains about any name it does not hold, including a well formed one.
// Independent, which is what makes them two fields rather than one.
func TestUnsetFunctionSpeaksAboutTwoDifferentThings(t *testing.T) {
	for _, c := range []struct {
		name          string
		checks, warns Answer
		src           string
		want          string
		status        int
	}{
		{"quiet about both", No, No, "unset -f 1x\n", "", 0},
		{"quiet about a name that is merely undefined", No, No, "unset -f nosuch\n", "", 0},

		{"judging the name", Yes, No, "unset -f 1x\n", "invalid function name", 1},
		{
			// The name is fine, so the shell that judges names has nothing
			// to say — even though no such function exists.
			"and saying nothing about a good name that is undefined",
			Yes, No, "unset -f nosuch\n", "", 0,
		},

		{"reporting the table", No, Yes, "unset -f nosuch\n", "unset: nosuch: not found", 1},
		{
			// And the shell that reports its table says so about a name that
			// could never be a function name either, for its own reason.
			"including one that is not a name at all",
			No, Yes, "unset -f 1x\n", "unset: 1x: not found", 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := unsetFnRun(t, c.src, c.checks, c.warns)
			if c.want == "" {
				if out != "" {
					t.Errorf("said %q, want nothing", out)
				}
			} else if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Unsetting a function that is there is quiet in all four, and it really is
// unset afterwards — which the reporting must not get in the way of.
func TestUnsetFunctionThatIsThereIsQuietAndWorks(t *testing.T) {
	for _, c := range []struct {
		name          string
		checks, warns Answer
	}{
		{"quiet", No, No},
		{"judging names", Yes, No},
		{"reporting the table", No, Yes},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := unsetFnRun(t, "f() { echo ran; }\nunset -f f\nf\n", c.checks, c.warns)
			if strings.Contains(out, "ran") {
				t.Errorf("said %q, want the function gone", out)
			}
			if strings.Contains(out, "not found\nunset") {
				t.Errorf("said %q, want nothing said about unsetting it", out)
			}
			// The `f` afterwards is a command that does not exist, which is
			// a different complaint and the one that should be left.
			if st == 0 {
				t.Errorf("status = %d, want the later call to fail", st)
			}
		})
	}
}

// Each operand is answered for, and the status survives to the end.
func TestUnsetFunctionAnswersForEachOperand(t *testing.T) {
	out, st := unsetFnRun(t, "unset -f 1x nosuch\n", No, Yes)
	if n := strings.Count(out, "not found"); n != 2 {
		t.Errorf("said %q, want both operands reported", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

func unsetFnRun(t *testing.T, src string, checks, warns Answer) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	sem.UnsetFunctionChecksTheName = checks
	sem.UnsetFunctionReportsMissing = warns
	sem.LoneDashIsAnOption = No
	r := newTestRunner(t, &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh"})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
