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

// floatRun is declRun with floating point in the arithmetic — syntax.Dialect
// .ArithFloat — which a float attribute cannot be tested without: the value a
// float name is given is an expression, and `1.5` is `operator expected` to a
// reader that counts in integers. The two go together in every real shell
// that has either, and neither is core.
func floatRun(t *testing.T, src string, set func(*Semantics), dg Diagnostics) (string, string, int) {
	t.Helper()
	dial := syntax.Core()
	dial.ArithFloat = true
	f, err := syntax.Parse(src, dial)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclaredNameWithoutValueIsEmpty = Yes
	sem.ValuelessDeclarationHidesTheOuterValue = Yes
	sem.DeclareListing = DeclareListingClustered
	sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dialect: &dial, Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The float attribute — `typeset -F`, where the letter is a float's precision
// rather than a function listing — and the one rule that reads the number an
// option letter takes: see Semantics.DeclareOptionsTakingANumber. Tests name
// axes and tables, never shells; what the real panel does is in the corpus.

// withFloatLetter gives the synthetic dialect the letter and says it takes a
// number, which are the two halves of having the attribute at all.
func withFloatLetter(s *Semantics) {
	s.DeclareOptions = "aAiFprx"
	s.DeclareOptionsTakingANumber = "F"
	s.IntegerAttributeTakesABase = Yes
	s.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// An attribute arriving over a name that already holds something reads
	// what it finds — `typeset -F 3 v=1.5; typeset -i v` is `1` — which is a
	// question of its own and not this letter's to answer.
	s.AttributeRereadsTheValueItFinds = Yes
}

// The number behind the letter is its argument and not a second name, under
// both spellings — which is the whole of the bug: the detached `3` was read
// as an operand and refused, and the attached `3` as an option letter.
func TestAPrecisionIsAnArgumentAndNotAName(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a precision as a word of its own", `typeset -F 3 v=3.14159; echo "[$v]"`, "[3.142]"},
		{"the same precision attached to the letter", `typeset -F3 v=3.14159; echo "[$v]"`, "[3.142]"},
		{"the letter with no number, at its default", `typeset -F v=1.5; echo "[$v]"`, "[1.5000000000]"},
		{"a zero is the absence of a number", `typeset -F 0 v=3.9; echo "[$v]"`, "[3.9000000000]"},
		{"more places than the value has", `typeset -F 12 v=1; echo "[$v]"`, "[1.000000000000]"},
		{"a negative value keeps its sign", `typeset -F 3 v=-2.5; echo "[$v]"`, "[-2.500]"},
		{"an exponent is a number like any other", `typeset -F 3 v=1e3; echo "[$v]"`, "[1000.000]"},
		{"the value is an expression", `typeset -F 3 v=1+2; echo "[$v]"`, "[3.000]"},
		{"two names behind one number", `typeset -F 3 v=1 w=2; echo "[$v][$w]"`, "[1.000][2.000]"},
		{"the end of the options is not a number", `typeset -F 3 -- v=1.5; echo "[$v]"`, "[1.500]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, withFloatLetter, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q, want a clean declaration", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Only a run of digits is the letter's argument. A word that is not one was
// never an argument and is a name again, which is what keeps `typeset -F abc
// v=1` declaring two floats rather than refusing a precision it cannot read.
func TestOnlyDigitsAreTheLettersArgument(t *testing.T) {
	out, errs, st := floatRun(t, `typeset -F abc v=1; echo "[$v][$abc]"`,
		withFloatLetter, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q, want two names declared", st, errs)
	}
	if got := strings.TrimSuffix(out, "\n"); got != "[1.0000000000][0.0000000000]" {
		t.Errorf("got %q, want both names declared float", got)
	}
}

// One number and not a list: the letter is satisfied by the first, so the
// second is an operand again and meets the name check like any other.
func TestOneNumberSatisfiesTheLetter(t *testing.T) {
	dg := Diagnostics{BuiltinBadNameNumeric: map[string]string{"typeset": "not an identifier: %[2]s"}}
	// Both spellings of the first number, because they satisfy the letter by
	// different routes and only the detached one leaves anything waiting: a
	// word that carried its number attached must not then reach for the next
	// one. Measured — `typeset -F3 4 v=1.5` is `not an identifier: 4` in that
	// shell exactly as the detached spelling is.
	for _, src := range []string{`typeset -F 3 4 v=1.5`, `typeset -F3 4 v=1.5`} {
		t.Run(src, func(t *testing.T) {
			_, errs, _ := floatRun(t, src, withFloatLetter, dg)
			if !strings.Contains(errs, "not an identifier: 4") {
				t.Errorf("stderr = %q, want the second number refused as a name", errs)
			}
			// The control: the first number is not refused, so the row is
			// about the *second* rather than about a letter reading none.
			if strings.Contains(errs, "not an identifier: 3") {
				t.Errorf("stderr = %q, want the first number taken as the precision", errs)
			}
		})
	}
}

// The rendered text is what the name holds, which is the shape the integer
// letter's output base has: every read sees these characters rather than a
// value some later print formats.
func TestAFloatNameStoresTheRenderedText(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"its length is of the digits written", `typeset -F 3 v=1.5; echo "${#v}"`, "5"},
		{"a later assignment folds too", `typeset -F 3 v; v=7; echo "[$v]"`, "[7.000]"},
		{"arithmetic reads the characters back", `typeset -F 3 v=1.5; echo "$((v+1))"`, "2.5"},
		{"a precision over a standing value re-renders it", `v=1.5; typeset -F 3 v; echo "[$v]"`, "[1.500]"},
		// Through the arithmetic and not through strconv: the standing text
		// may be an expression, a based integer or no number at all, and
		// each is what the fold makes of it rather than text left alone.
		{"a standing expression is evaluated", `v=1+2; typeset -F 3 v; echo "[$v]"`, "[3.000]"},
		{"a standing word that is no number is zero", `v=abc; typeset -F 3 v; echo "[$v]"`, "[0.000]"},
		{"a new precision re-renders it again", `typeset -F 3 v=1.5; typeset -F 6 v; echo "[$v]"`, "[1.500000]"},
		{"the bare letter keeps a precision the name has", `typeset -F 3 v=1.5; typeset -F v; echo "[$v]"`, "[1.500]"},
		{"a value that is no number at all is zero", `typeset -F 3 v=abc; echo "[$v]"`, "[0.000]"},
		{"the plus form leaves the text it rendered", `typeset -F 3 v=1.5; typeset +F v; v=2; echo "[$v]"`, "[2]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, withFloatLetter, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The letter the other one replaced is *gone* and not merely outvoted, which
// only a listing can say: attributeFolded reaches the float branch first, so
// a name left carrying both reads exactly like a name carrying one. A test on
// the value alone cannot tell them apart, and a mutant that stopped taking
// the integer letter off survived until this was here.
func TestTheReplacedAttributeIsGoneAndNotOutvoted(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the integer letter, taken off by the float one",
			`typeset -i v=5; typeset -F 3 v; typeset -p v`, `typeset -F v="5.000"`,
		},
		{
			"and its output base with it",
			`typeset -i16 v=255; typeset -F 3 v; typeset -p v`, `typeset -F v="255.000"`,
		},
		{
			"the float letter, taken off by the integer one",
			`typeset -F 3 v=1.5; typeset -i v; typeset -p v`, `typeset -i v="1"`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, func(s *Semantics) {
				withFloatLetter(s)
				s.DeclareListing = DeclareListingExportSpelled
			}, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// The two attributes that say what a name's values *are* cannot both stand,
// and the later declaration is the one that speaks.
func TestTheFloatAndIntegerAttributesAreExclusive(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"float over integer", `typeset -i v=5; typeset -F 3 v; echo "[$v]"`, "[5.000]"},
		{"integer over float", `typeset -F 3 v=1.5; typeset -i v; echo "[$v]"`, "[1]"},
		// The base the integer letter carried goes with it, so what the
		// float letter re-reads is the *number* and not the sixteen
		// characters it was written in. See TestTheReplacedAttributeIsGone-
		// AndNotOutvoted for the half only a listing can say.
		{
			"the base goes with the letter it belonged to",
			`typeset -i16 v=255; typeset -F 3 v; echo "[$v]"`, "[255.000]",
		},
		{"the earlier letter of one word wins", `typeset -Fi 3 v=1.5; echo "[$v]"`, "[1.500]"},
		{"and the other order wins the other way", `typeset -iF 3 v=1.5; echo "[$v]"`, "[3#1]"},
		// Two words rather than one, which is the only shape where both
		// letters are really read: the word ending at the first
		// number-taking letter settles the two rows above before the second
		// letter is reached. The rule is the same one — the first letter
		// written wins — and this is where it is decided rather than fallen
		// into. No shell says this: zsh refuses the combination outright and
		// leaves the name with neither attribute, `typeset v=1.5`, which
		// this engine does not model (#1461). What is pinned is the engine's
		// own rule holding across words as well as within one.
		{"the first letter still wins across two words", `typeset -i -F 3 v=1.5; echo "[$v]"`, "[1]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, withFloatLetter, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A number-taking letter ends its option word only when a number really
// follows: the letters behind it are the letter's own argument then, and
// ordinary options otherwise. Both halves, because a rule that only said the
// letter ends its word would lose every `typeset -ir n` there is.
func TestALetterEndsItsWordOnlyWhenItTakesANumber(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the letter behind it is lost when a number follows",
			`typeset -Fx 3 v=1.5; echo "[$v]"; typeset -p v`, "[1.500]\ntypeset -F v=\"1.500\"",
		},
		{
			"and is an option when none does",
			`typeset -Fx v=1.5; echo "[$v]"; typeset -p v`, "[1.5000000000]\nexport -F v=\"1.5000000000\"",
		},
		{
			"a letter in front of it is kept either way",
			`typeset -rF 3 v=1.5; typeset -p v`, "typeset -Fr v=\"1.500\"",
		},
		{
			"the integer letter reads its base the same way",
			`typeset -ix 16 n=255; echo "[$n]"`, "[16#FF]",
		},
		{
			"and keeps the letter behind it with no base to read",
			`typeset -ix n=255; typeset -p n`, "export -i n=\"255\"",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, func(s *Semantics) {
				withFloatLetter(s)
				s.DeclareListing = DeclareListingExportSpelled
			}, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// A dialect that does not name the letter reads `-F` the way the substrate
// always did — a function listing — and the word after it is a name again.
// The table is what decides, so a table with nothing in it changes nothing.
func TestWithoutTheTableTheLetterIsAFunctionListing(t *testing.T) {
	out, errs, st := declRun(t, "f() { echo hi; }\ntypeset -F",
		withDeclareLetters("aAfFgiprx"), Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("status %d stderr %q", st, errs)
	}
	if !strings.Contains(out, "f") {
		t.Errorf("stdout = %q, want the function named", out)
	}
	// And no number is consumed there: with no letter taking one, `3` is the
	// first operand, which is what bash measures and what the substrate did
	// before any letter took an argument at all.
	dg := Diagnostics{BuiltinBadNameNumeric: map[string]string{"typeset": "not an identifier: %[2]s"}}
	_, errs, _ = declRun(t, `typeset -i 8 n=64`, func(s *Semantics) {
		s.DeclareOptions = "aAiprx"
		s.IntegerAttributeTakesABase = No
	}, dg)
	if !strings.Contains(errs, "not an identifier: 8") {
		t.Errorf("stderr = %q, want the number left an operand", errs)
	}
}

// The dialect is not asked whether its integer letter reads a base unless a
// number is actually written, because an unanswered axis refuses and a plain
// declaration must meet no question at all. The letters *behind* the integer
// letter are the trap: deciding whether they are options looks like it needs
// the answer, and the lookahead is what means it does not.
func TestNoNumberAsksTheDialectNothing(t *testing.T) {
	for _, src := range []string{`typeset -i n=5`, `typeset -irx n=5`, `typeset -ix n=5`} {
		t.Run(src, func(t *testing.T) {
			_, errs, _ := floatRun(t, src, func(s *Semantics) {
				s.DeclareOptions = "aAiFprx"
				s.IntegerAttributeTakesABase = Unspecified
			}, Diagnostics{})
			if strings.Contains(errs, "output base") {
				t.Errorf("stderr = %q, want no question about a base nothing named", errs)
			}
		})
	}
	// The control: a number written down does ask, and the refusal stands.
	_, errs, _ := floatRun(t, `typeset -i 16 n=5`, func(s *Semantics) {
		s.DeclareOptions = "aAiFprx"
		s.IntegerAttributeTakesABase = Unspecified
	}, Diagnostics{})
	if !strings.Contains(errs, "output base") {
		t.Errorf("stderr = %q, want the unanswered axis to refuse", errs)
	}
}

// A plus word takes no number: the letter is being *removed*, so what follows
// it is an operand and meets the name check like any other. Measured — zsh
// declares two names for `typeset +F 3 v=1.5` rather than reading a precision
// it is in the middle of taking away.
func TestThePlusFormReadsNoNumber(t *testing.T) {
	dg := Diagnostics{BuiltinBadNameNumeric: map[string]string{"typeset": "not an identifier: %[2]s"}}
	_, errs, _ := floatRun(t, `typeset +F 3 v=1.5`, withFloatLetter, dg)
	if !strings.Contains(errs, "not an identifier: 3") {
		t.Errorf("stderr = %q, want the number left an operand under the plus form", errs)
	}
	// The control: the same word with a minus reads it as the precision and
	// says nothing, so the row above is about the sign and not about a
	// letter that never takes a number at all.
	out, errs, st := floatRun(t, `typeset -F 3 v=1.5; echo "[$v]"`, withFloatLetter, dg)
	if st != 0 || errs != "" || strings.TrimSuffix(out, "\n") != "[1.500]" {
		t.Errorf("with a minus: got %q stderr %q status %d, want [1.500]", out, errs, st)
	}
}

// A number that will not parse stays with the word it was written in: the
// letter is not left hunting for one in the next word, which would take an
// operand and declare something the script did not ask for. Both shells with
// the attribute refuse this line — on different halves of it — and the shape
// that matters is that neither reads the `4`.
func TestAMalformedAttachedNumberDoesNotReachForTheNextWord(t *testing.T) {
	out, errs, _ := floatRun(t, `typeset -F3g 4 v=1.5; echo "[$v]"`, withFloatLetter, Diagnostics{})
	if errs == "" {
		t.Errorf("stdout %q with nothing on stderr, want the malformed number refused", out)
	}
	if got := strings.TrimSuffix(out, "\n"); got != "[]" {
		t.Errorf("got %q, want the `4` left an operand and nothing declared, not four places", got)
	}
}

// The attribute is a property of the name, so it must not outlive a subshell
// and must not survive the name being unset.
func TestTheFloatAttributeIsTheNames(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a subshell's declaration does not escape it",
			`v=1; (typeset -F 3 v); v=2; echo "[$v]"`, "[2]",
		},
		{
			"unset takes the attribute with the value",
			`typeset -F 3 v=1.5; unset v; v=9; echo "[$v]"`, "[9]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := floatRun(t, c.src, withFloatLetter, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
