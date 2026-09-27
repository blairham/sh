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

// A command with **no command word** asks the same axis, and the middle answer
// collapses into the first: an assignment with nothing in front of it is
// always a real store, so the column that works through a prefix first only
// where it persists does so here too. See
// Runner.assignmentsExpandedBeforeTheRedirections and #4613.
//
// The probe writes the two sides' **order** rather than reading it off a
// status. A status here is one number answering two questions — which
// substitution ran last, and whether the open failed — so a row that only
// checked the number could be right for the wrong reason.
func bareRedirOrderRun(t *testing.T, src string, order PrefixRedirectionOrder) (string, int) {
	t.Helper()
	sem := testSemantics()
	sem.PrefixExpandedBeforeTheRedirections = order
	return sourceRun(t, t.TempDir(), src, sem, Diagnostics{})
}

const bareRedirOrderProbe = "false\n" +
	"X=$(echo A >&2; exit 2) > $(echo B >&2; echo /dev/null; exit 3)\n" +
	"echo \"st=$?\"\n"

func TestBareAssignmentsCanBeExpandedBeforeTheRedirectionsOpen(t *testing.T) {
	t.Parallel()
	for _, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsAlways,
		PrefixExpandedBeforeRedirectionsWhereItPersists,
	} {
		got, _ := bareRedirOrderRun(t, bareRedirOrderProbe, order)
		if got != "A\nB\nst=3\n" {
			t.Errorf("%v: got %q, want the assignment's side effect first and the redirection's status", order, got)
		}
	}
}

func TestBareAssignmentsCanBeExpandedAfterTheRedirectionsOpen(t *testing.T) {
	t.Parallel()
	got, _ := bareRedirOrderRun(t, bareRedirOrderProbe, PrefixExpandedBeforeRedirectionsNever)
	if got != "B\nA\nst=2\n" {
		t.Errorf("got %q, want the redirection's word first and the assignment's status", got)
	}
}

// And the open failing is where the order is worth something rather than
// merely visible: behind the redirections, a value whose file would not open
// is never expanded at all and the name keeps what it had.
func TestABareAssignmentBehindAFailedOpenIsNeverExpanded(t *testing.T) {
	t.Parallel()
	const src = "X=$(echo A >&2; echo v) > /nope/dir/x\necho \"X=[${X-unset}]\"\n"
	first, _ := bareRedirOrderRun(t, src, PrefixExpandedBeforeRedirectionsAlways)
	if !strings.Contains(first, "A\n") || !strings.Contains(first, "X=[v]") {
		t.Errorf("expanded first: got %q, want the side effect and the stored value", first)
	}
	behind, _ := bareRedirOrderRun(t, src, PrefixExpandedBeforeRedirectionsNever)
	if strings.Contains(behind, "A\n") || !strings.Contains(behind, "X=[unset]") {
		t.Errorf("opened first: got %q, want no side effect and no store", behind)
	}
}

// The axis is asked **only where both are written**, which is what keeps a
// dialect that has no answer from refusing an ordinary `>f`. With no answer
// at all the pair refuses and each half alone still runs.
func TestABareRedirectionAloneAsksNothingOfTheOrder(t *testing.T) {
	t.Parallel()
	run := func(src string) (string, int) {
		return bareRedirOrderRun(t, src, PrefixExpandedBeforeRedirectionsUnspecified)
	}
	if out, st := run("> alone\necho ok\n"); out != "ok\n" || st != 0 {
		t.Errorf("a redirection alone: got %q status %d, want it to run", out, st)
	}
	if out, st := run("x=1\necho \"[$x]\"\n"); out != "[1]\n" || st != 0 {
		t.Errorf("an assignment alone: got %q status %d, want it to run", out, st)
	}
	out, _ := run("x=1 > both\necho \"st=$?\"\n")
	if !strings.Contains(out, "no dialect was chosen") || !strings.Contains(out, "st=2\n") {
		t.Errorf("both written: got %q, want the refusal and its status", out)
	}
}
