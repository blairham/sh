// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `E` letter of a declaration, and the three axes that arrived with it.
// Tests name axes and never shells; what the real panel does is in the corpus
// and in the dialect suites.

// withExponentLetter gives the synthetic dialect both float letters, so a row
// can say which of the two declared a name.
func withExponentLetter(format FloatFormatPolicy) func(*Semantics) {
	return func(s *Semantics) {
		withFloatLetter(s)
		s.DeclareOptions = "aAiEFprx"
		s.DeclareOptionsTakingANumber = "EF"
		s.FloatFormatLetterE = format
		// A bad name on a declaration is reported and the line goes on, so a
		// row where the number is left an operand can still say what the
		// name holds. The other answer ends the script and the row would
		// have nothing to read.
		s.BadNameToDeclarationFatal = No
	}
}

// The letter is a **format** and not a width, which is the whole of why it is
// an axis: the two readings agree on almost no value a script would write.
func TestTheExponentLetterRendersByTheAxis(t *testing.T) {
	for _, c := range []struct {
		name, src   string
		exponent    string
		significant string
	}{
		{"three figures", `typeset -E 3 v=3.14159; echo "$v"`, "3.14e+00", "3.14"},
		{"a small number", `typeset -E 3 v=0.000123456; echo "$v"`, "1.23e-04", "0.000123"},
		// The discriminating row: a number that needs no exponent gets one
		// under the first reading and none under the second.
		{"no exponent needed", `typeset -E 3 v=100; echo "$v"`, "1.00e+02", "100"},
		// One figure is no places at all under the first reading, which is
		// the n−1 in so many words.
		{"one figure", `typeset -E 1 v=3.14159; echo "$v"`, "3e+00", "3"},
		{"the default is ten", `typeset -E v=1.5; echo "$v"`, "1.500000000e+00", "1.5"},
		{"a word that is no number", `typeset -E 3 v=abc; echo "$v"`, "0.00e+00", "0"},
		// The rendered text is what is stored, the same as `-F`'s: the
		// length counts the characters written.
		{"the text is what is stored", `typeset -E 3 v=100; echo "${#v}"`, "8", "3"},
	} {
		for _, p := range []struct {
			name   string
			policy FloatFormatPolicy
			want   string
		}{
			{"an exponent with n-1 places", FloatFormatExponentWithPlaces, c.exponent},
			{"n significant digits", FloatFormatSignificantDigits, c.significant},
		} {
			t.Run(c.name+"/"+p.name, func(t *testing.T) {
				out, errs, st := floatRun(t, c.src, withExponentLetter(p.policy), Diagnostics{})
				if st != 0 || errs != "" {
					t.Fatalf("status %d stderr %q", st, errs)
				}
				if got := strings.TrimSuffix(out, "\n"); got != p.want {
					t.Errorf("got %q, want %q", got, p.want)
				}
			})
		}
	}
}

// A dialect that spells the letter and has not said which rendering it means
// refuses by name, the way every unanswered axis does — rather than picking
// one shell's numbers and reporting success.
func TestAnUnansweredFloatFormatRefuses(t *testing.T) {
	_, errs, st := floatRun(t, `typeset -E 3 v=1.5`,
		withExponentLetter(FloatFormatUnspecified), Diagnostics{})
	if st == 0 || !strings.Contains(errs, "the `E` letter of a declaration") {
		t.Errorf("status %d stderr %q, want the axis refused by name", st, errs)
	}
}

// The plain letter is unaffected by the axis, which is what says the two
// letters are one attribute with two renderings rather than two attributes.
func TestThePlainFloatLetterIgnoresTheFormat(t *testing.T) {
	for _, p := range []FloatFormatPolicy{FloatFormatExponentWithPlaces, FloatFormatSignificantDigits} {
		out, errs, st := floatRun(t, `typeset -F 3 v=100; echo "$v"`, withExponentLetter(p), Diagnostics{})
		if st != 0 || errs != "" {
			t.Fatalf("status %d stderr %q", st, errs)
		}
		if got := strings.TrimSuffix(out, "\n"); got != "100.000" {
			t.Errorf("under %v: got %q, want the places reading", p, got)
		}
	}
}

// Whether a **detached** number reaches a letter that does not end its option
// word, which is the rule that makes any numeric letter reachable under the
// stricter dialect's name.
func TestADetachedNumberAndTheWordEnd(t *testing.T) {
	dg := Diagnostics{BuiltinBadNameNumeric: map[string]string{"typeset": "not an identifier: %[2]s"}}
	for _, c := range []struct {
		name, src    string
		anywhere     string
		onlyAtTheEnd string
	}{
		{
			// The letter ends its word under both answers, so this is the
			// control: it says the rows below are about the *position* and
			// not about the letter having stopped taking a number.
			name: "the letter ends the word", src: `typeset -E 3 v=3.14159; echo "[$v]"`,
			anywhere: "[3.14e+00]", onlyAtTheEnd: "[3.14e+00]",
		},
		{
			// A letter behind it: one answer takes the number and discards
			// the `x`, the other leaves the `3` an operand and refuses it.
			name: "a letter behind it", src: `typeset -Ex 3 v=3.14159; echo "[$v]"`,
			// The `3` is an operand under the strict answer and is refused
			// as a name, so the declaration is the bare `-E` — the letter's
			// default precision, over the value the operand behind it
			// carried.
			anywhere: "[3.14e+00]", onlyAtTheEnd: "[3.141590000e+00]",
		},
		{
			// And a letter in *front* of it is the same word-end under both,
			// which is the pair that says the rule is not "the letter stands
			// alone".
			name: "a letter in front of it", src: `typeset -xE 3 v=3.14159; echo "[$v]"`,
			anywhere: "[3.14e+00]", onlyAtTheEnd: "[3.14e+00]",
		},
	} {
		for _, p := range []struct {
			name string
			only Answer
			want string
		}{
			{"wherever the letter stands", No, c.anywhere},
			{"only where the letter ends its word", Yes, c.onlyAtTheEnd},
		} {
			t.Run(c.name+"/"+p.name, func(t *testing.T) {
				set := func(s *Semantics) {
					withExponentLetter(FloatFormatExponentWithPlaces)(s)
					s.DeclareNumberDetachedOnlyAtTheWordEnd = p.only
				}
				out, _, _ := floatRun(t, c.src, set, dg)
				if got := strings.TrimSuffix(out, "\n"); got != p.want {
					t.Errorf("got %q, want %q", got, p.want)
				}
			})
		}
	}
}

// What a **bare** float letter does to a name that already has a precision.
//
// Read through a later assignment rather than off the standing value: the
// text already stored was rendered at the old precision under both answers,
// so only what the *next* assignment renders as says which attribute the name
// now carries.
func TestABareFloatLetterAndAStandingPrecision(t *testing.T) {
	for _, p := range []struct {
		name  string
		reset Answer
		want  string
	}{
		{"kept", No, "[1.23e+00]"},
		{"reset to the default", Yes, "[1.234567890e+00]"},
	} {
		t.Run(p.name, func(t *testing.T) {
			set := func(s *Semantics) {
				withExponentLetter(FloatFormatExponentWithPlaces)(s)
				s.BareFloatLetterResetsThePrecision = p.reset
			}
			src := `typeset -E 3 v=1.5; typeset -E v; v=1.23456789; echo "[$v]"`
			out, errs, st := floatRun(t, src, set, Diagnostics{})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != p.want {
				t.Errorf("got %q, want %q", got, p.want)
			}
		})
	}
}

// Whether the integer letter and a float letter may stand on one declaration.
func TestTheNumericLettersTogether(t *testing.T) {
	dg := Diagnostics{BuiltinUsage: map[string]string{"typeset": "typeset: usage: typeset [-aAiEFprx]"}}
	for _, c := range []struct{ name, src, read, taken string }{
		// The first letter written wins where the pair is taken, which the
		// parse settles and this axis does not.
		// The `3` is the *integer base* under this reading, which is what
		// "the first letter written wins" means when that letter is `i`:
		// the number it takes is a base and the value renders in it.
		{"the integer letter first", `typeset -iE 3 v=1.5`, `echo "[$v]"`, "[3#1]"},
		{"the float letter first", `typeset -Ei 3 v=1.5`, `echo "[$v]"`, "[1.50e+00]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			set := func(s *Semantics) {
				withExponentLetter(FloatFormatExponentWithPlaces)(s)
				s.NumericTypeLettersAreExclusive = No
			}
			out, errs, st := floatRun(t, c.src+"; "+c.read, set, dg)
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q", st, errs)
			}
			if got := strings.TrimSuffix(out, "\n"); got != c.taken {
				t.Errorf("got %q, want %q", got, c.taken)
			}
		})
		t.Run(c.name+"/refused", func(t *testing.T) {
			set := func(s *Semantics) {
				withExponentLetter(FloatFormatExponentWithPlaces)(s)
				s.NumericTypeLettersAreExclusive = Yes
				// The two axes meet here and the pairing is not a choice:
				// under the loose reading the first number-taking letter
				// swallows the rest of its word, so the second letter is
				// never *written* as far as the flags are concerned and
				// there is no pair left to refuse. The dialect that refuses
				// the pair is the one that reads a detached number only at
				// the word end, which is what keeps both letters.
				s.DeclareNumberDetachedOnlyAtTheWordEnd = Yes
			}
			out, errs, st := floatRun(t, c.src, set, dg)
			if st != 2 || !strings.Contains(errs, "typeset: usage:") {
				t.Errorf("status %d stderr %q out %q, want the usage block at 2", st, errs, out)
			}
		})
	}
}
