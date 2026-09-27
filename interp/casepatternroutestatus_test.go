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

// One dialect ends the shell at nought for a pattern it will not compile in a
// `case` arm when the program was handed over as an argument, and at its
// ordinary fatal status when the program came from a file or from standard
// input.
//
// The axis is named here and the shell that sets it is not. Three things have
// to be true at once for the answer to be reached — the surface, the route and
// the field — and the rows below move one of them at a time, because a row
// that varied all three could not say which one decides.
func TestARefusedCaseArmPatternMayAnswerByHowTheShellStarted(t *testing.T) {
	for _, c := range []struct {
		name   string
		src    string
		route  Route
		nought bool
		want   int
	}{
		{"a command string, with the field on", caseArm, RouteCommandString, true, 0},
		{"a script file, with the field on", caseArm, RouteScriptFile, true, 1},
		{"standard input, with the field on", caseArm, RouteStandardInput, true, 1},
		{"a command string, with the field off", caseArm, RouteCommandString, false, 1},
		{"a script file, with the field off", caseArm, RouteScriptFile, false, 1},

		// The surface, held against the same route and the same field: a
		// condition has a status of its own and keeps it on both routes.
		// It is the only other surface this vector can reach — an element
		// filter and an `(r)` subscript are constructs the core does not
		// have — so the rest of the surface question is a row in the
		// dialect package that does have them.
		{"a condition, which has its own status", condArm, RouteCommandString, true, 2},
		{"a condition from a file", condArm, RouteScriptFile, true, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := caseArmStatus(t, c.src, c.route, c.nought); got != c.want {
				t.Errorf("%q on %v: status = %d, want %d", c.src, c.route, got, c.want)
			}
		})
	}
}

// And the answer belongs to the shell that was handed the string rather than
// to every runner under it: a function is the same shell and keeps it, a
// subshell and a command substitution are copies and do not, and borrowed text
// is a boundary of its own and does not either.
//
// The set is the shape. A subshell row alone cannot tell "a boundary drops it"
// from "only the outermost statement carries it", and the function row is what
// says the discriminator is the boundary rather than the frame.
func TestTheCaseArmNoughtBelongsToTheShellHandedTheString(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		{"the shell itself", caseArm, 0},
		{"a function it calls", "f() { " + strings.TrimSuffix(caseArm, "\n") + " }\nf\n", 0},
		{"a subshell", "( case '[a' in ([a) :;; esac )\n", 1},
		{"a command substitution", "v=$( case '[a' in ([a) :;; esac )\n", 1},
		{"borrowed text", "eval 'case \"[a\" in ([a) :;; esac'\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := caseArmStatus(t, c.src, RouteCommandString, true); got != c.want {
				t.Errorf("%q: status = %d, want %d", c.src, got, c.want)
			}
		})
	}
}

// Nothing after the refusal runs, on either route and at either answer — so
// what the status costs a caller is the status, and a row that read output
// would be reading the same silence in every cell.
func TestARefusedCaseArmPatternStillEndsTheShell(t *testing.T) {
	for _, route := range []Route{RouteCommandString, RouteScriptFile} {
		var buf strings.Builder
		runCaseArm(t, &buf, "case '[a' in ([a) echo ONE;; (*) echo STAR;; esac\necho AFTER\n", route, true)
		if strings.Contains(buf.String(), "AFTER") || strings.Contains(buf.String(), "STAR") {
			t.Errorf("%v: output = %q, want nothing after the complaint", route, buf.String())
		}
		if !strings.Contains(buf.String(), "[a") {
			t.Errorf("%v: output = %q, want the refusal to name the pattern", route, buf.String())
		}
	}
}

const (
	caseArm = "case '[a' in ([a) :;; esac\n"
	condArm = "[[ '[a' == [a ]]\n"
)

func caseArmStatus(t *testing.T, src string, route Route, nought bool) int {
	t.Helper()
	var buf strings.Builder
	return runCaseArm(t, &buf, src, route, nought)
}

// runCaseArm runs one snippet under a vector that refuses an unterminated
// bracket outright, which is the only reading of that axis this question can
// be asked under: a dialect that reads the bracket as a literal never refuses
// the pattern and so never reaches a status at all.
func runCaseArm(t *testing.T, buf *strings.Builder, src string, route Route, nought bool) int {
	t.Helper()
	sem := PosixSemantics()
	sem.FatalErrorStatusIsOne = Yes
	sem.UnterminatedBracket = BracketBadPattern
	sem.UnterminatedBracketAfterASubExpression = BracketBadPattern
	dg := Diagnostics{CasePatternRefusalEndsACommandStringAtNought: nought}
	dial := syntax.Core()
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh", Route: route,
		Dialect: &dial, Stdout: buf, Stderr: buf,
	})
	f, err := syntax.Parse(src, dial)
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return st
}
