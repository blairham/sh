// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a declaration that *assigns* does to the export attribute of the name
// it assigned to. One dialect resets it and two keep it, so it is an axis; the
// name keeps its value either way, which is why every probe here reads through
// a real child rather than through a listing.

// exportResetRun answers the neighbors this axis stands next to, so each case reaches
// the question rather than a refusal.
func exportResetRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = No
		s.TypesetLocalNeedsKeywordFunction = No
		set(s)
	})
}

func TestADeclarationThatAssignsClearingTheExportAttributeIsAnAxis(t *testing.T) {
	const probe = `export FOO=bar; typeset FOO=baz; ` +
		`/usr/bin/env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"`
	out, st := exportResetRun(t, probe, func(s *Semantics) {
		s.DeclarationAssignmentClearsTheExportAttribute = Yes
	})
	if st != 0 || out != "(none)\nread=[baz]\n" {
		t.Errorf("got %q status %d, want the attribute off and the value kept", out, st)
	}
	out, st = exportResetRun(t, probe, func(s *Semantics) {
		s.DeclarationAssignmentClearsTheExportAttribute = No
	})
	if st != 0 || out != "FOO=baz\nread=[baz]\n" {
		t.Errorf("got %q status %d, want the child told the new value", out, st)
	}
}

func TestNamingTheAttributeOnTheDeclarationAnswersItOutright(t *testing.T) {
	// `-x` on the line settles it, so the axis is not asked and the answer
	// is the same on both sides of it.
	for _, answer := range []Answer{Yes, No} {
		out, st := exportResetRun(t,
			`export FOO=bar; typeset -x FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
			func(s *Semantics) {
				s.DeclarationAssignmentClearsTheExportAttribute = answer
				s.DeclareOptions = "x"
			})
		if st != 0 || out != "FOO=baz\n" {
			t.Errorf("answer %v: got %q status %d, want the letter to say so outright", answer, out, st)
		}
	}
}

func TestAValuelessDeclarationDoesNotAskAboutTheExportAttribute(t *testing.T) {
	// The value on the line is what asks it. With none, the attribute is
	// left alone whichever way the axis is answered.
	for _, answer := range []Answer{Yes, No} {
		out, st := exportResetRun(t,
			`export FOO=bar; typeset FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
			func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = answer })
		if st != 0 || out != "FOO=bar\n" {
			t.Errorf("answer %v: got %q status %d, want the attribute left alone", answer, out, st)
		}
	}
}

func TestAPlainAssignmentDoesNotAskAboutTheExportAttribute(t *testing.T) {
	// Not a declaration utility's doing at all, and no shell measured takes
	// the attribute off for one.
	out, st := exportResetRun(t,
		`export FOO=bar; FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = Yes })
	if st != 0 || out != "FOO=baz\n" {
		t.Errorf("got %q status %d, want a plain assignment to ask nothing", out, st)
	}
}

func TestClearingTheExportAttributeIsAResetAndNotARefusal(t *testing.T) {
	out, st := exportResetRun(t,
		`export FOO=bar; typeset FOO=baz; export FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = Yes })
	if st != 0 || out != "FOO=baz\n" {
		t.Errorf("got %q status %d, want naming the attribute again to put it back", out, st)
	}
}

func TestReadonlyAsksTheSameQuestionAsTypeset(t *testing.T) {
	// One shell's `readonly` is its `typeset -r`, so an assignment through
	// it resets the attribute where `typeset`'s does.
	out, st := exportResetRun(t,
		`export FOO=bar; readonly FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"`,
		func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = Yes })
	if st != 0 || out != "(none)\nread=[baz]\n" {
		t.Errorf("got %q status %d, want the attribute off through readonly too", out, st)
	}
	out, st = exportResetRun(t,
		`export FOO=bar; readonly FOO; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = Yes })
	if st != 0 || out != "FOO=bar\n" {
		t.Errorf("got %q status %d, want a valueless readonly to ask nothing", out, st)
	}
}

func TestExportItselfNeverAsksTheQuestion(t *testing.T) {
	// `export NAME=value` names the attribute in the word that runs it, so
	// it is the same case as `typeset -x` by another spelling.
	out, st := exportResetRun(t,
		`export FOO=bar; export FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) { s.DeclarationAssignmentClearsTheExportAttribute = Yes })
	if st != 0 || out != "FOO=baz\n" {
		t.Errorf("got %q status %d, want export to keep what it names", out, st)
	}
}

func TestAnImportedNameIsResetTheSameWay(t *testing.T) {
	// A name is exported by having arrived, so the question is the same by
	// that route — and the runner's own record has to say so, because there
	// is nothing in its table for the environment to be superseded by.
	out, st := run(t, "typeset IMPORTED=changed\n/usr/bin/env | grep '^IMPORTED=' || echo \"(none)\"",
		func(r *Runner) {
			sem := CoreSemantics()
			sem.DeclaredNameWithoutValueIsEmpty = No
			sem.TypesetLocalNeedsKeywordFunction = No
			sem.DeclarationAssignmentClearsTheExportAttribute = Yes
			r.Semantics = &sem
			r.Env = append(r.Env, "IMPORTED=arrived")
		})
	if st != 0 || out != "(none)\n" {
		t.Errorf("got %q status %d, want the inherited name no longer told to a child", out, st)
	}
}

func TestAScopedDeclarationAsksTheOtherAxisInstead(t *testing.T) {
	// Where the declaration took a scope, the export question is
	// LocalInheritsTheExportAttribute and this axis must not fire as well:
	// the attribute comes back when the function returns, and taking it off
	// here would take it off for good.
	out, st := exportResetRun(t,
		`export FOO=bar; f() { typeset FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; `+
			`/usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) {
			s.DeclarationAssignmentClearsTheExportAttribute = Yes
			s.LocalInheritsTheExportAttribute = No
		})
	if st != 0 || out != "(none)\nFOO=bar\n" {
		t.Errorf("got %q status %d, want the attribute back on return", out, st)
	}
	// And with the scoped axis answered the other way, the guard is the only
	// thing standing between the local's own attribute and a name left
	// unexported for the rest of the script: nothing puts the attribute back
	// where the local inherited it rather than taking it off.
	out, st = exportResetRun(t,
		`export FOO=bar; f() { typeset FOO=baz; /usr/bin/env | grep '^FOO=' || echo "(none)"; }; f; `+
			`/usr/bin/env | grep '^FOO=' || echo "(none)"`,
		func(s *Semantics) {
			s.DeclarationAssignmentClearsTheExportAttribute = Yes
			s.LocalInheritsTheExportAttribute = Yes
		})
	if st != 0 || out != "FOO=baz\nFOO=bar\n" {
		t.Errorf("got %q status %d, want the local's value inside and the outer name still exported after", out, st)
	}
	// And where the dialect gives that declaration no scope, the assignment
	// reaches the caller and this axis is the one that answers — for good.
	out, st = axisRun(t,
		`export FOO=bar; f() { typeset FOO=baz; }; f; `+
			`/usr/bin/env | grep '^FOO=' || echo "(none)"; echo "read=[$FOO]"`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.TypesetLocalNeedsKeywordFunction = Yes
			s.DeclarationAssignmentClearsTheExportAttribute = Yes
			s.LocalInheritsTheExportAttribute = No
		})
	if st != 0 || out != "(none)\nread=[baz]\n" {
		t.Errorf("got %q status %d, want the unscoped declaration to reach the caller", out, st)
	}
}

func TestAnUnansweredExportResetIsRefused(t *testing.T) {
	// A runner with no dialect refuses the declaration rather than making it
	// one way and saying so.
	_, st := axisRun(t, `export FOO=bar; typeset FOO=baz`, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = No
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclarationAssignmentClearsTheExportAttribute = Unspecified
	})
	if st != 2 {
		t.Errorf("status %d, want the unanswered axis refused", st)
	}
}
