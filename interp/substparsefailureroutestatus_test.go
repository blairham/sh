// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `$( … )` body that will not parse ends the script with a number one
// column takes from **the route the program came from**.
//
// It is a third answer rather than a second. bash exits 127 for such a body
// when the program was a `-c` string, 2 when the identical text came from a
// file or from standard input, and 2 for a plain syntax error on the `-c`
// route — so neither the refusal's own number nor the route's stands for it.
// Measured 2026-09-26 under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdout
// and stderr discarded and `$?` taken immediately, against
// /opt/homebrew/bin/bash 5.3.20 (`not a Go executable` by `go version -m`);
// see [Diagnostics.SubstitutionParseFailureStatusFromCommandString] for the
// twelve rows and the four controls (#4697).
//
// The axis lives here and not on the boundary
// [Diagnostics.ExpansionFailureStatusFromCommandString] turns on, and the
// `( … )` row is what says so: a failed **expansion** inside a subshell
// leaves that column's ordinary 1 under `-c`, and this refusal inside the
// same subshell is still 127.

// substParseRouteStatus runs src on one route and answers the status.
func substParseRouteStatus(t *testing.T, src string, route Route, fromArgument int) int {
	t.Helper()
	_, st := runGrammar(t, src, nil, func(r *Runner) {
		r.Route = route
		s := *r.Semantics
		// The reading under which such a body ends the script at all, which
		// is what there is a number to ask about.
		s.SubstitutionParseErrorIsFatal = Yes
		s.SubstitutionParseErrorEscapesASubshell = Yes
		s.SubstitutionParseFailureCarriesTheFatalStatus = No
		s.FatalErrorStatusIsOne = Yes
		r.Semantics = &s
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.SyntaxErrorStatus = 2
		d.SubstitutionParseFailureStatusFromCommandString = fromArgument
		r.Diagnostics = &d
	})
	return st
}

func TestARefusedBodyTakesTheCommandStringsOwnStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"a word", "echo $(echo hi; for)\n"},
		{"an assignment", "v=$(echo hi; for)\n"},
		{"after another command", "echo one; echo $(echo hi; for)\n"},
		{"inside a subshell", "( echo $(echo hi; for) )\n"},
		{"on a pipeline element", "echo $(echo hi; for) | cat\n"},
		{"inside a function", "f() { echo $(echo hi; for); }\nf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := substParseRouteStatus(t, tc.src, RouteCommandString, 127); got != 127 {
				t.Errorf("from a command string: status %d, want the route's 127", got)
			}
			// The same text on every other route keeps the refusal's own
			// number, which is what says the 127 belongs to the route.
			for _, route := range []Route{RouteScriptFile, RouteStandardInput} {
				if got := substParseRouteStatus(t, tc.src, route, 127); got != 2 {
					t.Errorf("route %v: status %d, want the refusal's 2", route, got)
				}
			}
		})
	}
}

// Unset, the command string answers what every route answers: a preset that
// never measured this does not move.
func TestARefusedBodyWithNoRouteNumberKeepsTheRefusalsOwn(t *testing.T) {
	t.Parallel()
	if got := substParseRouteStatus(t, "echo $(echo hi; for)\n", RouteCommandString, 0); got != 2 {
		t.Errorf("status %d, want the refusal's 2", got)
	}
}

// And the number is the one the failure **ends the script with**, not one it
// leaves behind wherever such a body is written: a body in a here-document
// that the column carries the line on from is settled by its own axis and
// keeps the fatal number on the same route.
func TestAHeredocBodyCarriedOnFromKeepsItsOwnNumber(t *testing.T) {
	t.Parallel()
	out, st := runGrammar(t, "cat <<END\n$(echo hi; for)\nEND\necho AFTER\n", nil, func(r *Runner) {
		r.Route = RouteCommandString
		s := *r.Semantics
		s.SubstitutionParseErrorIsFatal = Yes
		s.SubstitutionParseFailureInAHeredocBodyEndsTheShell = No
		s.SubstitutionParseFailureCarriesTheFatalStatus = Yes
		s.FatalErrorStatusIsOne = Yes
		r.Semantics = &s
		d := CoreDiagnostics()
		if r.Diagnostics != nil {
			d = *r.Diagnostics
		}
		d.SyntaxErrorStatus = 2
		d.SubstitutionParseFailureStatusFromCommandString = 127
		r.Diagnostics = &d
	})
	if !strings.Contains(out, "AFTER") {
		t.Fatalf("out = %q, want the script carried on — the row is about a shell that did not stop", out)
	}
	if st != 0 {
		t.Errorf("status %d, want the last command's 0", st)
	}
}
