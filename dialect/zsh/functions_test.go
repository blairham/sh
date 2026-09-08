// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `functions` and `unfunction` are this shell's names for `typeset -f` and
// `unset -f`. These are here rather than in interp because what is being
// pinned is the dialect's tables — that the words are registered at all, and
// which letters each takes — and a case built on a permissive base cannot
// see either (#1386, #1477).

// TestFunctionsSaysAFunctionBack, which is the whole of the first name.
func TestFunctionsSaysAFunctionBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "f(){ echo hi; }\nfunctions f\necho st=$?\n")
	if !strings.Contains(out, "f () {\n\techo hi\n}\n") {
		t.Errorf("output = %q, want the body written out", out)
	}
	if !strings.Contains(out, "st=0\n") || st != 0 {
		t.Errorf("output = %q status %d, want 0", out, st)
	}
	// The same listing `typeset -f` gives, because it is the same code. A
	// second implementation that drifted is what this compares against.
	byOtherName, _ := runZsh(t, t.TempDir(), "f(){ echo hi; }\ntypeset -f f\n")
	if out2, _ := runZsh(t, t.TempDir(), "f(){ echo hi; }\nfunctions f\n"); out2 != byOtherName {
		t.Errorf("functions = %q, typeset -f = %q, want one listing", out2, byOtherName)
	}
}

// TestFunctionsPrintsTheKeywordFormWithoutTheKeyword. #1406 is the same
// question asked of the printer, where the answer is the other one — a tree
// keeps FuncDecl.Keyword because ksh93's `typeset` reads it, so printing a
// program back without it changes which names are local. A *listing* is not
// that: measured 2026-09-08, zsh 5.9.2 writes `f () ` back for either
// spelling, and it may because `typeset` declares a local in both bodies
// here, so nothing about the round trip changes.
func TestFunctionsPrintsTheKeywordFormWithoutTheKeyword(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "function f { typeset x=1; }\nfunctions f\n")
	if !strings.HasPrefix(out, "f () {\n") {
		t.Errorf("output = %q, want the paren header", out)
	}
	if strings.Contains(out, "function f") {
		t.Errorf("output = %q, want no keyword in the listing", out)
	}
	// And the round trip really is a round trip: the locals stay local.
	out, st := runZsh(t, t.TempDir(),
		"x=OUT\nfunction f { typeset x=1; }\nsrc=$(functions f)\nunfunction f\neval \"$src\"\nf\necho after=$x\n")
	if !strings.Contains(out, "after=OUT\n") || st != 0 {
		t.Errorf("output = %q status %d, want the listing to re-eval with the same locals", out, st)
	}
}

// TestFunctionsReportsANameItDoesNotHold — silent at 1, and the 1 stands
// however many other names printed.
func TestFunctionsReportsANameItDoesNotHold(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "functions nosuch\necho st=$?\n")
	if out != "st=1\n" || st != 0 {
		t.Errorf("output = %q status %d, want a silent 1", out, st)
	}
	out, _ = runZsh(t, t.TempDir(), "f(){ :; }\nfunctions f nosuch\necho st=$?\n")
	if !strings.Contains(out, "f () {") || !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want the one printed and the status still 1", out)
	}
}

// TestUnfunctionRemovesAFunction, and answers about a name it does not hold
// with the same sentence `unset -f` answers with — which is what running the
// same code gets, rather than a second wording that would drift.
func TestUnfunctionRemovesAFunction(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "f(){ :; }\nunfunction f\necho st=$?\nfunctions f\necho st2=$?\n")
	if out != "st=0\nst2=1\n" || st != 0 {
		t.Errorf("output = %q status %d, want the function gone at 0", out, st)
	}
	out, _ = runZsh(t, t.TempDir(), "unfunction nosuch\necho st=$?\n")
	if !strings.Contains(out, "no such hash table element: nosuch") || !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want the table complaint at 1", out)
	}
	// The builtin names itself in the location, which follows from being
	// dispatched under its own name and not from a wording of its own.
	if !strings.Contains(out, ":unfunction:") {
		t.Errorf("output = %q, want the invoked name in the location", out)
	}
	// Several names, one of them absent: the others still go.
	out, _ = runZsh(t, t.TempDir(), "f(){ :; }\ng(){ :; }\nunfunction f nosuch g\necho st=$?\nfunctions\n")
	if !strings.Contains(out, "st=1\n") || strings.Contains(out, "f () {") || strings.Contains(out, "g () {") {
		t.Errorf("output = %q, want both removed and the status 1", out)
	}
}

// TestUnfunctionWithNoOperandComplains, where a bare `unset` in three of the
// panel is silent. Same sentence, same status, and the name is the invoked
// one.
func TestUnfunctionWithNoOperandComplains(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "unfunction\necho st=$?\n")
	if !strings.Contains(out, ":unfunction:1: not enough arguments") || !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want the complaint at 1", out)
	}
	out, _ = runZsh(t, t.TempDir(), "unset\necho st=$?\n")
	if !strings.Contains(out, ":unset:1: not enough arguments") || !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want the same complaint under the other name", out)
	}
}

// TestTheLettersAreNarrowerThanTheBuiltinRenamed. `functions` is `typeset -f`
// and yet `-f` is a bad option to it; `unfunction` is `unset -f` and takes
// neither `-f` nor `-v`. Measured 2026-09-08 by sweeping the alphabet.
func TestTheLettersAreNarrowerThanTheBuiltinRenamed(t *testing.T) {
	for _, word := range []string{"functions -f", "functions -p", "functions -F"} {
		out, _ := runZsh(t, t.TempDir(), "f(){ :; }\n"+word+"\necho st=$?\n")
		if !strings.Contains(out, "bad option: -") || !strings.Contains(out, "st=1\n") {
			t.Errorf("%s: output = %q, want a bad option", word, out)
		}
		if strings.Contains(out, "f () {") {
			t.Errorf("%s: output = %q, want no listing from a refused letter", word, out)
		}
	}
	for _, word := range []string{"unfunction -f f", "unfunction -v f", "unfunction -M f"} {
		out, _ := runZsh(t, t.TempDir(), "f(){ :; }\n"+word+"\nfunctions f\n")
		if !strings.Contains(out, "bad option: -") {
			t.Errorf("%s: output = %q, want a bad option", word, out)
		}
		if !strings.Contains(out, "f () {") {
			t.Errorf("%s: output = %q, want the function still there", word, out)
		}
	}
}

// TestTheMathFunctionFacilityIsRefusedByName. `functions -M` is the one thing
// under this word that is not a rename: it registers a shell function as a
// math function callable from arithmetic, which needs the evaluator to call
// back into the interpreter. It is refused as *missing* rather than as
// unknown, so a script can tell a shell that lacks the facility from a typo —
// and it must never be taken in silence, because the plugin manager that
// needs it writes `functions -M ... 2>/dev/null` and would then read a
// registration that never happened.
func TestTheMathFunctionFacilityIsRefusedByName(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ REPLY=$(($1+100)); }\nfunctions -M -- mf 1 1 g\necho st=$?\n")
	if !strings.Contains(out, "-M is not implemented yet") {
		t.Errorf("output = %q, want the letter refused by name", out)
	}
	if strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q, want a failing status", out)
	}
	// Every other letter this shell spells and this engine does not is the
	// same refusal rather than "bad option", which is a different and worse
	// answer about a letter zsh really has.
	for _, letter := range []string{"-u", "-U", "-k", "-z", "-t", "-T", "-W", "-c"} {
		out, _ := runZsh(t, t.TempDir(), "f(){ :; }\nfunctions "+letter+" f\n")
		if !strings.Contains(out, letter+" is not implemented yet") {
			t.Errorf("functions %s: output = %q, want it named as missing", letter, out)
		}
	}
}

// TestTheMatchingLetterIsAListingAndARemoval — `-m` is the one letter of
// either word this engine implements, and the two disagree about an empty
// match on purpose: a listing that found nothing has answered, a removal that
// removed nothing has not.
func TestTheMatchingLetterIsAListingAndARemoval(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "fa(){ :; }\ngb(){ :; }\nfunctions -m 'f*'\necho st=$?\n")
	if !strings.Contains(out, "fa () {") || strings.Contains(out, "gb () {") {
		t.Errorf("output = %q, want only the matching one", out)
	}
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q, want 0", out)
	}
	out, _ = runZsh(t, t.TempDir(), "fa(){ :; }\nfunctions -m 'zz*'\necho st=$?\n")
	if out != "st=0\n" {
		t.Errorf("output = %q, want a listing of nothing at 0", out)
	}
	out, _ = runZsh(t, t.TempDir(), "fa(){ :; }\ngb(){ :; }\nunfunction -m 'f*'\necho st=$?\nfunctions\n")
	if strings.Contains(out, "fa () {") || !strings.Contains(out, "gb () {") {
		t.Errorf("output = %q, want only the matching one removed", out)
	}
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q, want 0 where something matched", out)
	}
	out, _ = runZsh(t, t.TempDir(), "fa(){ :; }\nunfunction -m 'zz*'\necho st=$?\n")
	if out != "st=1\n" {
		t.Errorf("output = %q, want 1 where nothing matched", out)
	}
}

// TestTheMatchingLetterOnUnsetPicksTheFunctionTable. `unset -f -m` is the
// spelling `unfunction -m` is a name for, and the letter used to be read
// first: the patterns walked the *parameter* table, so the variables went and
// every function named survived, in silence at 0.
func TestTheMatchingLetterOnUnsetPicksTheFunctionTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"fa(){ :; }\nfa=keep\nunset -f -m 'f*'\necho st=$?\necho \"v=[$fa]\"\nfunctions\n")
	if strings.Contains(out, "fa () {") {
		t.Errorf("output = %q, want the function removed", out)
	}
	if !strings.Contains(out, "v=[keep]") {
		t.Errorf("output = %q, want the parameter left alone", out)
	}
	if !strings.Contains(out, "st=0\n") || st != 0 {
		t.Errorf("output = %q status %d, want 0", out, st)
	}
	// And without `-f` the same patterns are the parameter table, which is
	// the half that must not move.
	out, _ = runZsh(t, t.TempDir(), "fa(){ :; }\nfa=keep\nunset -m 'f*'\necho \"v=[${fa-gone}]\"\nfunctions\n")
	if !strings.Contains(out, "fa () {") || !strings.Contains(out, "v=[gone]") {
		t.Errorf("output = %q, want the parameter gone and the function kept", out)
	}
}
