// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind the **names-only** shape of a declaration's listing:
// the sign, read where no pattern was written. Tests name the axis and the
// letter and never a shell — the measurements are in
// interp/declarebuiltin.go and dialect/zsh/namesonlylisting_test.go.
//
// Two axes carry it, because the three shells with the builtin give three
// answers between them: Semantics.SignAloneIsAnOptionWord decides whether a
// bare `-` or `+` is an option word at all, and
// Semantics.FunctionNamesUnderPlus decides whether a plus-signed `f` names
// its functions or goes on writing them.

// withNamesOnly is declRun's setter for a dialect that reads the sign the way
// the shells with a names-only listing do. The attribute-word listing comes
// with it because the bare plus writes that listing's rows without their
// values, and a dialect with no such listing could not be asked.
func withNamesOnly(s *Semantics) {
	s.DeclareOptions = "aAfgilmprux"
	s.BareTypesetListing = BareLocalListsEveryParameter
	s.DeclaredNameWithoutValueIsEmpty = Yes
	s.SignAloneIsAnOptionWord = Yes
	s.FunctionNamesUnderPlus = Yes
}

// The plus sense of the `f` letter, with no pattern anywhere on the line —
// the row the `-m` path answered and this one did not.
func TestAPlusSignedFunctionLetterNamesItsFunctions(t *testing.T) {
	src := "pb() { echo two; }\npa() { echo one; }\n"
	for _, tc := range []struct{ name, line, want string }{
		{"with no operand", "typeset +f", "pa\npb\n"},
		{"with an operand", "typeset +f pa", "pa\n"},
		// The *letter's* sign and not the word's, and the last one written
		// wins: the two lines below differ in nothing else.
		{"the last letter decides, plus", "typeset -f +f pa", "pa\n"},
		{"the last letter decides, minus", "typeset +f -f pa", "pa () \n{ \n  echo one\n}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if out != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// The other answer to the same axis: a dialect whose plus form is not a
// names-only listing goes on writing the bodies, and its own `-F` letter is
// unaffected. Both answers are asserted because a fix that ignored the axis
// would pass the row above and fail this one.
func TestAPlusSignedFunctionLetterOnlyNamesWhereTheDialectSaysSo(t *testing.T) {
	src := "pa() { echo one; }\ntypeset +f pa"
	out, errs, st := declRun(t, src, func(s *Semantics) {
		withNamesOnly(s)
		s.FunctionNamesUnderPlus = No
	}, Diagnostics{})
	want := "pa () \n{ \n  echo one\n}\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("typeset +f pa = %q (stderr %q, status %d), want the body %q", out, errs, st, want)
	}
}

// A status the sign does not touch: a name nobody defined is silent at 1, and
// the 1 stands however many other names printed.
func TestANamedFunctionListingReportsOneForANameItDoesNotHold(t *testing.T) {
	src := "pa() { echo one; }\ntypeset +f pa nosuch"
	out, errs, st := declRun(t, src, withNamesOnly, Diagnostics{})
	if out != "pa\n" || errs != "" || st != 1 {
		t.Errorf("typeset +f pa nosuch = %q (stderr %q, status %d), want %q at 1", out, errs, st, "pa\n")
	}
}

// A sign carrying no letters is an option word where the axis says so, and
// the two signs are two listings: values under a minus, attribute words and
// the bare name under a plus.
func TestABareSignIsAnOptionWordAndPicksTheListing(t *testing.T) {
	src := "qa=1\ntypeset -i qb=2\n"
	for _, tc := range []struct{ name, line, want string }{
		{"a minus is the bare listing", "typeset -", "qa=\"1\"\ninteger qb=\"2\"\n"},
		{"a plus leaves the values off", "typeset +", "qa\ninteger qb\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// onlyQ keeps the rows naming the parameters a test wrote, so that a listing
// of the whole table can be compared *exactly* rather than by containment —
// which is what lets these rows assert an absence, and a listing that wrote
// values where it should have written names fails on the value rather than on
// a count.
//
// A Runner is born holding `IFS`, `OPTIND`, `PPID`, `PWD` and `TMPDIR`, and
// the last two hold a temporary directory whose generated name can contain
// any letter — so the test is on the *name* position and not on the line,
// which a plain substring search got wrong the first time.
var qNameRow = regexp.MustCompile(`(^|\s)q[a-z0-9_]*(=|$)`)

func onlyQ(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if qNameRow.MatchString(line) {
			keep = append(keep, line)
		}
	}
	if len(keep) == 0 {
		return ""
	}
	return strings.Join(keep, "\n") + "\n"
}

// And where the axis says otherwise the same word is an operand — a name, and
// not one a script may declare. The refusal is the dialect's own and is
// asserted by its status and its silence on stdout, since the wording belongs
// to whichever preset supplied it.
func TestABareSignIsANameWhereTheDialectSaysSo(t *testing.T) {
	out, errs, st := declRun(t, "pa=1\ntypeset +", func(s *Semantics) {
		withNamesOnly(s)
		s.SignAloneIsAnOptionWord = No
	}, Diagnostics{})
	if out != "" || st == 0 {
		t.Errorf("typeset + = %q (stderr %q, status %d), want the operand refused", out, errs, st)
	}
	if !strings.Contains(errs, "+") {
		t.Errorf("stderr = %q, want the sign named as the operand it is", errs)
	}
}

// A plus-signed attribute letter with no pattern is that letter's filter over
// the whole table: the names carrying the attribute, and no attribute words in
// front of them. Two letters are a union and not an intersection, which is the
// row a single letter cannot discriminate.
func TestAPlusSignedAttributeLetterIsAFilterOverTheWholeTable(t *testing.T) {
	src := "qa=1\nexport qb=2\ntypeset -i qc=3\n"
	for _, tc := range []struct{ name, line, want string }{
		{"one letter", "typeset +x", "qb\n"},
		{"the other letter", "typeset +i", "qc\n"},
		{"two letters are a union", "typeset +xi", "qb\nqc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, src+tc.line, withNamesOnly, Diagnostics{})
			if onlyQ(out) != tc.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q", tc.line, out, errs, st, tc.want)
			}
		})
	}
}

// A letter the dialect spells and this engine records nothing for is still an
// attribute to select on: nothing carries it, so the listing is empty rather
// than whole. The control is the letter that says where a declaration lands
// rather than what a name is — that one drops out and the whole table stands.
func TestALetterWithoutEffectSelectsNothingAndTheGlobalLetterSelectsEverything(t *testing.T) {
	inert := func(s *Semantics) {
		withNamesOnly(s)
		s.DeclareOptions = "aAfgilmpruxz"
		s.DeclareOptionsWithoutEffect = "z"
	}
	src := "qa=1\n"
	out, errs, st := declRun(t, src+"typeset -z", inert, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("typeset -z = %q (stderr %q, status %d), want silence at 0", out, errs, st)
	}
	out, errs, st = declRun(t, src+"typeset +z", inert, Diagnostics{})
	if out != "" || errs != "" || st != 0 {
		t.Errorf("typeset +z = %q (stderr %q, status %d), want silence at 0", out, errs, st)
	}
	// The same shape with the letter that filters nothing, which is what
	// keeps the row above from reading as "any letter this engine does not
	// model prints nothing".
	for _, line := range []string{"typeset -g", "typeset +g"} {
		out, errs, st = declRun(t, src+line, inert, Diagnostics{})
		if onlyQ(out) != "qa=\"1\"\n" || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want the whole table", line, out, errs, st)
		}
	}
	// And the inert letter still *declares* where a name was written, which
	// is the half that says it was taken rather than skipped.
	out, errs, st = declRun(t, "typeset -z qz\ntypeset -p qz", inert, Diagnostics{})
	if !strings.Contains(out, "qz") || errs != "" || st != 0 {
		t.Errorf("typeset -z qz = %q (stderr %q, status %d), want the name declared", out, errs, st)
	}
}
