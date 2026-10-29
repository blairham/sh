// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether a prefix the shell keeps in front of a declaration that is declaring
// locals is a local of the running function —
// Semantics.PrefixToALocalDeclarationIsALocal. `local` is made special here,
// so its prefix is kept, and a function the body calls is the reader: it sees
// the body's locals, and the line after the call sees the shell's own name.
// Each row runs twice: with the prefix worked through ahead of the
// redirections where it persists, as the column that answers Yes does, and
// with the redirections opened first, where the store is the first thing to
// reach the value. Tests name the axis and never a shell.

func prefixDeclarationLocalRun(t *testing.T, src string, answer Answer) (string, string) {
	t.Helper()
	var outs, errs [2]string
	for i, order := range []PrefixRedirectionOrder{
		PrefixExpandedBeforeRedirectionsWhereItPersists,
		PrefixExpandedBeforeRedirectionsNever,
	} {
		sem := testSemantics()
		sem.SpecialBuiltinsBeyondPosix = "local"
		sem.AssignmentPrefixPersistsOnSpecialBuiltin = Yes
		sem.PrefixExpandedBeforeTheRedirections = order
		sem.PrefixToALocalDeclarationIsALocal = answer
		outs[i], errs[i] = builtinPrefixRun(t, src, sem)
	}
	if outs[0] != outs[1] {
		t.Errorf("the prefix walked ahead of the redirections wrote %q, and stored without it %q", outs[0], outs[1])
	}
	return outs[1], errs[0] + errs[1]
}

const prefixDeclarationLocalProbe = `g() { echo "g v=$v"; }; f() { v=new local y=1; echo "in v=$v"; g; }; v=old; f; echo "out v=$v"` + "\n"

func TestAKeptPrefixCanBeALocalOfTheFunction(t *testing.T) {
	t.Parallel()
	out, errs := prefixDeclarationLocalRun(t, prefixDeclarationLocalProbe, Yes)
	if want := "in v=new\ng v=new\nout v=old\n"; out != want || errs != "" {
		t.Errorf("got %q, stderr %q, want %q", out, errs, want)
	}
}

func TestAKeptPrefixCanBeAnOrdinaryAssignment(t *testing.T) {
	t.Parallel()
	out, errs := prefixDeclarationLocalRun(t, prefixDeclarationLocalProbe, No)
	if want := "in v=new\ng v=new\nout v=new\n"; out != want || errs != "" {
		t.Errorf("got %q, stderr %q, want %q", out, errs, want)
	}
}

// The local is a fresh cell, made after the value is expanded and before an
// append joins it: the value reads the name the body could see, and the append
// joins nothing.
func TestTheLocalIsMadeAfterTheValueAndBeforeTheJoin(t *testing.T) {
	t.Parallel()
	const src = `f() { v=${v}x local y=1; echo "in v=$v"; w+=x local z=1; echo "in w=$w"; }; v=old; w=old; f; echo "out v=$v w=$w"` + "\n"
	out, errs := prefixDeclarationLocalRun(t, src, Yes)
	if want := "in v=oldx\nin w=x\nout v=old w=old\n"; out != want || errs != "" {
		t.Errorf("got %q, stderr %q, want %q", out, errs, want)
	}
}

// Asked only at the disagreement: a vector that leaves the axis unanswered
// runs a kept prefix in front of anything but a declaration, and a declaration
// outside a function, without refusing either.
func TestAPrefixThatIsNotInFrontOfALocalDeclarationAsksNothing(t *testing.T) {
	t.Parallel()
	const src = `f() { v=new export y=1; v=new :; echo "in v=$v"; }; v=old; f; v=new2 local z=1; echo "out v=$v"` + "\n"
	out, errs := prefixDeclarationLocalRun(t, src, Unspecified)
	if want := "in v=new\nout v=new2\n"; out != want || strings.Contains(errs, "unanswered") {
		t.Errorf("got %q, stderr %q, want %q and no refusal", out, errs, want)
	}
	// And the question is put where it is one.
	_, errs = prefixDeclarationLocalRun(t, prefixDeclarationLocalProbe, Unspecified)
	if errs == "" {
		t.Error("an unanswered axis was not refused in front of a local declaration")
	}
}
