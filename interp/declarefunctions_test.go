// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `declare`, `typeset` and `local` read the letters the dialect gives them —
// see Semantics.DeclareOptions and LocalOptions — and the letters beyond the
// substrate's old set: functions said back with -f and -F, globals with -g,
// case folding with -l and -u. Tests name axes and wordings, never shells.

// declRun runs src with the semantics the setter leaves, a fixed function
// layout, and the given diagnostics, and reports stdout, stderr and status.
func declRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, string, int) {
	t.Helper()
	return declRunEnv(t, src, set, dg, nil)
}

// declRunEnv is declRun with an environment the shell was started with, for
// the one question that is about a name the script never assigned.
func declRunEnv(t *testing.T, src string, set func(*Semantics), dg Diagnostics, env []string) (string, string, int) {
	t.Helper()
	return declRunWith(t, src, set, dg, env, nil)
}

// declRunTied is declRun with a tie the *shell* made rather than the script,
// which is the only input the hide-in-scope letter is observable through: a
// `local` of one half of a script's tie is an ordinary local either way. See
// interp/tielocal.go.
func declRunTied(t *testing.T, src string, set func(*Semantics), dg Diagnostics, pairs ...[2]string) (string, string, int) {
	t.Helper()
	return declRunWith(t, src, set, dg, nil, func(r *Runner) {
		for _, pair := range pairs {
			r.Tie(pair[0], pair[1], ":")
		}
	})
}

// declRunWith is what the three above are: a run whose Runner a caller may
// arrange before the script sees it, the way a dialect arranges one.
func declRunWith(t *testing.T, src string, set func(*Semantics), dg Diagnostics, env []string, before func(*Runner)) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclaredNameWithoutValueIsEmpty = No
	sem.ValuelessDeclarationHidesTheOuterValue = Yes
	sem.DeclareListing = DeclareListingClustered
	sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
	// See vector_test.go: the declaration divergences answered the quiet
	// way, so a test that is not about one of them is not refused by it. The
	// suites that are about them set their own.
	sem.ExportLetterDeclaresAGlobal = No
	sem.ValuelessDeclarationOfAHeldNameListsIt = No
	sem.ScalarOverACompoundIsAnInconsistentType = No
	sem.ReadonlyRecordsTheCompoundAttribute = No
	sem.TypeLetterAndAnArrayLiteralIsAnInconsistentType = No
	sem.NumericAttributeReplacesTheCaseAttribute = No
	sem.CaseAttributeReplacesTheNumericAttribute = No
	sem.NumericAttributeReplacesTheArrayAttribute = No
	sem.ArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = No
	sem.AppendedArrayLiteralOverANameNotDeclaredAnArrayStartsItOver = No
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh", Env: env,
	})
	// One arrangement for every test that says a function back; the tests
	// about arrangements live with the dialects that own them.
	r.SetFunctionLayout(syntax.Layout{
		Indent: "  ", Nested: true, Lines: true,
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}, syntax.Layout{Indent: " ", Lines: true, BraceOpenSuffix: " "})
	if before != nil {
		before(r)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

func withDeclareLetters(letters string) func(*Semantics) {
	return func(s *Semantics) { s.DeclareOptions = letters }
}

// TestDeclareFSaysTheFunctionsBack: `-f` writes the function itself — every
// one, sorted, when nothing narrows it — and a name that is no function is a
// silent 1 that stands however many others printed.
func TestDeclareFSaysTheFunctionsBack(t *testing.T) {
	src := "zz() { echo two; }\naa() { echo one; }\ntypeset -f"
	out, errs, st := declRun(t, src, withDeclareLetters("aAfFgiprx"), Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want a clean listing", st, errs)
	}
	if !strings.Contains(out, "echo two") || !strings.Contains(out, "echo one") {
		t.Errorf("stdout = %q, want both bodies", out)
	}
	if strings.Index(out, "aa") > strings.Index(out, "zz") {
		t.Errorf("stdout = %q, want the listing sorted", out)
	}

	out, errs, st = declRun(t, "f() { echo hi; }\ntypeset -f f nosuch\necho st=$?",
		withDeclareLetters("aAfFgiprx"), Diagnostics{})
	if !strings.Contains(out, "echo hi") {
		t.Errorf("stdout = %q, want the named function's body", out)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want the missing name passed over in silence", errs)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("stdout = %q, want status 1 for the missing name", out)
	}
	_ = st
}

// TestDeclareFHonorsTheHeaderWording: the join between the name and the body
// is the dialect's — see Diagnostics.FunctionListingHeader.
func TestDeclareFHonorsTheHeaderWording(t *testing.T) {
	out, _, _ := declRun(t, "f() { echo hi; }\ntypeset -f f",
		withDeclareLetters("f"), Diagnostics{FunctionListingHeader: "%[1]s () %[2]s"})
	if !strings.HasPrefix(out, "f () { ") {
		t.Errorf("stdout = %q, want the brace kept on the header's line", out)
	}
	out, _, _ = declRun(t, "f() { echo hi; }\ntypeset -f f",
		withDeclareLetters("f"), Diagnostics{})
	if !strings.HasPrefix(out, "f () \n{ ") {
		t.Errorf("stdout = %q, want the fallback header with the brace on its own line", out)
	}
}

// TestDeclareCapitalFNamesThem: `-F` writes one line per function — its own
// two shapes, `declare -f name` for the full listing and the bare name once
// operands narrow it — and never a body.
func TestDeclareCapitalFNamesThem(t *testing.T) {
	out, _, st := declRun(t, "f() { echo hi; }\ng() { :; }\ntypeset -F",
		withDeclareLetters("aAfFgiprx"), Diagnostics{})
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	if want := "declare -f f\ndeclare -f g\n"; out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
	out, _, st = declRun(t, "f() { echo hi; }\ntypeset -F f",
		withDeclareLetters("aAfFgiprx"), Diagnostics{})
	if out != "f\n" || st != 0 {
		t.Errorf("stdout %q status %d, want the bare name and 0", out, st)
	}
	_, _, st = declRun(t, "typeset -F nosuch", withDeclareLetters("F"), Diagnostics{})
	if st != 1 {
		t.Errorf("status = %d, want 1 for a name that is no function", st)
	}
}

// TestDeclareGlobalReachesPastALocal: `-g` lands on the global cell — and
// whether it does so *past a local of the same name* is the axis, with one
// engine writing the global and the other assigning the local it can see.
func TestDeclareGlobalReachesPastALocal(t *testing.T) {
	src := "x=out\nf() { local x=in; typeset -g x=new; echo in=$x; }\nf\necho out=$x"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.DeclareGlobalReachesPastALocal = Yes
	}, Diagnostics{})
	if errs != "" {
		t.Fatalf("stderr = %q, want none", errs)
	}
	if !strings.Contains(out, "in=in") {
		t.Errorf("stdout = %q, want the local untouched", out)
	}
	if !strings.Contains(out, "out=new") {
		t.Errorf("stdout = %q, want the global cell written", out)
	}

	out, _, _ = declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.DeclareGlobalReachesPastALocal = No
	}, Diagnostics{})
	if !strings.Contains(out, "in=new") || !strings.Contains(out, "out=out") {
		t.Errorf("stdout = %q, want the local assigned and the global left alone", out)
	}

	// With no local in the way, the letter is not a question: the global is
	// written without the axis being asked.
	out, errs, _ = declRun(t, "f() { typeset -g g1=set; }\nf\necho g=$g1",
		withDeclareLetters("aAgiprx"), Diagnostics{})
	if !strings.Contains(out, "g=set") || errs != "" {
		t.Errorf("stdout %q stderr %q, want the global declared without a question", out, errs)
	}
}

// TestCaseAttributesFoldAssignments: `-l` and `-u` are properties of the
// name, folding the declaring assignment and every later one.
func TestCaseAttributesFoldAssignments(t *testing.T) {
	src := "typeset -l v=ABC\necho $v\nv=DEF\necho $v\ntypeset -u w=abc\necho $w\ntypeset -p v"
	out, _, _ := declRun(t, src, withDeclareLetters("aAilpurx"), Diagnostics{})
	for _, want := range []string{"abc\n", "def\n", "ABC\n", "declare -l v=\"def\"\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout = %q, want %q in it", out, want)
		}
	}
}

// TestDeclareLettersAreTheDialects: a letter outside DeclareOptions is
// refused — as missing where the dialect has it, as unknown where it does
// not — and the refusal ends the script only where TypesetBadOptionFatal
// says so.
func TestDeclareLettersAreTheDialects(t *testing.T) {
	out, errs, st := declRun(t, "typeset -f x\necho after",
		func(s *Semantics) { s.DeclareOptions = "aAiprx"; s.TypesetBadOptionFatal = No },
		Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "f"}})
	if !strings.Contains(errs, "-f is not implemented yet") {
		t.Errorf("stderr = %q, want the letter named as missing", errs)
	}
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("stdout %q status %d, want the script to carry on", out, st)
	}

	_, errs, _ = declRun(t, "typeset -q x",
		func(s *Semantics) { s.DeclareOptions = "aAiprx"; s.TypesetBadOptionFatal = No },
		Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: unknown option"})
	if !strings.Contains(errs, "typeset: -q: unknown option") {
		t.Errorf("stderr = %q, want the dialect's own refusal", errs)
	}

	out, _, st = declRun(t, "typeset -q x\necho after",
		func(s *Semantics) { s.DeclareOptions = "aAiprx"; s.TypesetBadOptionFatal = Yes },
		Diagnostics{})
	if strings.Contains(out, "after") || st != 2 {
		t.Errorf("stdout %q status %d, want the refusal to end the script with 2", out, st)
	}
}

// TestADeclareLetterWithoutEffectIsTakenInSilence: a letter the dialect has
// and this engine models nothing for is accepted, decides nothing, and says
// nothing — Semantics.DeclareOptionsWithoutEffect, the quiet counterpart of
// Diagnostics.UnimplementedOptionLetters.
//
// Named for the axis rather than for a shell, as every test in this package
// is. The letter used is `F`, because that is the interesting case: the
// substrate gives it a meaning of its own — the function listing — and the
// field has to take that meaning away rather than merely add silence to it.
func TestADeclareLetterWithoutEffectIsTakenInSilence(t *testing.T) {
	silent := func(s *Semantics) {
		s.DeclareOptions = "aAfFgiprx"
		s.DeclareOptionsWithoutEffect = "F"
	}
	// The letter's substrate meaning is gone: two functions are defined and
	// the listing that would have named them writes nothing at all.
	out, errs, st := declRun(t, "f() { echo hi; }\ng() { :; }\ntypeset -F", silent, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want not one byte written and 0", out, errs, st)
	}

	// With an operand it is still silent, where the substrate's meaning
	// would have written the bare name.
	out, errs, st = declRun(t, "f() { echo hi; }\ntypeset -F f", silent, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want not one byte written and 0", out, errs, st)
	}

	// And a name that is no function is 0 here, not the 1 the listing
	// answers — the letter decides nothing, so there is nothing to fail.
	_, _, st = declRun(t, "typeset -F nosuch", silent, Diagnostics{})
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// TestADeclareLetterWithoutEffectIsStillADeclaration: the silent letter must
// not turn its command into the bare word, whose listing is a different
// command entirely — see BareDeclarationListing.
func TestADeclareLetterWithoutEffectIsStillADeclaration(t *testing.T) {
	silent := func(s *Semantics) {
		s.DeclareOptions = "aAfFgiprx"
		s.DeclareOptionsWithoutEffect = "F"
		s.BareTypesetListing = BareLocalListsEveryParameter
	}
	out, errs, st := declRun(t, "marked=here\ntypeset -F", silent, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want a silent 0 rather than a listing", out, errs, st)
	}
	// The control: the bare word does list under these semantics, so the
	// row above says something about the letter.
	out, _, st = declRun(t, "marked=here\ntypeset", silent, Diagnostics{})
	if st != 0 || !strings.Contains(out, `marked="here"`) {
		t.Errorf("bare typeset = %q status %d, want the name listed", out, st)
	}
	// And an operand beside the silent letter is still declared and
	// assigned, which is what keeps it a declaration.
	out, _, _ = declRun(t, "typeset -F v=1.5\necho \"[$v]\"", silent, Diagnostics{})
	if out != "[1.5]\n" {
		t.Errorf("stdout = %q, want the assignment carried out", out)
	}
}

// TestADeclareLetterOutsideTheSilentSetStillRefuses: the field grants silence
// to the letters it names and to no others, so a letter the dialect has and
// this engine does not is still refused in the dialect's words — the loud
// counterpart, which stays the answer wherever refusing is the smaller lie.
func TestADeclareLetterOutsideTheSilentSetStillRefuses(t *testing.T) {
	_, errs, st := declRun(t, "typeset -E v", func(s *Semantics) {
		s.DeclareOptions = "aAfFgiprx" // the dialect has `E` and this does not
		s.DeclareOptionsWithoutEffect = "F"
		s.TypesetBadOptionFatal = No
	}, Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "E"}})
	if want := "testsh: typeset: -E is not implemented yet\n"; errs != want {
		t.Errorf("stderr = %q, want exactly %q", errs, want)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// TestASilentLetterTheDialectDoesNotHaveGrantsNothing: the two fields cannot
// disagree about whether a letter exists. DeclareOptions decides that; this
// one only decides what a letter it already names means.
func TestASilentLetterTheDialectDoesNotHaveGrantsNothing(t *testing.T) {
	_, errs, st := declRun(t, "typeset -F v", func(s *Semantics) {
		s.DeclareOptions = "aAiprx" // no F
		s.DeclareOptionsWithoutEffect = "F"
		s.TypesetBadOptionFatal = No
	}, Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: unknown option"})
	if !strings.Contains(errs, "typeset: -F: unknown option") {
		t.Errorf("stderr = %q, want the letter refused as one the dialect does not have", errs)
	}
	if st == 0 {
		t.Errorf("status = %d, want a refusal", st)
	}
}
