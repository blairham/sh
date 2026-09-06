// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset -f` is the same question preludelisting_test.go asks, reached through
// removal rather than through a listing (#1082): a function the prelude
// defined is the *shell's*, so a script has no such function to remove.
//
// Measured, and the panel is unanimous about the part that matters. `dirs`,
// `popd` and `pushd` are builtins in both shells that have them, so
// `unset -f pushd; pushd /tmp` pushes in both — and in a shell where the
// dialect is written as shell, deleting the declaration took the directory
// stack away for the rest of the session, silently, at status 0.
//
// The two disagree only about whether the unset says anything, and they were
// already asked: bash is silent at 0, and zsh writes the same `no such hash
// table element` it writes for any name it does not hold, at 1. That is
// Semantics.UnsetFunctionReportsMissing and not a new field — a name this
// shell provides is exactly a name the script never defined. Nothing here
// sets a second mark for prelude-ness; every case is a consequence of the
// declaration comparison that was already there.

// reportsMissing is the zsh answer for the one axis these cases turn on.
func reportsMissing(s *Semantics) { s.UnsetFunctionReportsMissing = Yes }

// TestUnsettingAPreludeFunctionLeavesTheShellItsName: the loss the bug was,
// on both dialect answers. The name still runs afterwards, which is the half
// the panel agrees on.
func TestUnsettingAPreludeFunctionLeavesTheShellItsName(t *testing.T) {
	for _, c := range []struct {
		name   string
		set    func(*Semantics)
		errs   string
		status int
	}{
		{"silent", nil, "", 0},
		{"reporting the table", reportsMissing, "testsh: unset: p: not found\n", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Two statements, so the status graded is the `unset`'s own.
			out, errs, st := preludeListingRun(t, listingPrelude, "unset -f p\n", c.set)
			if out != "" {
				t.Errorf("stdout = %q, want nothing written", out)
			}
			if errs != c.errs {
				t.Errorf("stderr = %q, want exactly %q", errs, c.errs)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}

			// And the name is still the shell's afterwards: `p` calls the
			// prelude's `__helper`, so `helper` on stdout is the whole
			// function still standing rather than only the name resolving.
			out, _, st = preludeListingRun(t, listingPrelude, "unset -f p\np\n", c.set)
			if out != "helper\n" {
				t.Errorf("stdout = %q, want %q — the shell keeps its own function", out, "helper\n")
			}
			if st != 0 {
				t.Errorf("status = %d, want the call to succeed", st)
			}
		})
	}
}

// TestUnsettingAPreludeHelperLeavesItToo: the name nobody would type is the
// one a capture leaked, and it is the shell's by the same rule. `p` calling
// it still works.
func TestUnsettingAPreludeHelperLeavesItToo(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "unset -f __helper\np\n", nil)
	if out != "helper\n" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want %q silently at 0", out, errs, st, "helper\n")
	}
}

// TestUnsettingARedefinitionGivesTheNameBackToTheShell: the other half, and
// the reason this is not a refusal. A script that wrote its own `p` owns the
// name — by declaration, which is #603's rule — so `unset -f p` removes
// *that*, and what is left is the shell's own function again. Measured:
// `pushd() { echo mine; }; unset -f pushd; pushd /tmp` pushes in both shells
// with the builtin, because removing the function uncovers it.
func TestUnsettingARedefinitionGivesTheNameBackToTheShell(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude,
		"p() { echo mine; }\nunset -f p\np\n", reportsMissing)
	if out != "helper\n" {
		t.Errorf("stdout = %q, want %q — the prelude's `p` stands again", out, "helper\n")
	}
	// Silent even under the answer that reports its table, because the name
	// *was* a function: the script's, and it went.
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}

	// Which the second `unset -f` proves from the other side: the name is
	// the shell's again, so it is reported exactly as an undefined one is.
	out, errs, st = preludeListingRun(t, listingPrelude,
		"p() { echo mine; }\nunset -f p\nunset -f p\n", reportsMissing)
	if out != "" {
		t.Errorf("stdout = %q, want nothing written", out)
	}
	if want := "testsh: unset: p: not found\n"; errs != want {
		t.Errorf("stderr = %q, want exactly %q", errs, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestUnsettingTheScriptsOwnFunctionStillRemovesIt: the ordinary case, which
// none of this may weaken. A name the prelude never declared goes for good,
// and the second `unset -f` says so.
func TestUnsettingTheScriptsOwnFunctionStillRemovesIt(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "f() { echo f; }\nunset -f f\nunset -f f\n", reportsMissing)
	if out != "" {
		t.Errorf("stdout = %q, want nothing written", out)
	}
	if want := "testsh: unset: f: not found\n"; errs != want {
		t.Errorf("stderr = %q, want exactly %q — the first removal is final", errs, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestUnsettingAPreludeFunctionChangesNoListing: the listing surface is where
// this rule came from, and removal must not move it either way. `p` was never
// the script's, so it is absent before and absent after, and `f` is there
// both times.
func TestUnsettingAPreludeFunctionChangesNoListing(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "f() { echo f; }\nunset -f p\ntypeset -F\n", nil)
	if want := "declare -f f\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
}

// TestAPreludeFunctionStillAnswersByNameAfterUnset: the decision #1035 took
// and this must not contradict. A named operand is answered, so `typeset -f p`
// says the prelude's function back after `unset -f p` exactly as it does
// before — one answer to "is this a function", not two.
func TestAPreludeFunctionStillAnswersByNameAfterUnset(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "unset -f p\ntypeset -f p\n", nil)
	if want := "p () \n{ \n  __helper\n}\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
}
