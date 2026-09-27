// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A name the shell holds a binding for, which the second declaration word may
// not move into a scope — see interp/parameterscopefixed.go.
//
// The seam and not the set: which names a shell marks is that shell's and
// lives in its own package, so every row here marks a name of its own making
// and nothing below names a real parameter or a real shell.
//
// The control is the whole of what makes these rows evidence. A word that
// refused every declaration would pass the refusal rows and fail the taken
// ones, and a mark that was never read would do the reverse.

// fixedWord is a dialect with the word and one name it says is its own.
func fixedWord(r *Runner) {
	r.Register("private", PrivateBuiltin())
	r.MarkParameterScopeFixed("held")
}

// TestAScopeFixedNameIsRefusedByTheSecondDeclarationWord is the refusal, the
// status it carries, and the script running on past it.
func TestAScopeFixedNameIsRefusedByTheSecondDeclarationWord(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `held=outer
f() { private held; echo "st=$?"; echo "held=[$held]"; }
f
echo "after=[$held]"`, withPrivate, Diagnostics{}, nil, fixedWord)
	if !strings.Contains(errs, "can't change scope of existing param: held") {
		t.Errorf("stderr = %q, want the refusal in it", errs)
	}
	want := "st=1\nheld=[outer]\nafter=[outer]\n"
	if out != want || st != 0 {
		t.Errorf("the refusal = %q (status %d), want %q", out, st, want)
	}
	// The control: an unmarked name is declared, and the callee's own scope
	// is what it gets. Without this row a builtin that refused everything
	// would pass the check above.
	out, errs, st = declRunWith(t, `free=outer
f() { private free=inner; echo "st=$? free=[$free]"; }
f
echo "after=[$free]"`, withPrivate, Diagnostics{}, nil, fixedWord)
	want = "st=0 free=[inner]\nafter=[outer]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("an unmarked name = %q/%q (status %d), want %q", out, errs, st, want)
	}
	// And the other control: the *first* declaration word is untouched by the
	// mark. `local` over the same name declares it, which is what says this
	// belongs to the word rather than to the name.
	out, errs, st = declRunWith(t, `held=outer
f() { local held=inner; echo "st=$? held=[$held]"; }
f
echo "after=[$held]"`, withPrivate, Diagnostics{}, nil, fixedWord)
	want = "st=0 held=[inner]\nafter=[outer]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("local over a marked name = %q/%q (status %d), want %q", out, errs, st, want)
	}
}

// The mark is about the **name** and not about a value standing under it: a
// name the script has removed is still the shell's, where the mark a dialect
// puts on a name for a *listing* is not — see Runner.shellOwnParameter, which
// is deliberately a different table.
func TestAScopeFixedNameIsStillRefusedWithNothingUnderIt(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `unset held
f() { private held; echo "st=$?"; }
f`, withPrivate, Diagnostics{}, nil, fixedWord)
	if !strings.Contains(errs, "can't change scope of existing param: held") {
		t.Errorf("stderr = %q, want the refusal in it", errs)
	}
	if out != "st=1\n" || st != 0 {
		t.Errorf("the refusal over nothing = %q (status %d), want st=1", out, st)
	}
}

// The top level takes it, because a declaration that takes no scope is not
// moving a binding anywhere. The same line in Runner.shadow decides both.
func TestAScopeFixedNameIsTakenWhereThereIsNoScope(t *testing.T) {
	t.Parallel()
	atTopLevel := func(s *Semantics) {
		withPrivate(s)
		s.LocalOutsideAFunctionIsAnError = No
	}
	out, errs, st := declRunWith(t, `held=outer
private held=inner
echo "st=$? held=[$held]"`, atTopLevel, Diagnostics{}, nil, fixedWord)
	want := "st=0 held=[inner]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("at the top level = %q/%q (status %d), want %q", out, errs, st, want)
	}
}

// The hide-in-scope letter asks for an ordinary local over the shell's name
// instead of a binding of its own, and is the one way past this refusal. It
// is not a way past a **redeclaration**, which is the asymmetry the two
// reasons have and the reason they are one gate with two questions.
func TestTheHideLetterIsTheWayPastAScopeFixedName(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `held=outer
f() { private -h held=inner; echo "st=$? held=[$held]"; }
f
echo "after=[$held]"`, hideLetterToo, Diagnostics{}, nil, fixedWord)
	want := "st=0 held=[inner]\nafter=[outer]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("the hide letter = %q/%q (status %d), want %q", out, errs, st, want)
	}
	out, _, _ = declRunWith(t, `f() { private v=1; private -h v=2; echo "st=$? v=[$v]"; }
f`, hideLetterToo, Diagnostics{}, nil, fixedWord)
	if out != "st=1 v=[1]\n" {
		t.Errorf("the hide letter over a redeclaration = %q, want the refusal", out)
	}
}

// hideLetterToo is withPrivate with the hide-in-scope letter on both words.
func hideLetterToo(s *Semantics) {
	withPrivate(s)
	s.LocalOptions += "h"
	s.PrivateOptions += "h"
}

// bareSignToo is withPrivate for a dialect that reads a sign on its own as an
// option word. The axis is asked because the shells do not agree on it, and a
// dialect that reads `+` as a name never reaches the exemption at all.
func bareSignToo(s *Semantics) {
	hideLetterToo(s)
	s.SignAloneIsAnOptionWord = Yes
}

// A bare sign is an option word rather than a declaration, so neither reason
// reaches it — and an operand carrying a value brings both back.
func TestABareSignIsNotADeclarationForEitherReason(t *testing.T) {
	t.Parallel()
	out, errs, st := declRunWith(t, `held=outer
f() { private + held; echo "st=$?"; }
f`, bareSignToo, Diagnostics{}, nil, fixedWord)
	if out != "st=0\n" || errs != "" || st != 0 {
		t.Errorf("a bare sign = %q/%q (status %d), want st=0", out, errs, st)
	}
	out, errs, st = declRunWith(t, `held=outer
f() { private + held=inner; echo "st=$?"; }
f
echo "after=[$held]"`, bareSignToo, Diagnostics{}, nil, fixedWord)
	if !strings.Contains(errs, "can't change scope of existing param: held") {
		t.Errorf("stderr = %q, want the refusal in it", errs)
	}
	if out != "st=1\nafter=[outer]\n" || st != 0 {
		t.Errorf("a bare sign with a value = %q (status %d), want the refusal", out, st)
	}
	// And `+h`, which sets the same removal flag and is not a bare sign. The
	// pair is what says this reads the word and not `f.remove`.
	out, _, _ = declRunWith(t, `held=outer
f() { private +h held; echo "st=$?"; }
f`, bareSignToo, Diagnostics{}, nil, fixedWord)
	if out != "st=1\n" {
		t.Errorf("+h over a marked name = %q, want the refusal", out)
	}
}
