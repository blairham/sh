// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a declaration that changes a float name's letter does with the number
// the name was holding. Tests name the axis and never a shell.
//
// The table is built so that the **noun** is under test rather than merely
// varied: every row starts from a rendering the letter it was made with threw
// digits away from, and then asks for a wider one. Under "keep the number"
// the lost digits come back; under "re-read the rendering" they do not. The
// rows that move only the *precision* are the other half of the noun — they
// are the same shape and the same width of change, and the axis must not move
// them.
func TestAFloatLetterChangeRereadsByTheAxis(t *testing.T) {
	const x = "3.14159265358979"
	for _, c := range []struct {
		name, src    string
		reread, keep string
	}{
		// The letter moves.
		{
			"from places to figures",
			`typeset -F 1 f=` + x + `; typeset -E 10 f; echo "$f"`,
			"3.1", "3.141592654",
		},
		{
			"from figures to places",
			`typeset -E 3 f=` + x + `; typeset -F 14 f; echo "$f"`,
			"3.14000000000000", "3.14159265358979",
		},
		{
			"and back again",
			`typeset -F 3 f=` + x + `; typeset -E 5 f; echo "$f"`,
			"3.142", "3.1416",
		},
		// The precision moves, and the letter does not. These must read the
		// same under both answers, which is what says the rule is keyed on
		// the letter rather than on "a declaration that changes something".
		{
			"the same letter, a wider precision",
			`typeset -F 1 f=` + x + `; typeset -F 14 f; echo "$f"`,
			"3.14159265358979", "3.14159265358979",
		},
		{
			"the same letter, wider still",
			`typeset -E 3 f=` + x + `; typeset -E 10 f; echo "$f"`,
			"3.141592654", "3.141592654",
		},
		{
			"the same letter at the same precision",
			`typeset -E 3 f=` + x + `; typeset -E 3 f; echo "$f"`,
			"3.14", "3.14",
		},
		// And the number itself is untouched either way until the letter
		// arrives, which is what says this is not a rounding on assignment.
		{
			"the arithmetic value under a narrow rendering",
			`typeset -E 3 f=` + x + `; echo "$(( f ))"`,
			x, x,
		},
	} {
		for _, a := range []struct {
			name   string
			answer Answer
			want   string
		}{
			{"re-reading the rendering", Yes, c.reread},
			{"keeping the number", No, c.keep},
		} {
			t.Run(c.name+"/"+a.name, func(t *testing.T) {
				out, errs, st := floatRun(t, c.src, func(s *Semantics) {
					withExponentLetter(FloatFormatSignificantDigits)(s)
					s.FloatLetterChangeRereadsTheRendering = a.answer
				}, Diagnostics{ArithFloatDigits: 17})
				if st != 0 || errs != "" {
					t.Fatalf("status %d stderr %q", st, errs)
				}
				if got := strings.TrimSuffix(out, "\n"); got != a.want {
					t.Errorf("got %q, want %q", got, a.want)
				}
			})
		}
	}
}

// A dialect that spells both letters and has not answered the axis is refused
// by name where a declaration really changes one, and nowhere else.
func TestAnUnansweredFloatLetterChangeRefusesOnlyWhereItArises(t *testing.T) {
	out, errs, st := floatRun(t, `typeset -F 1 f=3.14159; typeset -F 8 f; echo "$f"`,
		withExponentLetter(FloatFormatSignificantDigits), Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("a precision change: status %d stderr %q", st, errs)
	}
	if got := strings.TrimSuffix(out, "\n"); got != "3.14159000" {
		t.Errorf("a precision change gave %q, want %q", got, "3.14159000")
	}
	_, errs, st = floatRun(t, `typeset -F 1 f=3.14159; typeset -E 8 f`,
		withExponentLetter(FloatFormatSignificantDigits), Diagnostics{})
	if st == 0 || !strings.Contains(errs, "float letter") {
		t.Errorf("a letter change gave %q at %d, want a refusal naming the axis", errs, st)
	}
}
