// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// answered fills in the axes these cases pass through, so that what they
// report is the visibility rule and not an unanswered question: whether
// `type` writes a body after its sentence, and what status `command -v`
// gives a name that is nothing.
func answered(s *Semantics) {
	s.TypePrintsFunctionBody = No
	s.CommandNotFoundStatusIsNotFound = No
}

// A prelude is sourced, so everything a dialect writes as shell is a function
// — the helpers included. The names it *presents* are the shell's answer to
// "what does this shell have"; the helpers are how those are built, and a
// script asking about one is asking about a name no shell has (#2464).
//
// The rule is the name's own: a prelude declaration whose name starts with
// `__` is machinery. Nothing here sets a second mark, for the reason #1035
// gives — a parallel record has to be cleared on every route a redefinition
// can arrive by, and this package does not see those from one place.
//
// These name a *rule* and no shell, which is the substrate's side of it. What
// the dialects' own helpers are called is asserted under dialect/bash and
// dialect/zsh.

// TestAPrivatePreludeFunctionIsNotAName: the four questions a script has about
// whether a name exists, all answering the same way.
func TestAPrivatePreludeFunctionIsNotAName(t *testing.T) {
	for _, tc := range []struct {
		src, out, errs string
		status         int
	}{
		{src: "type __helper", errs: "testsh: type: __helper: not found\n", status: 1},
		{src: "command -v __helper", status: 1},
		{src: "typeset -f __helper", status: 1},
		{src: "typeset -F __helper", status: 1},
		// The presented ones are unchanged: `type` has called a prelude
		// function a function since #603 and still does.
		{src: "type p", out: "p is a function\n"},
		{src: "command -v q", out: "q\n"},
	} {
		out, errs, st := preludeListingRun(t, listingPrelude, tc.src, answered)
		if out != tc.out || errs != tc.errs || st != tc.status {
			t.Errorf("%s = %q / %q at %d, want %q / %q at %d",
				tc.src, out, errs, st, tc.out, tc.errs, tc.status)
		}
	}
}

// TestAPrivatePreludeFunctionStillRuns: hidden from a report is not taken
// away. The presented functions are built out of the helpers, so a rule that
// made the name unreachable would break the thing it is protecting.
func TestAPrivatePreludeFunctionStillRuns(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "p\n__helper\n", answered)
	if want := "helper\nhelper\n"; out != want || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want %q and a silent 0", out, errs, st, want)
	}
}

// TestAScriptsOwnPrivateNameIsItsOwn: the mark reaches the prelude's
// declarations and no others. A script that writes `__helper` has written a
// function, and the shell answers for it — by the same comparison of
// declarations that moves a diagnostic's voice back to a script that
// redefines a prelude function.
func TestAScriptsOwnPrivateNameIsItsOwn(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "__helper() { echo mine; }\ntype __helper\ntypeset -F\n", answered)
	if want := "__helper is a function\ndeclare -f __helper\n"; out != want || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want %q and a silent 0", out, errs, st, want)
	}
}
