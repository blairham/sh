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

// And the nought is what the **refusal leaves**, not what a route decides: it
// survives every boundary that copies the shell, and only the two outermost
// routes overwrite it.
//
// Every row is run on both routes, and the pairing is what carries the claim.
// A subshell row on one route alone cannot tell "a boundary drops it" from
// "only the outermost statement carries it"; a subshell that answers nought
// from a *script file*, where the shell itself answers 1, can only be the
// refusal's own number surviving. The function row is the control on the
// other side — the same shell, a deeper frame, no boundary — and borrowed
// text is the row that keeps the rule from being "anything nested": an `eval`
// **reports** rather than copies, so it keeps the ordinary number, inside a
// subshell as well as out of one.
func TestTheCaseArmNoughtIsWhatTheRefusalLeaves(t *testing.T) {
	for _, c := range []struct {
		name             string
		src              string
		fromArg, fromFil int
	}{
		{"the shell itself", caseArm, 0, 1},
		{"a function it calls", "f() { " + strings.TrimSuffix(caseArm, "\n") + " }\nf\n", 0, 1},
		{"a subshell", "( case '[a' in ([a) :;; esac )\n", 0, 0},
		{"a subshell inside a function", "f() { ( case '[a' in ([a) :;; esac ) }\nf\n", 0, 0},
		{"a subshell inside a subshell", "( ( case '[a' in ([a) :;; esac ) )\n", 0, 0},
		{"a command substitution", "v=$( case '[a' in ([a) :;; esac )\n", 0, 0},
		{"borrowed text", "eval 'case \"[a\" in ([a) :;; esac'\n", 1, 1},
		{"borrowed text in a subshell", "( eval 'case \"[a\" in ([a) :;; esac' )\n", 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := caseArmStatus(t, c.src, RouteCommandString, true); got != c.fromArg {
				t.Errorf("%q from an argument: status = %d, want %d", c.src, got, c.fromArg)
			}
			if got := caseArmStatus(t, c.src, RouteScriptFile, true); got != c.fromFil {
				t.Errorf("%q from a file: status = %d, want %d", c.src, got, c.fromFil)
			}
		})
	}
}

// And every one of those cells is the dialect's ordinary fatal status with the
// field off, which is the control that says the field is what decides and not
// the boundary. Without it the subshell rows above would read the same for a
// shell that simply lost the status at a boundary.
func TestWithoutTheFieldEveryBoundaryCarriesTheOrdinaryStatus(t *testing.T) {
	for _, src := range []string{
		caseArm,
		"( case '[a' in ([a) :;; esac )\n",
		"v=$( case '[a' in ([a) :;; esac )\n",
		"( ( case '[a' in ([a) :;; esac ) )\n",
	} {
		for _, route := range []Route{RouteCommandString, RouteScriptFile} {
			if got := caseArmStatus(t, src, route, false); got != 1 {
				t.Errorf("%q on %v with the field off: status = %d, want 1", src, route, got)
			}
		}
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
	dg := Diagnostics{CasePatternRefusalLeavesNought: nought}
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
