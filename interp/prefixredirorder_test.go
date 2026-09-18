// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// When a command's assignment prefix is worked through against when its
// redirections are opened — Semantics.PrefixExpandedBeforeTheRedirections.
// The side effect of a substitution in a value is the instrument: where the
// redirection fails, it either ran or it did not. Tests name the axis and
// never a shell.

func redirOrderRun(t *testing.T, src string, order PrefixRedirectionOrder) (string, string) {
	t.Helper()
	sem := testSemantics()
	sem.PrefixExpandedBeforeTheRedirections = order
	return builtinPrefixRun(t, src, sem)
}

const redirOrderProbe = "f() { :; }\nw=$(echo S >&2) f > /nope/x\n"

func TestAPrefixCanBeExpandedBeforeTheRedirectionsOpen(t *testing.T) {
	t.Parallel()
	_, errs := redirOrderRun(t, redirOrderProbe, PrefixExpandedBeforeRedirectionsAlways)
	if !strings.HasPrefix(errs, "S\n") {
		t.Errorf("stderr = %q, want the value's side effect first", errs)
	}
}

func TestAPrefixCanBeExpandedAfterTheRedirectionsOpen(t *testing.T) {
	t.Parallel()
	_, errs := redirOrderRun(t, redirOrderProbe, PrefixExpandedBeforeRedirectionsNever)
	if strings.Contains(errs, "S\n") {
		t.Errorf("stderr = %q, want the value never expanded", errs)
	}
	if errs == "" {
		t.Errorf("stderr is empty; want the redirection's own complaint")
	}
}

// The third answer, which is the one that needs a named type: the prefix is
// worked through first only where it is a real store — a function or a
// special builtin — and the redirections open first for a regular builtin and
// an external.
func TestAPrefixCanBeExpandedFirstOnlyWhereItPersists(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		src   string
		first bool
	}{
		{"f() { :; }\nw=$(echo S >&2) f > /nope/x\n", true},
		{"w=$(echo S >&2) eval : > /nope/x\n", true},
		{"w=$(echo S >&2) true > /nope/x\n", false},
	} {
		_, errs := redirOrderRun(t, row.src, PrefixExpandedBeforeRedirectionsWhereItPersists)
		if got := strings.HasPrefix(errs, "S\n"); got != row.first {
			t.Errorf("%q: expanded first = %v, want %v (stderr %q)", row.src, got, row.first, errs)
		}
	}
	// And the control that says the same vector answers the other way for
	// the same commands: under the always reading every one of them expands
	// first.
	_, errs := redirOrderRun(t, "w=$(echo S >&2) true > /nope/x\n",
		PrefixExpandedBeforeRedirectionsAlways)
	if !strings.HasPrefix(errs, "S\n") {
		t.Errorf("stderr = %q, want the value's side effect first", errs)
	}
}

// And the walk that comes with it is in **written order**: a value earlier in
// the prefix is expanded before a later word is refused, where the refusals
// used to be a pass of their own ahead of every value.
func TestARefusedPrefixWordIsRefusedWhereItStands(t *testing.T) {
	t.Parallel()
	sem := testSemantics()
	sem.PrefixExpandedBeforeTheRedirections = PrefixExpandedBeforeRedirectionsAlways
	sem.SubscriptedAssignmentPrefix = SubscriptedPrefixIsRefused
	_, errs := builtinPrefixRun(t, "f() { :; }\nw=$(echo S1 >&2) a[1]=v f\n", sem)
	side, refusal := strings.Index(errs, "S1"), strings.Index(errs, "a[1]")
	if side < 0 || refusal < 0 || side > refusal {
		t.Errorf("stderr = %q, want the earlier word's value before the later word's refusal", errs)
	}
	// The refused word's **own** value is never expanded, which is what keeps
	// the walk from being "expand everything, then refuse".
	_, errs = builtinPrefixRun(t, "f() { :; }\na[1]=$(echo SIDE >&2) f\n", sem)
	if strings.Contains(errs, "SIDE") {
		t.Errorf("stderr = %q, want the refused word's own value left unexpanded", errs)
	}
}

// And a dialect that has not chosen refuses by name.
func TestThePrefixRedirectionOrderIsRefusedWhereTheDialectHasNotChosen(t *testing.T) {
	t.Parallel()
	_, errs := redirOrderRun(t, redirOrderProbe, PrefixExpandedBeforeRedirectionsUnspecified)
	if !strings.Contains(errs, "assignment prefix against its redirections") {
		t.Errorf("stderr = %q, want the axis named", errs)
	}
}
