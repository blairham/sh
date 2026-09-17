// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `#` after a letter in the option string: a numeric argument type in one
// dialect, and another option letter in the rest (#2947).
//
// The axis is asked nowhere else, so every row here spells an option string
// with a `#` in it — a string without one reaches no question, which the last
// test in this file is what proves.

// numericSem is the shared vector, with the two counting axes answered because
// the numeric branch reaches them: an argument inside the word leaves the scan
// part-way along it, which is the same position a cluster leaves.
func numericSem(a Answer) Semantics {
	s := getoptsSem()
	s.GetoptsCountsTheWordAtItsFirstLetter = No
	s.GetoptsCountsTheWordOnTheNextCall = No
	s.GetoptsEndOfOptionsNamesIt = Yes
	s.GetoptsOptionStringHasANumericType = a
	return s
}

// The two spellings, both ways round. Each prints the letter, OPTARG and
// OPTIND, so a wrong answer is visible in whichever of the three moved.
func TestGetoptsNumericArgumentTypeAxis(t *testing.T) {
	// WORDS is the placeholder, because the snippet is full of printf verbs
	// and a %s substituted into it would land in the first of them.
	const src = `set -- WORDS
while getopts 'n#' o 2>/dev/null; do printf '[%s:%s]' "$o" "${OPTARG-unset}"; done
printf ' end=%s\n' "$OPTIND"`

	for _, tc := range []struct {
		name, words string
		// The number type reads the digits; without it the `#` is a second
		// option letter and the digits are an operand or an option run.
		number, letter string
	}{
		{"the argument in the next word", "-n 5", "[n:5] end=3\n", "[n:unset] end=2\n"},
		{"the argument attached", "-n5", "[n:5] end=2\n", "[n:unset][?:unset] end=2\n"},
		{"a negative number", "-n -3 rest", "[n:-3] end=3\n", "[n:unset][?:unset] end=3\n"},
		{"digits then an operand", "-n7 rest", "[n:7] end=2\n", "[n:unset][?:unset] end=2\n"},
		{"nothing numeric there", "-n", "[?:unset] end=2\n", "[n:unset] end=2\n"},
		// Attached and not a numeral: the rest of the word goes with the
		// complaint under the type, and is read as option letters without it.
		{"attached and not a numeral", "-nab", "[?:unset] end=2\n", "[n:unset][?:unset][?:unset] end=2\n"},
		// The wart, recorded rather than tidied: an attached argument that
		// does not fill the word puts the **rest of the word** in OPTARG and
		// leaves the scan on the byte after the numeral.
		{"digits and a letter in one word", "-n5x", "[n:5x][?:unset] end=2\n", "[n:unset][?:unset][?:unset] end=2\n"},
		// The marker itself: a `#` the numeric type has consumed is not an
		// option at all, where without the type it is the second letter of
		// the string and `-#` is a perfectly good option.
		{"a marker is not a letter", "-#", "[?:unset] end=2\n", "[#:unset] end=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.number}, {No, tc.letter}} {
				sem := numericSem(a.answer)
				out, _ := run(t, strings.Replace(src, "WORDS", tc.words, 1), func(r *Runner) { r.Semantics = &sem })
				if out != a.want {
					t.Errorf("answer %v: got %q, want %q", a.answer, out, a.want)
				}
			}
		})
	}
}

// What the numeral reader takes and what it refuses, asked through the
// builtin rather than of the function, so the rule is pinned where a script
// can see it.
func TestGetoptsNumericArgumentIsANumeralAndNotAnExpression(t *testing.T) {
	// Taken: a sign, a radix prefix, a named base, leading blanks, and a
	// leading zero that is no octal escape hatch. The empty word and a word
	// of blanks are taken too — nothing is left over, which is the whole of
	// the test.
	taken := []string{
		"5", "-3", "+4", "007", "08", "00", "0x10", "0X1F",
		"16#FF", "36#z", "64#_", " 5", "",
	}
	// Refused: an expression, a float, a trailing byte, a base the notation
	// does not have, and a digit the base cannot use.
	refused := []string{
		"abc", "1+1", "3.5", "5x", "5 ", "-x", "0b101", "0x", "1_0",
		"2#", "2#12", "1#0", "0#5", "99#5", "65#1",
	}
	sem := numericSem(Yes)
	check := func(word, want string) {
		t.Helper()
		src := "set -- -n '" + word + "'\n" +
			`getopts 'n#' o 2>/dev/null; printf '[%s:%s]' "$o" "${OPTARG-unset}"`
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if out != want {
			t.Errorf("-n %q: got %q, want %q", word, out, want)
		}
	}
	for _, w := range taken {
		check(w, "[n:"+w+"]")
	}
	for _, w := range refused {
		check(w, "[?:unset]")
	}
}

// Silent mode reports the argument failure as `:`, the way a missing string
// argument is reported, rather than as the unknown-option `?`.
func TestGetoptsNumericArgumentInSilentMode(t *testing.T) {
	sem := numericSem(Yes)
	for _, tc := range []struct{ name, words, want string }{
		{"not a number", "-n abc", "[:][n]\n"},
		{"missing entirely", "-n", "[:][n]\n"},
		{"a letter the string has not", "-z", "[?][z]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "set -- " + tc.words + "\ngetopts ':n#' o; echo \"[$o][${OPTARG-unset}]\""
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The axis is asked at the disagreement and nowhere else: an option string
// with no `#` where a marker goes runs on an unanswered axis without a word,
// and one with a marker refuses.
func TestGetoptsAsksTheNumericAxisOnlyWhereAMarkerIs(t *testing.T) {
	sem := numericSem(Unspecified)
	out, _ := run(t, `set -- -a val; getopts 'a:b' o; echo "[$o][$OPTARG]"`,
		func(r *Runner) { r.Semantics = &sem })
	if out != "[a][val]\n" {
		t.Errorf("an ordinary option string reached the axis: %q", out)
	}
	out, _ = run(t, `set -- -n 5; getopts 'n#' o; echo "[$o]"`,
		func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "a `#` in an option string") {
		t.Errorf("got %q, want the unanswered-axis refusal", out)
	}
}
