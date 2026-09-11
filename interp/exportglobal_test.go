// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The export letter deciding where a declaration lands — see
// Semantics.ExportLetterDeclaresAGlobal and #1698.
//
// Named for the axis and never for a shell.

func exportGlobalRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.DeclaredNameWithoutValueIsEmpty = Yes
		// Which functions have a scope at all is a question of its own, and
		// not one these tests ask: answered so that the No side reaches the
		// local it is about rather than a second refusal.
		s.TypesetLocalNeedsKeywordFunction = No
		s.ExportLetterDeclaresAGlobal = a
	}, Diagnostics{})
}

// The headline, both ways: under Yes the name outlives the call, under No it
// is an ordinary local and the caller sees nothing.
func TestTheExportLetterDecidingTheScopeIsAnAxis(t *testing.T) {
	const src = `f(){ typeset -x lxx=1; }; f; echo "[${lxx-UNSET}]"`
	for _, w := range []struct {
		answer Answer
		want   string
	}{{Yes, "[1]\n"}, {No, "[UNSET]\n"}} {
		out, errs, st := exportGlobalRun(t, src, w.answer)
		if out != w.want || st != 0 || errs != "" {
			t.Errorf("%v: got %q/%d stderr %q, want %q", w.answer, out, st, errs, w.want)
		}
	}
}

// `local` is exempt under either answer, which is what makes this a question
// about the letter under the other words rather than about `-x`.
func TestTheExportLetterUnderLocalIsAlwaysLocal(t *testing.T) {
	const src = `f(){ local -x le=1; }; f; echo "[${le-UNSET}]"`
	for _, a := range []Answer{Yes, No} {
		out, errs, st := exportGlobalRun(t, src, a)
		if out != "[UNSET]\n" || st != 0 || errs != "" {
			t.Errorf("%v: got %q/%d stderr %q, want the local gone", a, out, st, errs)
		}
	}
}

// A name this scope has already made local stays local, so the letter says
// where a declaration *lands* and not what it does to a name already here.
// The exemption is the half a rule written as "the letter means -g" gets
// wrong, and it is silent: the caller's value would be overwritten.
func TestTheExportLetterLeavesANameAlreadyLocalAlone(t *testing.T) {
	const src = `m=outer; f(){ local m=1; typeset -x m; echo "in=[$m]"; }; f; echo "out=[$m]"`
	out, errs, st := exportGlobalRun(t, src, Yes)
	if want := "in=[1]\nout=[outer]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// Outside a function there is no scope to reach past, so nothing is asked and
// an unanswered axis is not a refusal.
func TestTheExportLetterAsksNothingAtTheTopLevel(t *testing.T) {
	out, errs, st := declRun(t, `typeset -x v=1; echo "[$v]"`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.DeclaredNameWithoutValueIsEmpty = Yes
		s.TypesetLocalNeedsKeywordFunction = No
		// Unanswered on purpose: the point is that nothing asks.
		s.ExportLetterDeclaresAGlobal = Unspecified
	}, Diagnostics{})
	if out != "[1]\n" || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want [1] with no refusal", out, st, errs)
	}
	// And inside one it is, which is what says the silence above is the
	// guard rather than a question nothing reaches.
	out, errs, _ = declRun(t, `f(){ typeset -x v=1; }; f`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.DeclaredNameWithoutValueIsEmpty = Yes
		s.TypesetLocalNeedsKeywordFunction = No
		s.ExportLetterDeclaresAGlobal = Unspecified
	}, Diagnostics{})
	if !strings.Contains(errs, "the export letter on a declaration reaching past the function") {
		t.Errorf("unanswered: got %q stderr %q, want a refusal naming the axis", out, errs)
	}
}

// The letter without a value reaches the same decision, which is the spelling
// a fix routed through the assignment would miss.
func TestTheExportLetterWithNoValueDeclaresTheSameScope(t *testing.T) {
	const src = `f(){ typeset -x u; }; f; echo "[${u-UNSET}]"`
	out, errs, st := exportGlobalRun(t, src, Yes)
	if out != "[]\n" || st != 0 || errs != "" {
		t.Errorf("yes: got %q/%d stderr %q, want the global brought into being", out, st, errs)
	}
	out, errs, st = exportGlobalRun(t, src, No)
	if out != "[UNSET]\n" || st != 0 || errs != "" {
		t.Errorf("no: got %q/%d stderr %q, want the local gone", out, st, errs)
	}
}
