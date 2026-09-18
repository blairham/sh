// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
)

// printfSem is a vector with every printf axis answered, so a test that is
// about one of them is not refused for the others.
func printfSem() Semantics {
	s := CoreSemantics()
	s.PrintfReportsBadNumber = No
	s.PrintfEmptyIsNotANumber = No
	s.PrintfBackslashC = PrintfBackslashCLiteral
	s.PrintfQuote = PrintfQuoteAnsiCWord
	s.PrintfLengthModifiers = PrintfLengthModifiersAbsent
	s.PrintfUnfinishedConversionIsAPercent = No
	s.PrintfHexEscape = PrintfHexEscapeAbsent
	s.PrintfBHexEscape = PrintfHexEscapeAbsent
	s.PrintfUnicodeEscape = PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = PrintfUnicodeEscapeAbsent
	s.PrintfEscEscape = No
	s.PrintfCapitalEscEscape = No
	s.PrintfBEscEscape = No
	s.PrintfBCapitalEscEscape = No
	s.PrintfBOctalWithoutZero = No
	s.PrintfBStopIsPadded = Yes
	s.PrintfAbsentNumberIsAnEmptyOne = No
	s.PrintfStarWithoutOperandIsRefused = No
	s.PrintfStarComplaintCostsTheStatus = Yes
	s.PrintfGroupingFlag = Yes
	s.PrintfGroupingFlagAfterTheWidth = No
	s.PrintfStarBesideTheFieldDigits = No
	s.PrintfFlagAfterTheField = No
	s.ArithDivisionByZeroYieldsAValue = No
	s.PrintfNumberOperand = PrintfNumberLeadingNumber
	s.PrintfRefusedOperandKeepsItsLeadingNumber = No
	s.PrintfFloatOperandIsEvaluatedTwice = No
	return s
}

// `\x` in a format is four readings, and they differ on three separate
// details: whether the escape is there at all, how wide the digit run is, and
// what an empty run means.
func TestPrintfHexEscapeIsFourReadings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy PrintfHexEscapePolicy
		src    string
		want   string
	}{
		{"absent leaves the escape as written", PrintfHexEscapeAbsent, `printf 'a\x41Z'`, `a\x41Z`},
		{"a byte reads two digits", PrintfHexEscapeByte, `printf 'a\x41Z'`, "aAZ"},
		{"a byte stops at two", PrintfHexEscapeByte, `printf '[\x0ff]'`, "[\x0ff]"},
		{"a byte is a byte and not a code point", PrintfHexEscapeByte, `printf 'a\x80Z'`, "a\x80Z"},
		{"one digit is enough", PrintfHexEscapeByte, `printf 'a\x1Z'`, "a\x01Z"},
		{"an empty run leaves the escape standing", PrintfHexEscapeByte, `printf 'a\xZ'`, "sh: printf: missing hex digit for \\x\na\\xZ"},
		{"an empty run is a zero", PrintfHexEscapeByteOrNul, `printf 'a\xZ'`, "a\x00Z"},
		{"a code point still stops at two for a byte", PrintfHexEscapeCodePoint, `printf 'a\xffZ'`, "a\xffZ"},
		{"a third digit makes it a code point", PrintfHexEscapeCodePoint, `printf '[\x0ff]'`, "[ÿ]"},
		{"every digit is taken", PrintfHexEscapeCodePoint, `printf '[\x0041]'`, "[A]"},
		{"a code point reads an empty run as a zero", PrintfHexEscapeCodePoint, `printf 'a\xZ'`, "a\x00Z"},
		// Past the last code point there is, the extended form UTF-8 has
		// room for — re-measured 2026-09-12 under `LC_ALL=C`, where ksh93u+
		// writes six bytes for `\x41414141` rather than the nothing this
		// row used to assert. A run too long for the value to hold keeps
		// the low bits, which is why the two below are the same six bytes.
		{"past the last code point there is, the extended encoding", PrintfHexEscapeCodePoint, `printf '[\x41414141]'`, "[\xfd\x81\x90\x94\x85\x81]"},
		{"a run too long to hold keeps the low bits", PrintfHexEscapeCodePoint, `printf '[\x41414141414141414141]'`, "[\xfd\x81\x90\x94\x85\x81]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfHexEscape = tc.policy
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The reading that leaves `\x` standing says so on standard error, and still
// reports success — a warning rather than a failure.
func TestPrintfHexEscapeWithNoDigitsWarnsWithoutFailing(t *testing.T) {
	sem := printfSem()
	sem.PrintfHexEscape = PrintfHexEscapeByte
	diag := Diagnostics{PrintfMissingHexDigit: "printf: no hex digit"}
	out, st := run(t, `printf 'a\xZ'`, func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &diag
	})
	if out != "sh: printf: no hex digit\na\\xZ" || st != 0 {
		t.Errorf("got %q status %d, want the complaint, the text, and 0", out, st)
	}
}

// The axis is asked only where a `\x` is actually in the format.
func TestPrintfHexEscapeIsAskedOnlyWhenOneIsThere(t *testing.T) {
	sem := printfSem()
	sem.PrintfHexEscape = PrintfHexEscapeUnspecified

	out, st := run(t, `printf '[%d]' 42`, func(r *Runner) { r.Semantics = &sem })
	if out != "[42]" || st != 0 {
		t.Errorf("a format with no hex escape: got %q status %d, want [42] and 0", out, st)
	}
	out, st = run(t, `printf 'a\x41Z'`, func(r *Runner) { r.Semantics = &sem })
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("a format with one: got %q status %d, want a refusal and 2", out, st)
	}
}

// A `%b` argument and a format are two escape tables, so `\x` is two
// questions: the format's axis is never put to a `%b` argument and the `%b`
// axis is never put to a format. ksh93 is what makes the distinction real —
// it reads `\x41` in a format and writes the four characters in a `%b`.
func TestPrintfHexEscapeIsAskedPerSite(t *testing.T) {
	t.Run("a %b argument does not ask the format's axis", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfHexEscape = PrintfHexEscapeUnspecified
		sem.PrintfBHexEscape = PrintfHexEscapeByte
		out, st := run(t, `printf '%b' 'a\x41Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != "aAZ" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "aAZ")
		}
	})
	t.Run("a format does not ask the %b axis", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfHexEscape = PrintfHexEscapeByte
		sem.PrintfBHexEscape = PrintfHexEscapeUnspecified
		out, st := run(t, `printf 'a\x41Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != "aAZ" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "aAZ")
		}
	})
	t.Run("ksh93's split: a format has the escape and a %b does not", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfHexEscape = PrintfHexEscapeCodePoint
		sem.PrintfBHexEscape = PrintfHexEscapeAbsent
		out, st := run(t, `printf 'a\x41Z'; printf '%b' 'a\x41Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != `aAZa\x41Z` || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, `aAZa\x41Z`)
		}
	})
	t.Run("a %b with no hex escape asks nothing", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfBHexEscape = PrintfHexEscapeUnspecified
		out, st := run(t, `printf '%b' 'a\tZ'`, func(r *Runner) { r.Semantics = &sem })
		if out != "a\tZ" || st != 0 {
			t.Errorf("got %q status %d, want a tab between the letters and 0", out, st)
		}
	})
	t.Run("a %b with one and no answer is refused", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfBHexEscape = PrintfHexEscapeUnspecified
		out, st := run(t, `printf '%b' 'a\x41Z'`, func(r *Runner) { r.Semantics = &sem })
		want := "sh: printf: \\x in a %b argument: the shells disagree here and no dialect was chosen\na\\x41Z"
		if out != want || st != 2 {
			t.Errorf("got %q status %d, want %q and 2", out, st, want)
		}
	})
}

// `\u` and `\U` in a format are four readings too, and they are not the four
// `\x` has: the three shells that have the escape agree on how wide the digit
// run is and split on what an *empty* run means, where one of the three ends
// the pass rather than substituting or leaving the text.
func TestPrintfUnicodeEscapeIsFourReadings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy PrintfUnicodeEscapePolicy
		src    string
		want   string
	}{
		{"absent leaves the escape as written", PrintfUnicodeEscapeAbsent, `printf 'a\u0041Z'`, `a\u0041Z`},
		{"absent leaves the long spelling too", PrintfUnicodeEscapeAbsent, `printf 'a\U00000041Z'`, `a\U00000041Z`},
		{"four digits after the short spelling", PrintfUnicodeEscapeCodePoint, `printf 'a\u0041Z'`, "aAZ"},
		{"eight after the long one", PrintfUnicodeEscapeCodePoint, `printf 'a\U00000041Z'`, "aAZ"},
		{"a shorter run is enough", PrintfUnicodeEscapeCodePoint, `printf 'a\u41Z'`, "aAZ"},
		{"the short spelling stops at four", PrintfUnicodeEscapeCodePoint, `printf 'a\u00410'`, "aA0"},
		{"the long spelling stops at eight", PrintfUnicodeEscapeCodePoint, `printf 'a\U000000410'`, "aA0"},
		{"the run ends at the first character that is not a digit", PrintfUnicodeEscapeCodePoint, `printf '[\u41Z]'`, "[AZ]"},
		{"an empty run leaves the escape standing", PrintfUnicodeEscapeCodePoint, `printf 'a\uZ'`, "sh: printf: missing unicode digit for \\u\na\\uZ"},
		{"an empty run is a zero", PrintfUnicodeEscapeCodePointOrNul, `printf 'a\uZ'`, "a\x00Z"},
		{"an empty run ends the pass", PrintfUnicodeEscapeCodePointOrTruncate, `printf 'a\uZ'`, "a"},
		{"a value is a code point and not a byte", PrintfUnicodeEscapeCodePoint, `printf '[\u0041]'`, "[A]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfUnicodeEscape = tc.policy
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The truncating reading ends the *pass* and not the builtin, which is what
// separates it from a `\c` that stops: the operands that are left go through
// the format again, so `printf '[%s]\uZ' x y` is `[x][y]` and not `[x]`.
//
// Written with two operands on purpose. One operand cannot tell the two apart,
// which is the whole reason printfPassEnd has three values.
func TestPrintfUnicodeTruncationEndsThePassAndNotTheBuiltin(t *testing.T) {
	t.Run("the loop over the operands goes on", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePointOrTruncate
		out, st := run(t, `printf '[%s]\uZ' x y`, func(r *Runner) { r.Semantics = &sem })
		if out != "[x][y]" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "[x][y]")
		}
	})
	t.Run("a stopping backslash-c does end it", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfBackslashC = PrintfBackslashCStops
		out, st := run(t, `printf '[%s]\cZ' x y`, func(r *Runner) { r.Semantics = &sem })
		if out != "[x]" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "[x]")
		}
	})
	t.Run("a pass that consumes nothing writes nothing and ends", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePointOrTruncate
		out, st := run(t, `printf '\uZ[%s]' x y`, func(r *Runner) { r.Semantics = &sem })
		if out != "" || st != 0 {
			t.Errorf("got %q status %d, want the empty string and 0", out, st)
		}
	})
}

// The reading that leaves the escape standing says so on standard error, and
// still reports success. The letter is a verb, so one wording covers both
// spellings rather than the two drifting apart.
func TestPrintfUnicodeEscapeWithNoDigitsWarnsWithoutFailing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the short spelling", `printf 'a\uZ'`, "sh: printf: no digit for \\u\na\\uZ"},
		{"the long one", `printf 'a\UZ'`, "sh: printf: no digit for \\U\na\\UZ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePoint
			diag := Diagnostics{PrintfMissingUnicodeDigit: `printf: no digit for \%s`}
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &diag
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The axis is asked only where a `\u` or a `\U` is actually there.
func TestPrintfUnicodeEscapeIsAskedOnlyWhenOneIsThere(t *testing.T) {
	sem := printfSem()
	sem.PrintfUnicodeEscape = PrintfUnicodeEscapeUnspecified

	out, st := run(t, `printf '[%d]' 42`, func(r *Runner) { r.Semantics = &sem })
	if out != "[42]" || st != 0 {
		t.Errorf("a format with no unicode escape: got %q status %d, want [42] and 0", out, st)
	}
	out, st = run(t, `printf 'a\u0041Z'`, func(r *Runner) { r.Semantics = &sem })
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("a format with one: got %q status %d, want a refusal and 2", out, st)
	}
}

// Two sites and two axes, exactly as `\x` has them, and ksh93 is again what
// makes the distinction real: it reads `\u0041` in a format and writes the ten
// characters as they stand in a `%b`.
func TestPrintfUnicodeEscapeIsAskedPerSite(t *testing.T) {
	t.Run("a %b argument does not ask the format's axis", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfUnicodeEscape = PrintfUnicodeEscapeUnspecified
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeCodePoint
		out, st := run(t, `printf '%b' 'a\u0041Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != "aAZ" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "aAZ")
		}
	})
	t.Run("a format does not ask the %b axis", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePoint
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeUnspecified
		out, st := run(t, `printf 'a\u0041Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != "aAZ" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "aAZ")
		}
	})
	t.Run("ksh93's split: a format has the escape and a %b does not", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePointOrTruncate
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeAbsent
		out, st := run(t, `printf 'a\u0041Z'; printf '%b' 'a\u0041Z'`, func(r *Runner) { r.Semantics = &sem })
		if out != `aAZa\u0041Z` || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, `aAZa\u0041Z`)
		}
	})
	t.Run("a %b with no unicode escape asks nothing", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeUnspecified
		out, st := run(t, `printf '%b' 'a\tZ'`, func(r *Runner) { r.Semantics = &sem })
		if out != "a\tZ" || st != 0 {
			t.Errorf("got %q status %d, want a tab between the letters and 0", out, st)
		}
	})
	t.Run("a %b with one and no answer is refused", func(t *testing.T) {
		sem := printfSem()
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeUnspecified
		out, st := run(t, `printf '%b' 'a\u0041Z'`, func(r *Runner) { r.Semantics = &sem })
		want := "sh: printf: \\u in a %b argument: the shells disagree here and no dialect was chosen\na\\u0041Z"
		if out != want || st != 2 {
			t.Errorf("got %q status %d, want %q and 2", out, st, want)
		}
	})
	t.Run("a truncating answer at the %b site ends the argument and not the builtin", func(t *testing.T) {
		// No dialect in the panel asks for this, so it is pinned here rather
		// than in a corpus row: the argument's text ends at the escape and
		// the format runs on to what follows the conversion.
		sem := printfSem()
		sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeCodePointOrTruncate
		out, st := run(t, `printf '[%b]!' 'a\uZb'`, func(r *Runner) { r.Semantics = &sem })
		if out != "[a]!" || st != 0 {
			t.Errorf("got %q status %d, want %q and 0", out, st, "[a]!")
		}
	})
}

// One encoder, shared with `echo`'s `\u` and with `print`: the value is written
// as the *original* UTF-8, so a surrogate and a value past the last code point
// are encoded rather than replaced. Asserted as bytes, because a replacement
// character is three bytes too.
//
// In a UTF-8 locale, which is where the *encoder* is the whole of the
// question: every value here is above ASCII, so what a locale with no room
// for one does is the other axis's and is asserted in localeescape_test.go.
func TestPrintfUnicodeEscapeEncodesWhatUnicodeLaterRefused(t *testing.T) {
	sem := printfSem()
	sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePoint
	for _, tc := range []struct{ name, src, want string }{
		{"a surrogate", `printf '[\ud800]'`, "[\xed\xa0\x80]"},
		{"past the last code point", `printf '[\U00110000]'`, "[\xf4\x90\x80\x80]"},
		{"the five-byte form", `printf '[\U00200000]'`, "[\xf8\x88\x80\x80\x80]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Vars = map[string]string{"LC_ALL": "en_US.UTF-8"}
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// A format is a byte string. Every route from the format to the output writes
// the byte it decoded rather than the text an encoding spells that number
// with, which a rune conversion would: 0xc0 is one byte and not two.
//
// The word already held the byte — `%s` and a variable both carried it
// through untouched — so this was never about how a word is read.
func TestPrintfWritesBytesAndNotEncodedRunes(t *testing.T) {
	sem := printfSem()
	sem.PrintfHexEscape = PrintfHexEscapeByte
	for _, tc := range []struct{ name, src, want string }{
		{"a literal byte in the format", "printf 'a\xc0Z'", "a\xc0Z"},
		{"an octal escape in the format", `printf 'a\300Z'`, "a\xc0Z"},
		{"a hexadecimal escape in the format", `printf 'a\xc0Z'`, "a\xc0Z"},
		{"a literal byte in an operand", "printf '%s' 'a\xc0Z'", "a\xc0Z"},
		{"the first byte of a %c operand", "printf '%c' '\xc0Z'", "\xc0"},
		{"a %c operand is still padded", "printf '[%3c]' '\xc0Z'", "[  \xc0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got % x status %d, want % x and 0", out, st, tc.want)
			}
		})
	}
}

// C's `%c` has no precision and Go's `%s` does, so the precision has to be
// taken back out — the character is written through `%s`.
//
// No dialect is consulted, which is the claim as much as the output is: bash
// 5.3, zsh, ksh93, dash and BusyBox ash all ignore it, so an implementation
// that reached an axis here would be asking a question the panel does not
// have. Run under printfSem's core vector for that reason.
//
// The width rows are the other half. A `%c` does have a width, and a fix that
// dropped the whole field rather than the precision would pass every
// precision row here and quietly stop padding.
func TestPrintfCharacterIgnoresItsPrecision(t *testing.T) {
	sem := printfSem()
	for _, tc := range []struct{ name, src, want string }{
		{"a zero precision does not eat the character", `printf '[%.0c]' abc`, "[a]"},
		{"nor does a one", `printf '[%.1c]' abc`, "[a]"},
		{"nor does a larger one repeat it", `printf '[%.3c]' abc`, "[a]"},
		{"a bare point is a zero precision and is ignored too", `printf '[%.c]' abc`, "[a]"},
		{"a star precision is ignored where its operand is zero", `printf '[%.*c]' 0 abc`, "[a]"},
		{"and where it is negative", `printf '[%.*c]' -1 abc`, "[a]"},
		{"the width still pads", `printf '[%5.0c]' abc`, "[    a]"},
		{"and still pads on the right", `printf '[%-5.2c]' abc`, "[a    ]"},
		{"a star width still pads", `printf '[%*.0c]' 4 abc`, "[   a]"},
		{"the empty operand keeps its NUL through a precision", `printf '[%.0c]' ''`, "[\x00]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The C length modifiers are a set with three answers, and every one of them
// reads the letters and throws them away.
func TestPrintfLengthModifiersAreThreeSets(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  PrintfLengthModifierSet
		src  string
		want string
	}{
		{"none takes the letter for the conversion", PrintfLengthModifiersAbsent, `printf "[%ld]" 42`, "sh: printf: %l: invalid directive"},
		{"C89 takes l", PrintfLengthModifiersC89, `printf "[%ld]" 42`, "[42]"},
		{"C89 takes h", PrintfLengthModifiersC89, `printf "[%hd]" 42`, "[42]"},
		{"C89 takes L", PrintfLengthModifiersC89, `printf "[%Lf]" 1.5`, "[1.500000]"},
		{"C89 refuses a doubled letter", PrintfLengthModifiersC89, `printf "[%lld]" 42`, "sh: printf: %ll: invalid directive"},
		{"C89 refuses the C99 additions", PrintfLengthModifiersC89, `printf "[%zX]" 255`, "sh: printf: %z: invalid directive"},
		{"C99 takes the C99 additions", PrintfLengthModifiersC99, `printf "[%zX][%jd][%td]" 255 42 42`, "[FF][42][42]"},
		{"C99 takes a doubled letter", PrintfLengthModifiersC99, `printf "[%lld]" 42`, "[42]"},
		{"C99 takes a run of them", PrintfLengthModifiersC99, `printf "[%llld][%hld]" 42 42`, "[42][42]"},
		{"a modifier changes no width", PrintfLengthModifiersC99, `printf "[%hhd]" 300`, "[300]"},
		{"the flags and width still come first", PrintfLengthModifiersC99, `printf "[%-5.3ld]" 42`, "[042  ]"},
		{"a width after the modifier is not a conversion", PrintfLengthModifiersC99, `printf "[%l5d]" 42`, "sh: printf: %l5: invalid directive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfLengthModifiers = tc.set
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// The axis is asked only where a letter one of the answers would take is
// actually in the format, so the strict core runs `%d` and refuses `%ld`.
func TestPrintfLengthModifiersAreAskedOnlyWhenOneIsThere(t *testing.T) {
	sem := printfSem()
	sem.PrintfLengthModifiers = PrintfLengthModifiersUnspecified

	out, st := run(t, `printf "[%d]" 42`, func(r *Runner) { r.Semantics = &sem })
	if out != "[42]" || st != 0 {
		t.Errorf("a format with no modifier: got %q status %d, want [42] and 0", out, st)
	}
	out, st = run(t, `printf "[%ld]" 42`, func(r *Runner) { r.Semantics = &sem })
	if st != 2 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("a format with one: got %q status %d, want a refusal and 2", out, st)
	}
}

// A format that runs out before it reaches a conversion character is its own
// complaint, with a wording beside the bad-conversion one rather than that
// one spelled with an empty name. One verb: the whole directive, since there
// is no conversion character in it to name.
func TestPrintfMissingVerbNamesTheWholeDirective(t *testing.T) {
	for _, tc := range []struct{ name, wording, src, want string }{
		{"nothing after the percent", "printf: %[1]s: no", `printf "a%"`, "sh: printf: %: no\na"},
		{"nothing after a width", "printf: %[1]s: no", `printf "a%5"`, "sh: printf: %5: no\na"},
		{"nothing after a modifier", "printf: %[1]s: no", `printf "a%ll"`, "sh: printf: %ll: no\na"},
		{"nothing after a precision", "printf: %[1]s: no", `printf "a%."`, "sh: printf: %.: no\na"},
		{"a wording may name nothing at all", "printf: no", `printf "a%5"`, "sh: printf: no\na"},
		{"only the last conversion is unfinished", "printf: %[1]s: no", `printf "a%%b%"`, "sh: printf: %: no\na%b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfLengthModifiers = PrintfLengthModifiersC99
			diag := Diagnostics{PrintfMissingVerb: tc.wording}
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &diag
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The two complaints are separate: a conversion nobody has still gets the
// bad-conversion wording, and a format that ran out never does.
func TestPrintfMissingVerbIsNotTheBadVerbComplaint(t *testing.T) {
	sem := printfSem()
	diag := Diagnostics{
		PrintfBadVerb:           "printf: bad %[2]s",
		PrintfMissingVerb:       "printf: missing %[1]s",
		PrintfBadVerbStatus:     1,
		PrintfMissingVerbStatus: 2,
	}
	set := func(r *Runner) {
		r.Semantics = &sem
		r.Diagnostics = &diag
	}
	if out, st := run(t, `printf "a%v"`, set); out != "sh: printf: bad %v\na" || st != 1 {
		t.Errorf("a conversion nobody has: got %q status %d", out, st)
	}
	if out, st := run(t, `printf "a%5"`, set); out != "sh: printf: missing %5\na" || st != 2 {
		t.Errorf("a format that ran out: got %q status %d", out, st)
	}
}

// One shell does not treat it as an error at all: the whole unfinished
// conversion becomes a single literal percent, prefix and all, and the
// command succeeds.
func TestPrintfUnfinishedConversionCanBeALiteralPercent(t *testing.T) {
	sem := printfSem()
	sem.PrintfLengthModifiers = PrintfLengthModifiersC99
	sem.PrintfUnfinishedConversionIsAPercent = Yes
	for _, tc := range []struct{ src, want string }{
		{`printf "a%"`, "a%"},
		{`printf "a%5"`, "a%"},
		{`printf "a%ll"`, "a%"},
		{`printf "a%%b%"`, "a%b%"},
	} {
		out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// A `*` in the part of an unfinished conversion that *was* written still
// takes its operand, even though the conversion never completes — so the
// format is reused once per star's worth of operands where `%` and `%5`
// consume nothing and the builtin ends after one pass (#2667).
//
// Visible only in the dialect above, because the other four refuse an
// unfinished conversion outright and the pass is over before any operand is
// looked at.
func TestPrintfUnfinishedConversionStillTakesItsStarOperands(t *testing.T) {
	sem := printfSem()
	sem.PrintfLengthModifiers = PrintfLengthModifiersC99
	sem.PrintfUnfinishedConversionIsAPercent = Yes
	for _, tc := range []struct{ src, want string }{
		// Nothing is consumed without a star, so one pass and no more
		// however many operands are waiting.
		{`printf "a%" 5 9`, "a%"},
		{`printf "a%5" 5 9`, "a%"},
		// One operand per star, so one pass per operand.
		{`printf "a%*" 5 9`, "a%a%"},
		{`printf "a%*" 5 9 7`, "a%a%a%"},
		// Two stars take two.
		{`printf "a%*.*" 5 9 7 3`, "a%a%"},
		// Whatever else the unfinished prefix holds — a length modifier is
		// as unfinished as a bare `%`, and a star in the precision reads its
		// operand exactly as one in the width does.
		{`printf "a%*ll" 5 9`, "a%a%"},
		{`printf "a%.*" 5 9`, "a%a%"},
	} {
		out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// The axis is asked only where a format actually ends inside a conversion.
func TestPrintfUnfinishedConversionIsAskedOnlyWhenOneIsThere(t *testing.T) {
	sem := printfSem()
	sem.PrintfUnfinishedConversionIsAPercent = Unspecified

	if out, st := run(t, `printf "[%d]" 42`, func(r *Runner) { r.Semantics = &sem }); out != "[42]" || st != 0 {
		t.Errorf("a format that finishes: got %q status %d, want [42] and 0", out, st)
	}
	// Compared whole rather than searched, because a refusal is one answer:
	// asking the axis, being refused and then complaining as well would
	// still contain the refusal, and would tell a reader the format was
	// wrong on top of telling them nothing chose.
	out, st := run(t, `printf "a%5"`, func(r *Runner) { r.Semantics = &sem })
	want := "sh: a format that ends inside a conversion: the shells disagree here and no dialect was chosen\na"
	if st != 2 || out != want {
		t.Errorf("a format that does not: got %q status %d, want %q and 2", out, st, want)
	}
}

// A conversion nothing has is named two ways, and a length modifier is what
// makes the two visible: the character is not the directive once the
// directive can hold more than a verb.
func TestPrintfBadVerbNamesTheConversionOrTheDirective(t *testing.T) {
	for _, tc := range []struct{ name, wording, src, want string }{
		{"the character alone", "printf: %[1]s: no", `printf "%v]xY" 1`, "sh: printf: v: no\n"},
		{"the whole directive", "printf: %[2]s: no", `printf "%v]xY" 1`, "sh: printf: %v: no\n"},
		{"the character, past a modifier", "printf: %[1]s: no", `printf "%lQ" 1`, "sh: printf: Q: no\n"},
		{"the directive, past a modifier", "printf: %[2]s: no", `printf "%lQ" 1`, "sh: printf: %lQ: no\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfLengthModifiers = PrintfLengthModifiersC99
			diag := Diagnostics{PrintfBadVerb: tc.wording}
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Diagnostics = &diag
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `%c` with no character to write is one NUL byte and not nothing, which is
// the reading of that conversion easiest to get backwards: an empty operand
// looks like there is nothing to write, and every reference writes a byte.
//
// The empty operand and the absent one are one case rather than two. That is
// worth a row each, because the distinction exists a few lines away — a `%d`
// with nothing left is a different question from a `%d` given the empty
// string in ash — and here it makes no difference to a single byte.
//
// Not an axis. bash 5.3, zsh, ksh93, dash and BusyBox ash all write the NUL;
// bash 3.2 alone writes nothing, which is the age of one binary rather than a
// language the dialects have to carry (#2647).
func TestPrintfCWithNothingToWriteIsANul(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an empty operand", `printf "[%c]" ''`, "[\x00]"},
		{"no operand at all", `printf "[%c]"`, "[\x00]"},
		{"two conversions in one pass", `printf "[%c%c]" '' ''`, "[\x00\x00]"},
		{"the format reused around one", `printf "[%c]" a '' b`, "[a][\x00][b]"},
		{"the byte goes through the field", `printf "[%3c][%-3c]" '' ''`, "[  \x00][\x00  ]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The parts every shell in the panel agrees on, which is most of printf and
// worth pinning as the thing a dialect does not get to change.
func TestPrintfTheUnanimousParts(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the format is reused", `printf "[%s]" a b c`, "[a][b][c]"},
		{"a missing argument is empty", `printf "[%s][%s]" a`, "[a][]"},
		{"%b escapes its argument", `printf "[%b]" 'a\tb'`, "[a\tb]"},
		{"%s does not", `printf "[%s]" 'a\tb'`, `[a\tb]`},
		{"the format is always escaped", `printf 'a\tb'`, "a\tb"},
		{"%c is the first character", `printf "[%c]" abc`, "[a]"},
		{"width and precision", `printf "[%5s][%-5s][%.2s]" ab ab abcd`, "[   ab][ab   ][ab]"},
		{"%% is a percent", `printf "100%%"`, "100%"},
		{"an octal escape", `printf "[\101]"`, "[A]"},
		{"a format with no verbs runs once", `printf "x" a b c`, "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `\c` is three things, and the middle one is why it is a policy: a control
// character *looks* like truncation until the bytes are read.
func TestPrintfBackslashCIsThreeThings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy PrintfBackslashCPolicy
		want   string
	}{
		{"literal", PrintfBackslashCLiteral, `a\cbZ`},
		{"control character", PrintfBackslashCControl, "a\x02Z"},
		{"stops the output", PrintfBackslashCStops, "a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBackslashC = tc.policy
			out, _ := run(t, `printf 'a\cbZ'`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The control-character answer is one rule, and a format reads it the same
// way the quoted form does. A letter cannot show that — clearing the top bits
// and toggling bit 6 agree over `@` through `_` — so every case here is
// outside that span or is an argument that has to be decoded first.
func TestPrintfBackslashCControlIsTheQuotedFormsRule(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a digit is not a letter", `printf 'a\c1Z'`, "aqZ"},
		{"a symbol above the letters", `printf 'a\c~Z'`, "a>Z"},
		{"the delete character needs no special case", `printf 'a\c?Z'`, "a\x7fZ"},
		{"a lower-case letter is upper-cased first", `printf 'a\caZ'`, "a\x01Z"},
		{"the argument is decoded before it is controlled", `printf 'a\c\tZ'`, "aIZ"},
		{"a doubled backslash is the one character", `printf 'a\c\\Z'`, "a\x1cZ"},
		{"an escape the dialect does not know loses its backslash", `printf 'a\c\QZ'`, "a\x11Z"},
		{"a hexadecimal argument", `printf 'a\c\x41Z'`, "a\x01Z"},
		{"nothing after the escape is a NUL", `printf 'a\c'`, "a\x00"},
		{"a backslash at the end escapes the end", `printf 'a\c\'`, "a@"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBackslashC = PrintfBackslashCControl
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A control character is a byte and not a code point. Toggling bit 6 of an
// argument above the ASCII range leaves it above it, and what comes out has to
// be that one byte rather than the two its encoding would take.
func TestPrintfBackslashCControlWritesOneByte(t *testing.T) {
	sem := printfSem()
	sem.PrintfBackslashC = PrintfBackslashCControl
	// The argument is written as an octal escape rather than as the byte
	// itself, so the assertion is about the escape and not about how a
	// source file holding a byte no encoding claims is read.
	out, _ := run(t, `printf 'a\c\300Z'`, func(r *Runner) { r.Semantics = &sem })
	if want := "a\x80Z"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestPrintfQuoteIsThreeAnswersAndAnAbsence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style PrintfQuoteStyle
		want  string
	}{
		{"backslash", PrintfQuoteAnsiCWord, `a\ b`},
		{"single quoted", PrintfQuoteSingle, `'a b'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfQuote = tc.style
			out, _ := run(t, `printf "%q" "a b"`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}

	// Absent is not a style but a refusal, and it stops the output where it
	// is rather than writing nothing at all.
	sem := printfSem()
	sem.PrintfQuote = PrintfQuoteAbsent
	out, _ := run(t, `printf "[%q]" "a b"`, func(r *Runner) { r.Semantics = &sem })
	// The complaint first and the `[` after it, because this vector holds
	// output back — the same order the dialect without %q prints.
	if want := "sh: printf: %q: invalid directive\n["; out != want {
		t.Errorf("got %q, want the complaint and the output so far", out)
	}
}

// An operand that is present and empty is an error in two dialects; one that
// is *missing* is an error in only one of those two. The pair is easy to
// conflate and the corpus has a case for each.
func TestPrintfEmptyOperandIsNotAMissingOne(t *testing.T) {
	sem := printfSem()
	sem.PrintfReportsBadNumber = Yes
	sem.PrintfEmptyIsNotANumber = Yes

	out, _ := run(t, `printf "[%d]" ""`, func(r *Runner) { r.Semantics = &sem })
	if want := "sh: printf: : invalid number\n[0]"; out != want {
		t.Errorf("present and empty: got %q, want a complaint and the zero", out)
	}
	out, _ = run(t, `printf "[%d]"`, func(r *Runner) { r.Semantics = &sem })
	if out != "[0]" {
		t.Errorf("missing: got %q, want the zero and no complaint", out)
	}
}

// The absent operand is ash's own answer, and it is a second axis rather than
// a reading of the first: the two cross. bash complains about an operand that
// is present and empty and not about one that is absent; ash complains about
// both; the other three complain about neither (#2648).
func TestPrintfAbsentNumberIsAnEmptyOneIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"absent is its own case", No, "[0]"},
		{"absent is the empty one", Yes, "sh: printf: : invalid number\n[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfReportsBadNumber = Yes
			sem.PrintfEmptyIsNotANumber = Yes
			sem.PrintfAbsentNumberIsAnEmptyOne = tc.answer

			out, st := run(t, `printf "[%d]"`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if want := map[Answer]int{No: 0, Yes: 1}[tc.answer]; st != want {
				t.Errorf("status %d, want %d", st, want)
			}
		})
	}
}

// One complaint per absent operand, and the zero still printed for each. A
// reading that stopped at the first would have been indistinguishable on
// `printf '[%d]'` and wrong on the line that found it.
func TestPrintfAbsentNumberComplainsOncePerConversion(t *testing.T) {
	sem := printfSem()
	sem.PrintfReportsBadNumber = Yes
	sem.PrintfEmptyIsNotANumber = Yes
	sem.PrintfAbsentNumberIsAnEmptyOne = Yes

	out, st := run(t, `printf "[%x][%o][%u]"`, func(r *Runner) { r.Semantics = &sem })
	want := strings.Repeat("sh: printf: : invalid number\n", 3) + "[0][0][0]"
	if out != want || st != 1 {
		t.Errorf("got %q status %d, want %q and 1", out, st, want)
	}
}

// A dialect that lets a present-and-empty operand through is never asked
// about an absent one, because every column that allows the first allows the
// second. Without the guard, a core with neither axis answered would refuse
// `printf '%d'` — which no shell in the panel does.
func TestPrintfAbsentNumberIsNotAskedWhereEmptyIsAllowed(t *testing.T) {
	sem := printfSem()
	sem.PrintfEmptyIsNotANumber = No
	sem.PrintfAbsentNumberIsAnEmptyOne = Unspecified

	out, st := run(t, `printf "[%d]"`, func(r *Runner) { r.Semantics = &sem })
	if out != "[0]" || st != 0 {
		t.Errorf("got %q status %d, want the zero at 0 with the axis left unanswered", out, st)
	}
}

// The complaint arrives before the output or after it, which is only visible
// where both streams reach one place — as they do here.
func TestPrintfOutputPrecedesComplaintIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"held back", No, "sh: printf: abc: invalid number\n[0]"},
		{"written through", Yes, "[sh: printf: abc: invalid number\n0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfReportsBadNumber = Yes
			sem.PrintfOutputPrecedesComplaint = tc.answer
			out, _ := run(t, `printf "[%d]" abc`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A `%b` argument's escapes are their own table, unanimous where the panel
// agrees and four axes where it does not.
//
// The unanimous half is the point of #798: reading a `%b` with the format's
// reader made `\0101` a backspace and a `1` where all six shells write an
// `A`, and made `\c` three different things where all six stop.
func TestPrintfBEscapeTableIsNotTheFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"the XSI eight", `printf '%b' 'a\a\b\f\n\r\t\v\\Z'`, "a\a\b\f\n\r\t\v\\Z"},
		{"an octal introduced by a zero", `printf '%b' 'a\0101Z'`, "aAZ"},
		{"and at most three digits after it", `printf '%b' 'a\01011Z'`, "aA1Z"},
		{"the value is a byte", `printf '%b' 'a\0300Z'`, "a\xc0Z"},
		{"a value past a byte wraps", `printf '%b' 'a\0400Z'`, "a\x00Z"},
		{"a lone zero is a NUL", `printf '%b' 'a\0Z'`, "a\x00Z"},
		{"a digit outside octal ends the run", `printf '%b' 'a\08Z'`, "a\x008Z"},
		{"a backslash at the end is a backslash", `printf '%b' 'a\'`, `a\`},
		{"a character no escape claims keeps its backslash", `printf '%b' 'a\qZ'`, `a\qZ`},
		{"the format's octal reading is not used here", `printf 'a\0101Z'`, "a\b1Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// `\c` in a `%b` argument ends the output, in every shell in the panel — so
// it asks nothing, where the same two characters in a *format* are literal in
// bash and dash, control-X in ksh93 and a full stop in zsh.
func TestPrintfBackslashCInABArgumentAlwaysStops(t *testing.T) {
	for _, policy := range []PrintfBackslashCPolicy{
		PrintfBackslashCUnspecified,
		PrintfBackslashCLiteral,
		PrintfBackslashCControl,
		PrintfBackslashCStops,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBackslashC = policy
			out, st := run(t, `printf '%b' 'a\cbZ'`, func(r *Runner) { r.Semantics = &sem })
			if out != "a" || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, "a")
			}
		})
	}
}

// The four axes a `%b` argument does ask, each asked only where its escape is
// there and each refused by name when no dialect has answered it.
func TestPrintfBEscapeAxes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func(*Semantics)
		src     string
		want    string
		refused string
	}{
		{
			name: "an octal with no zero, taken", src: `printf '%b' 'a\101Z'`, want: "aAZ",
			answer: func(s *Semantics) { s.PrintfBOctalWithoutZero = Yes },
		},
		{
			name: "an octal with no zero, three digits at most", src: `printf '%b' 'a\1011Z'`, want: "aA1Z",
			answer: func(s *Semantics) { s.PrintfBOctalWithoutZero = Yes },
		},
		{
			name: "an octal with no zero, declined", src: `printf '%b' 'a\101Z'`, want: `a\101Z`,
			answer: func(s *Semantics) { s.PrintfBOctalWithoutZero = No },
		},
		{
			name: "an octal with no zero, unanswered", src: `printf '%b' 'a\101Z'`, want: `a\101Z`,
			answer:  func(s *Semantics) { s.PrintfBOctalWithoutZero = Unspecified },
			refused: `printf: \nnn in a %b argument`,
		},
		{
			name: "esc, taken", src: `printf '%b' 'a\eZ'`, want: "a\x1bZ",
			answer: func(s *Semantics) { s.PrintfBEscEscape = Yes },
		},
		{
			name: "esc, declined", src: `printf '%b' 'a\eZ'`, want: `a\eZ`,
			answer: func(s *Semantics) { s.PrintfBEscEscape = No },
		},
		{
			name: "esc, unanswered", src: `printf '%b' 'a\eZ'`, want: `a\eZ`,
			answer:  func(s *Semantics) { s.PrintfBEscEscape = Unspecified },
			refused: `printf: \e in a %b argument`,
		},
		{
			name: "capital esc, taken", src: `printf '%b' 'a\EZ'`, want: "a\x1bZ",
			answer: func(s *Semantics) { s.PrintfBCapitalEscEscape = Yes },
		},
		{
			name: "capital esc, declined", src: `printf '%b' 'a\EZ'`, want: `a\EZ`,
			answer: func(s *Semantics) { s.PrintfBCapitalEscEscape = No },
		},
		{
			name: "capital esc, unanswered", src: `printf '%b' 'a\EZ'`, want: `a\EZ`,
			answer:  func(s *Semantics) { s.PrintfBCapitalEscEscape = Unspecified },
			refused: `printf: \E in a %b argument`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			tc.answer(&sem)
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				// The refusal names the axis and the escape still stands,
				// which is the same shape `\x` takes: an unanswered axis
				// reports and leaves the text alone rather than guessing.
				want := "sh: " + tc.refused + ": the shells disagree here and no dialect was chosen\n" + tc.want
				if out != want || st != 2 {
					t.Errorf("got %q status %d, want %q and 2", out, st, want)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The two spellings of the escape character are two axes, because ksh93 and
// zsh answer them in opposite directions: one answer for both letters is
// wrong for half the panel.
func TestPrintfBEscAndCapitalEscAreTwoQuestions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		esc, capes Answer
		want       string
	}{
		{"bash has both", Yes, Yes, "a\x1bZ:a\x1bZ"},
		{"dash has neither", No, No, `a\eZ:a\EZ`},
		{"ksh93 has the capital alone", No, Yes, "a\\eZ:a\x1bZ"},
		{"zsh has the small alone", Yes, No, "a\x1bZ:a\\EZ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBEscEscape = tc.esc
			sem.PrintfBCapitalEscEscape = tc.capes
			out, st := run(t, `printf '%b' 'a\eZ:a\EZ'`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// An axis is asked only where its escape is actually in the argument, so a
// `%b` that carries none of them needs no dialect at all.
func TestPrintfBEscapeAxesAreAskedOnlyWhenTheEscapeIsThere(t *testing.T) {
	sem := CoreSemantics()
	out, st := run(t, `printf '%b' 'a\t\0101\cZ'`, func(r *Runner) { r.Semantics = &sem })
	if out != "a\tA" || st != 0 {
		t.Errorf("got %q status %d, want %q and 0", out, st, "a\tA")
	}
}

// A `\c` in a `%b` argument ends the whole `printf` and not only the
// conversion that read it: the rest of the format is abandoned, the operands
// after it go unused, and the format does not run again. Unanimous, and
// invisible to a case that reads one conversion.
func TestPrintfBackslashCInABArgumentEndsTheWholePrintf(t *testing.T) {
	sem := printfSem()
	for _, tc := range []struct{ name, src, want string }{
		{"the rest of the format", `printf '[%b][%s]' 'a\cb' x`, "[a"},
		{"the format is not reused", `printf '[%b]' 'a\cb' yy`, "[a"},
		{"a literal after the conversion", `printf '%b\n' 'a\cb'`, "a"},
		// The field is PrintfBStopIsPadded's question and printfSem answers
		// it Yes, which is five of the six; ksh93's No is
		// TestPrintfBStopIsPaddedOrNot's.
		{"width still applies to what was produced", `printf '[%5b]' 'a\cb'`, "[    a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// What a `\c` leaves of a `%b` still goes through the conversion's field in
// five of the six — the width, the precision and the left-justifying flag —
// where ksh93 alone writes the partial text as it stands (#910).
//
// It is a property of the *stop*: with nothing stopping it every shell pads
// and truncates, which the last two rows pin so the axis cannot be read as
// "this shell has no field for `%b`".
func TestPrintfBStopIsPaddedOrNot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		padded Answer
		src    string
		want   string
	}{
		{"a width, padded", Yes, `printf '[%5b]' 'a\cb'`, "[    a"},
		{"a width, not padded", No, `printf '[%5b]' 'a\cb'`, "[a"},
		{"left-justified, padded", Yes, `printf '[%-5b]' 'a\cb'`, "[a    "},
		{"left-justified, not padded", No, `printf '[%-5b]' 'a\cb'`, "[a"},
		{"a precision, applied", Yes, `printf '[%.1b]' 'ab\cc'`, "[a"},
		{"a precision, not applied", No, `printf '[%.1b]' 'ab\cc'`, "[ab"},
		{"both, applied", Yes, `printf '[%5.1b]' 'ab\cc'`, "[    a"},
		{"both, not applied", No, `printf '[%5.1b]' 'ab\cc'`, "[ab"},

		// Nothing stopped these, so the field applies either way and the
		// axis is not consulted at all.
		{"a width with no stop, under yes", Yes, `printf '[%5b]' 'ab'`, "[   ab]"},
		{"a width with no stop, under no", No, `printf '[%5b]' 'ab'`, "[   ab]"},
		{"a precision with no stop, under no", No, `printf '[%.1b]' 'abc'`, "[a]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBStopIsPadded = tc.padded
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The axis is asked only where a `\c` stopped a `%b` *and* the field would
// change the text, so an ordinary stop with no field needs no dialect.
func TestPrintfBStopPaddingIsAskedOnlyWhenItMatters(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bare %b that stops", `printf '[%b]' 'a\cb'`, "[a"},
		{"a width the text already fills", `printf '[%1b]' 'a\cb'`, "[a"},
		{"a precision the text is already under", `printf '[%.5b]' 'a\cb'`, "[a"},
		{"a %b with no stop at all", `printf '[%5b]' 'ab'`, "[   ab]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfBStopIsPadded = Unspecified
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0 with nothing asked", out, st, tc.want)
			}
		})
	}
}

// And it is refused by name where it does matter and no dialect answered.
func TestPrintfBStopPaddingUnansweredIsRefused(t *testing.T) {
	sem := printfSem()
	sem.PrintfBStopIsPadded = Unspecified
	out, st := run(t, `printf '[%5b]' 'a\cb'`, func(r *Runner) { r.Semantics = &sem })
	want := "sh: a `%b` a `\\c` cut short still going through its field: " +
		"the shells disagree here and no dialect was chosen\n[a"
	if out != want || st != 2 {
		t.Errorf("got %q status %d, want %q and 2", out, st, want)
	}
}

// The `*` a width or a precision may be written as takes its value from the
// operand list, ahead of the operand being converted. Unanimous across the
// whole panel — bash 5.3, bash as sh, bash 3.2, zsh, ksh93, dash and BusyBox
// ash — and so a correction rather than an axis (#2646).
func TestPrintfStarTakesTheWidthFromTheOperands(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a width", `printf '[%*d]' 6 42`, "[    42]"},
		{"a precision", `printf '[%.*s]' 3 hello`, "[hel]"},
		// The discriminating one: two stars in a single conversion, taken
		// as two operands in written order. A fix that resolved one star
		// passes both rows above and fails this.
		{"both, in order", `printf '[%*.*f]' 10 2 3.14159`, "[      3.14]"},
		// Ordering again, from the other side: were the two read the other
		// way round, this would be `[     3.141590]`.
		{"the order is not a coincidence", `printf '[%*.*f]' 2 10 3.14159`, "[3.1415900000]"},
		{"a negative width is the `-` flag", `printf '[%*d]' -5 42`, "[42   ]"},
		{"a negative precision is no precision", `printf '[%.*s]' -3 hello`, "[hello]"},
		{"a zero width is no width", `printf '[%*d]' 0 42`, "[42]"},
		{"a zero precision is not no precision", `printf '[%.*s]' 0 hello`, "[]"},
		{"the flags still stand", `printf '[%-*d]' 6 42`, "[42    ]"},
		{"including the one a digit would have been", `printf '[%0*d]' 6 42`, "[000042]"},
		{"a `-` flag and a negative width agree", `printf '[%-*d]' -6 42`, "[42    ]"},
		{"a star for a string's width", `printf '[%*s]' 6 hi`, "[    hi]"},
		// The conversions that do not read their operand as a number take
		// the same widths: the star belongs to the field, not to the verb.
		{"a star before a `%c`", `printf '[%*c]' 4 abc`, "[   a]"},
		{"a star before a `%b`", `printf '[%*b]' 5 'a\tb'`, "[  a\tb]"},
		{"a precision truncates a `%b` after its escapes", `printf '[%.*b]' 2 'a\tb'`, "[a\t]"},
		{"a width with no star is untouched", `printf '[%6d]' 42`, "[    42]"},
		{"a precision with no star is untouched", `printf '[%.3s]' hello`, "[hel]"},
		{"a star with nothing left is a zero", `printf '[%*d]' 6`, "[     0]"},
		{"and so is a precision's", `printf '[%.*s]' `, "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The format is reused until the operands run out, and a star consumes one of
// them. Both halves of that have to hold at once: a star that consumed
// nothing would spin on a format with no other conversion in it, and one
// whose count the loop did not see would be off by a pass.
//
// Written so that a wrong answer hangs or over-runs rather than mismatching,
// and run under a deadline so the hang is a failure and not a stuck suite.
func TestPrintfStarIsCountedByTheReuseLoop(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a star and its value, twice", `printf '[%*d]' 6 42 3 7`, "[    42][  7]"},
		{"an odd operand starts a pass the star pays for", `printf '[%*d]' 6 42 8`, "[    42][       0]"},
		{"two stars a pass", `printf '[%*.*f]' 10 2 3.14159 6 1 2.5`, "[      3.14][   2.5]"},
		// The format whose only conversion is a width. Were the star not
		// counted as an operand consumed, `used` would be zero every pass
		// and the loop would never reach its operands.
		{"a format that is nothing but a width", `printf '[%*s]' 3 1 4 1 5`, "[  1][   1][     ]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			done := make(chan struct{})
			var out string
			var st int
			go func() {
				defer close(done)
				sem := printfSem()
				out, st = run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("printf did not finish: the reuse loop is not counting the star's operand")
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// A star's operand is read by the same number reader the conversion's own
// operand is, so the complaint and the axis that governs it are the same
// ones — except for the status, which ash alone withholds.
func TestPrintfStarOperandIsReadAsANumber(t *testing.T) {
	for _, tc := range []struct {
		name   string
		costs  Answer
		src    string
		want   string
		status int
	}{
		{"a character constant is a width", Yes, `printf '[%*d]' "'A" 42`, "[" + strings.Repeat(" ", 63) + "42]", 0},
		{"something that is not a number complains", Yes, `printf '[%*s]' abc hi`, "sh: printf: abc: invalid number\n[hi]", 1},
		{"and the width it could not read is none", No, `printf '[%*s]' abc hi`, "sh: printf: abc: invalid number\n[hi]", 0},
		{"a precision's operand is read the same way", Yes, `printf '[%.*s]' abc hello`, "sh: printf: abc: invalid number\n[]", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfReportsBadNumber = Yes
			sem.PrintfStarComplaintCostsTheStatus = tc.costs
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// An absent operand at a star and an absent operand at the conversion are not
// one question, and ash is the column that proves it: the same shell that
// reads a missing `%d` operand as an empty string reads a missing width as a
// silent zero. `printf 'a%*db'` writes one complaint there and not two.
func TestPrintfStarWithNoOperandIsNotTheAbsentNumberCase(t *testing.T) {
	sem := printfSem()
	sem.PrintfReportsBadNumber = Yes
	sem.PrintfEmptyIsNotANumber = Yes
	sem.PrintfAbsentNumberIsAnEmptyOne = Yes

	out, st := run(t, `printf 'a%*db'`, func(r *Runner) { r.Semantics = &sem })
	if want := "sh: printf: : invalid number\na0b"; out != want || st != 1 {
		t.Errorf("got %q status %d, want %q and 1", out, st, want)
	}
	out, st = run(t, `printf '[%.*s]'`, func(r *Runner) { r.Semantics = &sem })
	if out != "[]" || st != 0 {
		t.Errorf("got %q status %d, want the empty field in silence at 0", out, st)
	}
}

// ksh93 alone refuses the directive outright when a star finds the operand
// list already empty. The trigger is the star's operand and not the operand
// count — a star that has its width is fine however little is left after it.
//
// The refusal takes the pass back with it, which is one answer and not two:
// the one column that refuses is also the one that rewinds, so nothing
// measured parts "refuses" from "refuses and drops what it had written"
// (#2664). The pass goes back to the *start* of the last conversion that
// completed, which is why `%s[%*d]` with `x` keeps none of `x[` — the `%s`
// completed, so the mark is where it began.
func TestPrintfStarWithoutOperandIsRefusedIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refused Answer
		src     string
		want    string
		status  int
	}{
		{"a silent zero", No, `printf '%s[%*d]' x`, "x[0]", 0},
		{"refused", Yes, `printf '%s[%*d]' x`, "sh: printf: .: invalid directive\n", 1},
		// The rewind is to the start of the last *completed* conversion and
		// not to the start of the pass: the second `%s` is the mark here, so
		// what the first one wrote stays.
		{"back to the last completed conversion", Yes, `printf 'AB%sCD%sEF%*dG' q r`, "sh: printf: .: invalid directive\nABqCD", 1},
		// Reaching the operand list is the test and not finding anything in
		// it. The `%s` read a missing operand and still moved the mark.
		{"a conversion that read a missing operand still marks", Yes, `printf 'AB%sCD%*dEF'`, "sh: printf: .: invalid directive\nAB", 1},
		// A `%%` is not a conversion that reads anything, so it moves
		// nothing and the whole pass goes.
		{"a percent moves no mark", Yes, `printf 'AB%%CD%*dEF'`, "sh: printf: .: invalid directive\n", 1},
		// Per pass and not per builtin: the first pass is written in full
		// and the second is taken back to nothing.
		{"the rewind is one pass's", Yes, `printf '%s[%*d];' a 3 7 d`, "a[  7];sh: printf: .: invalid directive\n", 1},
		// An ordinary refusal does not rewind — this is the star's own
		// behavior rather than a discard on any error.
		{"another refusal keeps the output", Yes, `printf 'X%*dY%vZ' 3 7 q`, "sh: printf: %v: invalid directive\nX  7Y", 1},
		// The star has its operand here; what ran out is the `%d`, which is
		// the other question and not this one.
		{"a star that has its width is not refused", Yes, `printf '[%*d]' 6`, "[     0]", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfStarWithoutOperandIsRefused = tc.refused
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The `'` flag is a flag in three of the panel and not a flag at all in the
// other two, where the character reaches the scan as the conversion it does
// not have — the same shape #2646 had, and the reason the two answers want
// different things from the prefix scan rather than from the formatter.
//
// The grouping itself is not in this table because there is none to see: the
// separator is the locale's, this shell has numeric data for the C locale
// alone, and `THOUSEP` there is empty. That is asserted rather than assumed by
// TestPrintfDropsTheGroupingFlagOnlyBecauseTheSeparatorIsEmpty below. See
// Semantics.PrintfGroupingFlag and #2675.
func TestPrintfGroupingFlagIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flag   Answer
		src    string
		want   string
		status int
	}{
		{"the flag is taken", Yes, `printf "[%'d]" 1234567`, "[1234567]", 0},
		{"and the rest of the format runs", Yes, `printf "a%'db%sc" 1234567 X`, "a1234567bXc", 0},
		// Not a flag: the `'` is the conversion character, which is refused
		// as any unknown one is and stops the format where it stands. The
		// `b` and everything after it never reach the output.
		{"not a flag, so it is the conversion", No, `printf "a%'db%sc" 1234567 X`, "sh: printf: %': invalid directive\na", 1},
		{"a flag run it is written into", Yes, `printf "[%-'10d]" 42`, "[42        ]", 0},
		{"and the same one refused", No, `printf "[%-'10d]" 42`, "sh: printf: %-': invalid directive\n[", 1},
		// The conversions that do not group take the flag all the same: it
		// is a property of the prefix and not of the verb, and no column
		// refuses it on one verb and not another.
		{"a string conversion takes it", Yes, `printf "[%'s]" abc`, "[abc]", 0},
		{"a float conversion takes it", Yes, `printf "[%'f]" 2.5`, "[2.500000]", 0},
		// The control: outside a conversion's prefix the character is
		// ordinary in all seven columns, so neither answer may touch it.
		{"past the verb it is a literal", Yes, `printf "[%d'x]" 7`, "[7'x]", 0},
		{"past the verb it is a literal here too", No, `printf "[%d'x]" 7`, "[7'x]", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfGroupingFlag = tc.flag
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// The `'` flag is dropped, and this is the assertion that says *why*.
//
// `printf` writes a conversion carrying the flag ungrouped, which is right
// only for as long as the separator it would group by is empty. That
// separator lives in one place — interp.LocaleNumericFor, which
// `$langinfo[THOUSEP]` reads too — and nothing in printfbuiltin.go consults
// it, because under every locale this shell has numeric data for the grouped
// string and the plain one are the same bytes.
//
// So the two halves are joined here instead. Give the C locale a separator
// and this test stops at its first assertion, naming `printf` as the thing
// that now has grouping to do; leave both alone and it keeps passing.
// Without it the reason for the drop is a comment, and a comment does not
// fail (#2675).
//
// The locale in force is not a variable of this test. LocaleNumericFor
// answers for the C locale and refuses every other, so the shell's output is
// the same under all of them — which is the third row, and the reason the
// separator and not the locale is the thing pinned.
func TestPrintfDropsTheGroupingFlagOnlyBecauseTheSeparatorIsEmpty(t *testing.T) {
	numeric, ok := LocaleNumericFor("C")
	if !ok {
		t.Fatal("LocaleNumericFor has no data for the C locale, which is the one locale it is for")
	}
	if numeric.ThousandsSeparator != "" {
		t.Fatalf("the C locale now groups by %q — printf must group with it rather than dropping the `'` flag; see interp/printfbuiltin.go and #2675",
			numeric.ThousandsSeparator)
	}
	for _, tc := range []struct{ name, src string }{
		{"the C locale", `printf "[%'d]" 1234567`},
		{"a locale that was never set", `unset LC_ALL LC_NUMERIC LANG; printf "[%'d]" 1234567`},
		{"a locale this shell has no numeric data for", `LC_ALL=en_US.UTF-8; printf "[%'d]" 1234567`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfGroupingFlag = Yes
			out, st := run(t, "LC_ALL=C; "+tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != "[1234567]" || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, "[1234567]")
			}
		})
	}
}

// Where the flag may be written, which is ksh93's question alone: it reads a
// `'` anywhere in the conversion prefix and the other two that have the flag
// read it among the flags and nowhere else.
//
// The third row is what makes this about the *position*. `%'15d` is the same
// flag ahead of the width and both answers take it, so a table without that
// row would pass for an implementation that had simply stopped accepting the
// flag at all.
func TestPrintfGroupingFlagAfterTheWidthIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		after  Answer
		src    string
		want   string
		status int
	}{
		{"past the width", Yes, `printf "[%10'd]" 1234567`, "[   1234567]", 0},
		{"past the width, refused", No, `printf "[%10'd]" 1234567`, "sh: printf: %10': invalid directive\n[", 1},
		{"past the precision", Yes, `printf "[%.5'd]" 1234567`, "[1234567]", 0},
		{"past the precision, refused", No, `printf "[%.5'd]" 1234567`, "sh: printf: %.5': invalid directive\n[", 1},
		{"ahead of the width is neither answer's business", Yes, `printf "[%'10d]" 1234567`, "[   1234567]", 0},
		{"ahead of the width, still not", No, `printf "[%'10d]" 1234567`, "[   1234567]", 0},
		// *Inside* a digit run, which is the position #2688 found and the
		// one that is not "the flag written one place further along": the
		// quote ends the run it is in, and before the `.` the scan starts
		// over at the flags — so `%1'0d` is width 1 and a zero-padding flag
		// rather than width ten.
		{"inside the width", Yes, `printf "[%1'0d]" 42`, "[42]", 0},
		{"inside the width, refused", No, `printf "[%1'0d]" 42`, "sh: printf: %1': invalid directive\n[", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfGroupingFlagAfterTheWidth = tc.after
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// A `*` beside a width's own digits, which C's grammar has no room for and
// one column reads as a field the star wins (#2824).
//
// The `%*0d` row is the one that separates this reading from "the scan starts
// over": the `0` after the star is a digit that is ignored, not the
// zero-padding flag, so a restart would answer `[0042]` here.
//
// The `%*8*d` row is the lost-star accounting. Both operands are read, in the
// order they were written, and the *last* star is the one the width comes
// from — an implementation that dropped the losing star would take 4 for the
// width and then hand 6 to the conversion.
func TestPrintfStarBesideTheFieldDigitsIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		beside Answer
		src    string
		want   string
		status int
	}{
		{"a star after the digits", Yes, `printf "[%5*d]" 4 42`, "[  42]", 0},
		{"a star after the digits, refused", No, `printf "[%5*d]" 4 42`, "sh: printf: %5*: invalid directive\n[", 1},
		{"a star before the digits", Yes, `printf "[%*5d]" 4 42`, "[  42]", 0},
		{"a star before the digits, refused", No, `printf "[%*5d]" 4 42`, "sh: printf: %*5: invalid directive\n[", 1},
		{"digits on both sides change nothing", Yes, `printf "[%8*9d]" 4 42`, "[  42]", 0},
		{"the flags in front still apply", Yes, `printf "[%-5*d][%05*d]" 4 42 4 42`, "[42  ][0042]", 0},
		{"a zero after the star is a digit", Yes, `printf "[%*0d]" 4 42`, "[  42]", 0},
		{"two stars, both read, the last winning", Yes, `printf "[%*8*d]" 4 6 42`, "[    42]", 0},
		{"a precision reads the same way", Yes, `printf "[%.2*d]" 4 42`, "[0042]", 0},
		{"both fields at once", Yes, `printf "[%5*.3*d]" 4 2 9`, "[  09]", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfStarBesideTheFieldDigits = tc.beside
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// And it is asked at that disagreement and nowhere else. Run *unanswered*, so
// a conversion that consults it refuses the command and names it, where one
// that does not runs as it always did.
func TestPrintfStarBesideTheDigitsIsAskedOnlyAtItsOwnDisagreement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		refused bool
	}{
		{"a star beside digits", `printf "[%5*d]" 4 42`, true},
		{"the same the other way round", `printf "[%*5d]" 4 42`, true},
		{"a star on its own settles nothing", `printf "[%*d]" 4 42`, false},
		{"nor two of them", `printf "[%*.*d]" 4 2 42`, false},
		{"nor a width with no star", `printf "[%5d]" 42`, false},
		{"nor a bare conversion", `printf "[%d]" 42`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfStarBesideTheFieldDigits = Unspecified
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if got := strings.Contains(out, "disagree"); got != tc.refused {
				t.Errorf("got %q, want the axis consulted = %v", out, tc.refused)
			}
		})
	}
}

// The second axis is asked at its own disagreement and nowhere else, and both
// halves of that are here. Run with it *unanswered*, so an implementation
// that consults it refuses the command and names it on stderr rather than
// passing quietly wherever a preset happens to agree.
//
// The first half: a dialect that has no `'` flag is never questioned about
// where the flag may be written, so `%10'd` there is dash's ordinary refusal
// of a conversion it does not have.
//
// The second: a conversion with the flag written *among* its flags settles
// nothing about the late position either, so `%'d` and `%'10d` must run under
// a shell that has the flag and has not answered where it goes. That row is
// the one a "consult it whenever the flag exists" implementation fails, and
// it is the difference between asking at the disagreement and asking at the
// feature.
func TestPrintfGroupingPositionIsAskedOnlyAtItsOwnDisagreement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flag    Answer
		src     string
		refused bool
	}{
		{"no flag at all, and the late position is not the reason", No, `printf "[%10'd]" 1234567`, true},
		{"no flag at all, plainly written", No, `printf "[%'d]" 1234567`, true},
		{"the flag among the flags asks nothing about the late position", Yes, `printf "[%'d]" 1234567`, false},
		{"nor does the flag ahead of a width", Yes, `printf "[%'10d]" 1234567`, false},
		{"nor a conversion with no quote in it", Yes, `printf "[%10d]" 1234567`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfGroupingFlag = tc.flag
			sem.PrintfGroupingFlagAfterTheWidth = Unspecified
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if strings.Contains(out, "disagree") {
				t.Fatalf("got %q — the position axis was consulted where nothing disagrees", out)
			}
			switch {
			case tc.refused && (!strings.Contains(out, "invalid directive") || st != 1):
				t.Errorf("got %q status %d, want the bad-directive refusal at 1", out, st)
			case !tc.refused && (!strings.Contains(out, "1234567") || st != 0):
				t.Errorf("got %q status %d, want the number at 0", out, st)
			}
		})
	}
}

// The three axes this file added are asked at the disagreement and nowhere
// else, and the strongest way to say so is to run the core vector — where
// every one of them is unanswered, and an axis that *is* consulted refuses
// the command and says which one on stderr.
//
// So an ordinary `printf` has to work under a shell that has answered none of
// them, and a star that is given its operands is ordinary: the panel is
// unanimous about it, which is what made #2646 a correction.
func TestPrintfDoesNotConsultTheStarAxesOnThePlainPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a number", `printf '%d' 5`, "5"},
		{"a string and a number", `printf '[%s=%d]' n 5`, "[n=5]"},
		{"a width written out", `printf '[%6d]' 42`, "[    42]"},
		{"a width from the operands", `printf '[%*d]' 6 42`, "[    42]"},
		{"a precision from the operands", `printf '[%.*s]' 3 hello`, "[hel]"},
		{"both of them", `printf '[%*.*f]' 10 2 3.14159`, "[      3.14]"},
		{"a negative width", `printf '[%*d]' -5 42`, "[42   ]"},
		{"the format reused", `printf '[%*d]' 6 42 3 7`, "[    42][  7]"},
		// The absent *conversion* operand is the panel's silent zero too,
		// and the guard in printfEmptyNumberIsAnError is what keeps the core
		// from being asked about it.
		{"an absent operand", `printf '[%d]'`, "[0]"},
		{"an absent operand part way through", `printf '[%d][%d]' 1`, "[1][0]"},
		// And the two the `'` flag added (#2665). A conversion with no `'`
		// in its prefix must not raise either question, and a `'` that is
		// past the verb is not in a prefix at all.
		{"a quote outside a conversion", `printf "[%d'x]" 7`, "[7'x]"},
		{"a quote in the format's text", `printf "it's [%s]" here`, "it's [here]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0 — an axis was consulted on the plain path", out, st, tc.want)
			}
		})
	}
}

// `%g` defaults to six significant digits, which is C's number and not Go's.
//
// Go's `%g` writes the shortest representation that round-trips, so handing
// the verb through with no precision wrote every digit the float had. Run
// under CoreSemantics deliberately: bash 5.3, bash as sh, bash 3.2, ksh93,
// zsh, dash and BusyBox ash all answer the same thing, so this is the core's
// and no axis may be consulted to reach it (#2687).
func TestPrintfGDefaultsToSixSignificantDigits(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"a value with more digits than six", `printf '[%g]' 1234567`, "[1.23457e+06]"},
		{"and one with a great many more", `printf '[%g]' 123456789`, "[1.23457e+08]"},
		{"a small one counts its digits the same way", `printf '[%g]' 0.0001234567`, "[0.000123457]"},
		{"a value whose digits are all significant", `printf '[%g]' 3.14159265358979`, "[3.14159]"},
		// Rounding to six and *then* switching to an exponent, which is the
		// row that needs both halves of `%g` at once: six significant digits
		// carry 999999.5 to 1000000, and the trailing zeros the conversion
		// strips make that 1e+06. A precision default with no stripping
		// writes 1.00000e+06 and a stripping with no default writes 999999.5.
		{"six digits and the stripping together", `printf '[%g]' 999999.5`, "[1e+06]"},
		// The two rows that agree whatever the default is. They are here
		// because a fix that rounded everything to six *decimal places*, or
		// that stopped stripping, passes the rows above and fails these.
		{"a value six digits already reach", `printf '[%g]' 0.1`, "[0.1]"},
		{"the exponent threshold from below", `printf '[%g]' 100000`, "[100000]"},
		{"and from above", `printf '[%g]' 1000000`, "[1e+06]"},
		{"a zero", `printf '[%g]' 0`, "[0]"},
		{"a digit past the sixth that rounds away", `printf '[%g]' 1.000000000000001`, "[1]"},
		{"the small end of the threshold", `printf '[%g]' 0.0001`, "[0.0001]"},
		{"and one step below it", `printf '[%g]' 0.00001`, "[1e-05]"},
		{"`%G` is the same number in capitals", `printf '[%G]' 123456789`, "[1.23457E+08]"},
		// The neighbors, pinned rather than changed: Go's default precision
		// for these three is already six, which is why the gap survived
		// beside verbs that look like it.
		{"`%e` keeps its own six", `printf '[%e]' 123456789`, "[1.234568e+08]"},
		{"`%E` too", `printf '[%E]' 123456789`, "[1.234568E+08]"},
		{"and `%f`, which counts places rather than digits", `printf '[%f]' 123456789`, "[123456789.000000]"},
		// A written precision was never wrong and still is not.
		{"an explicit precision wins", `printf '[%.10g]' 123456789`, "[123456789]"},
		{"a small explicit precision wins too", `printf '[%.3g]' 3.14159265358979`, "[3.14]"},
		{"a precision of zero is taken as one", `printf '[%.0g]' 3.14159265358979`, "[3]"},
		{"and so is a `.` with no digits after it", `printf '[%.g]' 3.14159265358979`, "[3]"},
		{"a precision from the operands wins", `printf '[%.*g]' 3 3.14159265358979`, "[3.14]"},
		// C's rule for a negative `*` precision is that there is none, so
		// this default is what fills in — the same path #2646 resolved.
		{"a negative one is no precision, so the default applies", `printf '[%.*g]' -1 123456789`, "[1.23457e+08]"},
		// The width applies to the *shortened* text, which is how a caller
		// notices the default at all in a field wide enough to hide it.
		{"a width pads what six digits left", `printf '[%12g]' 1234567`, "[ 1.23457e+06]"},
		{"the `-` flag with it", `printf '[%-12g]' 1234567`, "[1.23457e+06 ]"},
		{"the `0` flag with it", `printf '[%012g]' 1234567`, "[01.23457e+06]"},
		{"and a sign flag", `printf '[%+g]' 1234567`, "[+1.23457e+06]"},
		// `#` asks for the trailing zeros back, and asks for six of them.
		{"the `#` flag keeps the zeros the default counted", `printf '[%#g]' 1.5`, "[1.50000]"},
		{"including past the exponent threshold", `printf '[%#g]' 1000000`, "[1.00000e+06]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// An infinity and a not-a-number are written C's way, which is not Go's.
//
// Go's `strconv` spells them `+Inf`, `-Inf` and `NaN`, and the conversion
// handed the float straight to `fmt.Sprintf`, so every float conversion wrote
// Go's spelling at once (#2707). Run under CoreSemantics deliberately: the
// spelling itself is unanimous across bash 5.3, bash as sh, bash 3.2, zsh,
// dash and BusyBox ash, so no axis may be consulted to reach it — and the
// rows here are the ones where the two readings of
// PrintfNonFiniteIsConverted agree, which is what makes them the core's.
func TestPrintfWritesAnInfinityAndANotANumberCsWay(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"an infinity", `printf '[%f]' inf`, "[inf]"},
		{"a negative one keeps its sign", `printf '[%f]' -inf`, "[-inf]"},
		{"a not-a-number", `printf '[%f]' nan`, "[nan]"},
		// The sign of a not-a-number is never written. Six columns answer
		// `nan` for `-nan`, and no reading of the axis writes one.
		{"a negative not-a-number is still unsigned", `printf '[%f]' -nan`, "[nan]"},
		{"`%e` spells it the same way", `printf '[%e]' inf`, "[inf]"},
		{"and `%g`", `printf '[%g]' inf`, "[inf]"},
		{"and `%g` of a not-a-number", `printf '[%g]' nan`, "[nan]"},
		// A precision is meaningless for these and is dropped rather than
		// truncating the word, which a naive `%s` of it would do.
		{"a precision does not truncate it", `printf '[%.2f]' inf`, "[inf]"},
		{"nor a precision of zero", `printf '[%.0f]' inf`, "[inf]"},
		{"nor one on a not-a-number", `printf '[%.1e]' nan`, "[nan]"},
		// The operand spellings, which are the reader's half of the same
		// bug and need the same six columns to settle.
		{"capitals are read", `printf '[%f]' INF`, "[inf]"},
		{"mixed case too", `printf '[%f]' NaN`, "[nan]"},
		{"the long spelling", `printf '[%f]' infinity`, "[inf]"},
		{"the long spelling in capitals", `printf '[%f]' INFINITY`, "[inf]"},
		{"a sign before an infinity", `printf '[%f]' +inf`, "[inf]"},
		{"and a sign before a not-a-number, which Go refuses", `printf '[%f]' +nan`, "[nan]"},
		{"the parenthesized form, which Go refuses too", `printf '[%f]' 'nan(1)'`, "[nan]"},
		{"with letters in it", `printf '[%f]' 'nan(abc)'`, "[nan]"},
		{"with nothing in it", `printf '[%f]' 'nan()'`, "[nan]"},
		{"and with a sign as well", `printf '[%f]' '-nan(_1)'`, "[nan]"},
		// What is between the parentheses is not read. Five columns take
		// these three and BusyBox ash refuses all three, which is BSD's
		// strtod against musl's rather than a language — see cNotANumber.
		{"characters C leaves to the implementation", `printf '[%f]' 'nan(a-b)'`, "[nan]"},
		{"including a space", `printf '[%f]' 'nan(a b)'`, "[nan]"},
		{"and a glob character", `printf '[%f]' 'nan(*)'`, "[nan]"},
		// The controls. A finite float must not go near any of this, and
		// the words are only numbers where a *number* was asked for.
		{"a finite value is untouched", `printf '[%f]' 1.5`, "[1.500000]"},
		{"a negative zero keeps its sign", `printf '[%f]' -0.0`, "[-0.000000]"},
		{"a very large finite value", `printf '[%g]' 1e300`, "[1e+300]"},
		{"`%s` of the word is the word", `printf '[%s]' inf`, "[inf]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// Where the panel splits: whether the word goes through the conversion.
//
// Yes is bash, dash and BusyBox ash — C's own reading, because it is C's
// library — and No is zsh, which writes the word as it stands. The two
// consequences are tested together because they are one rule: an upper-case
// conversion capitalizes it, and a width pads it.
func TestPrintfNonFiniteAxis(t *testing.T) {
	for _, tc := range []struct {
		name      string
		src       string
		converted string
		bare      string
	}{
		{"`%E` capitalizes it", `printf '[%E]' inf`, "[INF]", "[inf]"},
		{"`%G` too", `printf '[%G]' inf`, "[INF]", "[inf]"},
		{"and a not-a-number", `printf '[%G]' nan`, "[NAN]", "[nan]"},
		{"a negative infinity keeps its sign under either", `printf '[%E]' -inf`, "[-INF]", "[-inf]"},
		{"a width pads it", `printf '[%10f]' inf`, "[       inf]", "[inf]"},
		{"the `-` flag pads on the right", `printf '[%-10f]' inf`, "[inf       ]", "[inf]"},
		// The `0` flag is ignored by the converted reading too: what pads is
		// spaces, because the field is C's `%s` field and not the float's.
		{"the `0` flag pads with spaces all the same", `printf '[%010f]' inf`, "[       inf]", "[inf]"},
		{"a width with a precision that means nothing", `printf '[%20.3e]' nan`, "[                 nan]", "[nan]"},
		{"the `+` flag signs an infinity", `printf '[%+f]' inf`, "[+inf]", "[inf]"},
		{"and so does the space flag", `printf '[% f]' inf`, "[ inf]", "[inf]"},
		{"a width and a sign together", `printf '[%+10g]' inf`, "[      +inf]", "[inf]"},
		{"a negative width from an operand", `printf '[%*f]' -10 inf`, "[inf       ]", "[inf]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, reading := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.converted}, {No, tc.bare}} {
				sem := printfSem()
				sem.PrintfNonFiniteIsConverted = reading.answer
				out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
				if out != reading.want || st != 0 {
					t.Errorf("%v: got %q status %d, want %q and 0", reading.answer, out, st, reading.want)
				}
			}
		})
	}
}

// The axis is asked at the disagreement and nowhere else.
//
// Both halves are asserted, because either one alone is satisfiable by a
// mistake: a shell that never asks writes zsh's answer for everyone, and one
// that always asks refuses `printf '%f' inf` under a core vector. So the
// rows the two readings agree on must reach an unanswered core, and the rows
// they differ on must refuse it and say so.
func TestPrintfAsksTheNonFiniteAxisOnlyWhereTheReadingsDiffer(t *testing.T) {
	for _, tc := range []struct {
		name  string
		src   string
		asked bool
	}{
		{"a bare conversion", `printf '[%f]' inf`, false},
		{"a not-a-number", `printf '[%f]' nan`, false},
		{"a negative infinity", `printf '[%f]' -inf`, false},
		{"a lower-case `%e`", `printf '[%e]' inf`, false},
		// A width no wider than the word pads nothing, so the two readings
		// still write the same three bytes.
		{"a width narrower than the word", `printf '[%2f]' inf`, false},
		{"a width exactly the word's length", `printf '[%3f]' inf`, false},
		// The `+` flag on a not-a-number is not a disagreement either: no
		// reading writes a sign for one.
		{"the `+` flag on a not-a-number", `printf '[%+f]' nan`, false},
		{"the space flag on one", `printf '[% f]' nan`, false},
		// And the whole of the finite path, which never reaches the question.
		{"a finite value with a width", `printf '[%10f]' 1.5`, false},
		{"a finite value under `%G`", `printf '[%G]' 1.5`, false},
		{"an upper-case conversion", `printf '[%E]' inf`, true},
		{"`%G`", `printf '[%G]' nan`, true},
		{"a width wide enough to pad", `printf '[%4f]' inf`, true},
		{"the `+` flag on an infinity", `printf '[%+f]' inf`, true},
		{"the space flag on one", `printf '[% f]' inf`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			asked := strings.Contains(out, "disagree")
			switch {
			case asked != tc.asked:
				t.Errorf("got %q status %d, want the axis consulted = %v", out, st, tc.asked)
			case asked && st == 0:
				t.Errorf("got %q status %d, want a refusal to cost the status", out, st)
			case !asked && st != 0:
				t.Errorf("got %q status %d, want 0", out, st)
			}
		})
	}
}

// A field wider than Go's `fmt` renders is laid out by this shell instead.
//
// Past `printfFmtFieldCeiling` the package writes its own error text into the
// output — `%!(NOVERB)%!(EXTRA int64=1)` — which is a message for a Go
// programmer arriving in a shell script's stdout. Measured by bisection
// 2026-09-14: 10000009 renders and 10000010 does not (#2663).
//
// The widths here are one past the ceiling rather than the issue's hundred
// million: the shape is the same and the test writes ten megabytes instead of
// a hundred.
func TestAFieldWiderThanFmtRendersIsLaidOutHere(t *testing.T) {
	// One past the ceiling the in-package TestAZeroInTheWidthIsNotTheZeroFlag
	// names; spelled out here because this file is the external test package.
	const over = 10000010
	for _, tc := range []struct {
		name        string
		src         string
		first, last byte
		wantLen     int
	}{
		// The plain case, and the one the issue reported.
		{"a literal width", `printf '%10000010d' 1`, ' ', '1', over},
		// The same field reached through a `*` operand, which is the route
		// that makes the width *data* — a script can hold one in a variable.
		{"a width from a star", `printf '%*d' 10000010 1`, ' ', '1', over},
		// Left-aligned: the digit leads and the padding follows.
		{"left-aligned", `printf '%-10000010d' 1`, '1', ' ', over},
		// Zero-padded, with the sign kept in front of the zeros it earned.
		{"zero-padded and signed", `printf '%010000010d' -42`, '-', '2', over},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src+"\n", nil)
			if len(out) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(out), tc.wantLen)
			}
			if out[0] != tc.first || out[len(out)-1] != tc.last {
				t.Errorf("ends are %q…%q, want %q…%q",
					out[0], out[len(out)-1], tc.first, tc.last)
			}
			if strings.Contains(out, "NOVERB") {
				t.Error("fmt's own complaint reached the output")
			}
		})
	}
}

// A precision wider than Go's `fmt` renders is honored by this shell instead.
//
// The same ceiling the width has, reached the other way and never covered:
// printfWideField takes a wide width out of the spec and lays it out, and
// there was no equivalent for the precision — so everything between the
// ceiling and the C int printfFieldBeyondAnInt settles fell straight through
// to `fmt` and put `%!(NOVERB)%!(EXTRA string=xyz)` on the shell's stdout at
// status 0 (#3017).
//
// Two halves, because a precision means two different things. A string
// conversion's only truncates, so one past the operand is a no-op and the
// field is the operand. A numeric conversion's is digits that have to be
// produced, and bash really does produce them — every length below is bash
// 5.3.20's, measured byte for byte 2026-09-15.
//
// One past the ceiling rather than the issue's two billion: the shape is the
// same and the test writes ten megabytes instead of two gigabytes.
func TestAPrecisionWiderThanFmtRendersIsHonoredHere(t *testing.T) {
	const over = 10000010
	for _, tc := range []struct {
		name        string
		src         string
		want        string
		wantLen     int
		first, last byte
	}{
		// The truncating conversions, where the answer is the whole operand.
		{name: "a string precision", src: `printf '[%.10000010s]' xyz`, want: "[xyz]"},
		{name: "reached through a star", src: `printf '[%.*s]' 10000010 xyz`, want: "[xyz]"},
		{name: "an escaped one", src: `printf '[%.10000010b]' 'a\tb'`, want: "[a\tb]"},
		{name: "a quoted one", src: `printf '[%.10000010q]' xyz`, want: "[xyz]"},
		// A format with no conversions in it, so the row is the precision
		// and not the machine's timezone.
		{name: "and a date", src: `printf '[%.10000010(epoch)T]' 0`, want: "[epoch]"},
		// The digits really are produced, and these are bash's lengths.
		{
			name: "a decimal precision is minimum digits", src: `printf '%.10000010d' 1`,
			wantLen: over, first: '0', last: '1',
		},
		{
			name: "so is a hexadecimal one", src: `printf '%.10000010x' 1`,
			wantLen: over, first: '0', last: '1',
		},
		{
			name: "a float precision is places after the point", src: `printf '%.10000010f' 1`,
			wantLen: over + 2, first: '1', last: '0',
		},
		{
			name: "and the sign leads them", src: `printf '%.10000010f' -1`,
			wantLen: over + 3, first: '-', last: '0',
		},
		{
			name: "a scientific one keeps its exponent", src: `printf '%.10000010e' 1`,
			wantLen: over + 6, first: '1', last: '0',
		},
		// `%g` counts *significant* digits and strips the trailing zeros,
		// so ten million of them come to one character.
		{name: "a significant-digit precision strips them again", src: `printf '[%.10000010g]' 1`, want: "[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfTimeConversion = Yes
			sem.PrintfTimeOperandIsADateString = No
			out, st := run(t, tc.src+"\n", func(r *Runner) { r.Semantics = &sem })
			if strings.Contains(out, "NOVERB") || strings.Contains(out, "EXTRA") {
				t.Fatalf("fmt's own complaint reached the output: %q", out[:min(len(out), 80)])
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
			if tc.want != "" {
				if out != tc.want {
					t.Errorf("got %q, want %q", out, tc.want)
				}
				return
			}
			if len(out) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(out), tc.wantLen)
			}
			if out[0] != tc.first || out[len(out)-1] != tc.last {
				t.Errorf("ends are %q…%q, want %q…%q",
					out[0], out[len(out)-1], tc.first, tc.last)
			}
		})
	}
}

// C99's three float conversions, which two of the panel's columns have not.
//
// Gated rather than always on: a dialect without them wants the letter to
// arrive at the scan as the conversion character it is and be refused the way
// any unknown one is — which is the shape #2646 had, where the diagnostics
// looked right and the answer was wrong.
func TestPrintfC99FloatConversionsAreAnAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"%F is %f in capitals", `printf '[%F]' 1.5`, "[1.500000]"},
		{"and capitalizes an infinity", `printf '[%F]' inf`, "[INF]"},
		{"where %f does not", `printf '[%f]' inf`, "[inf]"},
		{"and a not-a-number", `printf '[%F]' nan`, "[NAN]"},
		{"%a is C's hexadecimal float", `printf '[%a]' 1.5`, "[0x1.8p+0]"},
		{"%A is the same in capitals", `printf '[%A]' 1.5`, "[0X1.8P+0]"},
		{"and capitalizes an infinity too", `printf '[%A]' inf`, "[INF]"},
		{"but %a does not", `printf '[%a]' inf`, "[inf]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfC99FloatConversions = Yes
			sem.PrintfHexFloatDefaultIsTwelveDigits = No
			sem.PrintfNonFiniteIsConverted = Yes
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}

	t.Run("a dialect without them refuses the letter", func(t *testing.T) {
		for _, src := range []string{`printf '[%F]' 1.5`, `printf '[%a]' 1.5`, `printf '[%A]' 1.5`} {
			sem := printfSem()
			sem.PrintfC99FloatConversions = No
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if !strings.Contains(out, "invalid directive") || st == 0 {
				t.Errorf("%s: got %q status %d, want the refusal a conversion nobody has earns", src, out, st)
			}
		}
	})
}

// The hexadecimal float's layout, which is C's and not Go's `%x`.
//
// Every row is measured against bash 5.3.15 and dash, which agree on all of
// them; see printfHexFloat for the table. The rows that round are the ones a
// renderer built on strconv would have got wrong while looking right
// everywhere else — strconv rounds a tie away from zero and renormalizes when
// the rounding carries, and these two references do neither.
func TestPrintfHexFloatIsLaidOutCsWay(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the exponent is the shortest run of digits", `printf '[%a]' 1.5`, "[0x1.8p+0]"},
		{"a large exponent keeps all of them", `printf '[%a]' 1e300`, "[0x1.7e43c8800759cp+996]"},
		{"a negative exponent keeps its sign", `printf '[%a]' 0.1`, "[0x1.999999999999ap-4]"},
		{"a zero is a zero", `printf '[%a]' 0`, "[0x0p+0]"},
		{"a negative zero keeps the sign the value carries", `printf '[%a]' -0.0`, "[-0x0p+0]"},
		{"a subnormal is normalized", `printf '[%a]' 5e-324`, "[0x1p-1074]"},
		{"a stated precision is digits after the point", `printf '[%.3a]' 1.5`, "[0x1.800p+0]"},
		{"a precision of zero drops the point", `printf '[%.0a]' 1.5`, "[0x1p+0]"},
		{"the # flag puts the point back and adds nothing", `printf '[%#.0a]' 1.5`, "[0x1.p+0]"},
		{"and is invisible where there are digits", `printf '[%#a]' 1.5`, "[0x1.8p+0]"},
		{"a tie rounds toward zero", `printf '[%.1a]' 1.09375`, "[0x1.1p+0]"},
		{"in both directions", `printf '[%.1a]' 1.15625`, "[0x1.2p+0]"},
		{"a carry out of the leading digit keeps the exponent", `printf '[%.1a]' 255`, "[0x2.0p+7]"},
		{"and does so with no fraction at all", `printf '[%.0a]' 0.1`, "[0x2p-4]"},
		{"the + flag signs a positive value", `printf '[%+a]' 1.5`, "[+0x1.8p+0]"},
		{"the space flag does too", `printf '[% a]' 1.5`, "[ 0x1.8p+0]"},
		{"neither reaches a negative one", `printf '[%+a]' -1.5`, "[-0x1.8p+0]"},
		{"a width pads on the left", `printf '[%14a]' 1.5`, "[      0x1.8p+0]"},
		{"the - flag pads on the right", `printf '[%-14a]' 1.5`, "[0x1.8p+0      ]"},
		{"the 0 flag pads after the 0x", `printf '[%014a]' 1.5`, "[0x0000001.8p+0]"},
		{"and after the sign as well", `printf '[%014a]' -1.5`, "[-0x000001.8p+0]"},
		{"a width narrower than the value changes nothing", `printf '[%5a]' 1.5`, "[0x1.8p+0]"},
		{"the capital reaches the prefix and the p", `printf '[%014A]' 1.5`, "[0X0000001.8P+0]"},
		{"a star width is the same question", `printf '[%*a]' 14 1.5`, "[      0x1.8p+0]"},
		{"and a star precision", `printf '[%.*a]' 3 1.5`, "[0x1.800p+0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfC99FloatConversions = Yes
			sem.PrintfHexFloatDefaultIsTwelveDigits = No
			// C's placement for the fill: between the `0x` and the digits.
			// The other reading is PrintfHexFloatZeroFillPrecedesThePrefix
			// and has a case of its own below.
			sem.PrintfHexFloatZeroFillPrecedesThePrefix = No
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The default precision of a `%a` is a second axis, and it is a precision
// rather than a minimum width: the thirteenth digit of `0.1` is rounded away
// under the twelve-digit answer, and `255` is padded out to twelve.
func TestPrintfHexFloatDefaultPrecisionIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		twelve Answer
		src    string
		want   string
	}{
		{"shortest names the value exactly", No, `printf '[%a]' 0.1`, "[0x1.999999999999ap-4]"},
		{"twelve rounds the thirteenth away", Yes, `printf '[%a]' 0.1`, "[0x1.99999999999ap-4]"},
		{"shortest stops where the digits do", No, `printf '[%a]' 255`, "[0x1.fep+7]"},
		{"twelve pads out to twelve", Yes, `printf '[%a]' 255`, "[0x1.fe0000000000p+7]"},
		{"a stated precision is not asked", Yes, `printf '[%.3a]' 0.1`, "[0x1.99ap-4]"},
		{"and neither is one of zero", Yes, `printf '[%.0a]' 1.5`, "[0x1p+0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfC99FloatConversions = Yes
			sem.PrintfHexFloatDefaultIsTwelveDigits = tc.twelve
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The `'` does not merely get skipped in the one dialect that takes it past
// the flags: it **ends the digit run it is in**, and before the `.` the scan
// starts over at the flags. So the digits around it are not the number they
// look like, and a fix that only widened the acceptance would have written a
// silently different width (#2688).
//
// Measured against ksh93u+ under `LC_ALL=C`; every row here is one it wrote.
func TestPrintfGroupingFlagEndsTheFieldRunItIsIn(t *testing.T) {
	sem := printfSem()
	sem.PrintfGroupingFlagAfterTheWidth = Yes
	for _, tc := range []struct{ src, want string }{
		// The row the issue was filed from: width 1, then `0` read as the
		// zero-padding *flag*. Deleting the quote would make it width ten.
		{`printf "[%1'0d]" 42`, "[42]"},
		// Each run after a quote replaces the width rather than extending it.
		{`printf "[%1'2'3d]" 42`, "[ 42]"},
		// An empty run leaves the width already read, and the flag it found
		// applies to that width.
		{`printf "[%5'0d]" 42`, "[00042]"},
		{`printf "[%5''d]" 42`, "[   42]"},
		// The flags accumulate across the restarts.
		{`printf "[%1'-5d]" 42`, "[42   ]"},
		{`printf "[%1'+5d]" 42`, "[  +42]"},
		{`printf "[%1'#5x]" 42`, "[ 0x2a]"},
		{`printf "[%'0'5d]" 42`, "[00042]"},
		// After a `.` the scan does *not* restart at the flags — the digits
		// keep being the precision, and the last run wins there too.
		{`printf "[%.'5d]" 42`, "[00042]"},
		{`printf "[%.5'3d]" 42`, "[042]"},
		{`printf "[%.0'5d]" 42`, "[00042]"},
		{`printf "[%.1'0d]" 42`, "[42]"},
		{`printf "[%1'0.3d]" 42`, "[042]"},
		// A star is a field run like any other, so a later run replaces it —
		// and the operand it took is still gone.
		{`printf "[%5'*d]" 3 42`, "[ 42]"},
		{`printf "[%*'5d]" 3 42`, "[   42]"},
		{`printf "[%*'*d]" 3 4 42`, "[  42]"},
		{`printf "[%*.*'5d]" 3 4 42`, "[00042]"},
		// And the lost operand is read in the place it was written, which is
		// what this row is for: the width's star precedes the precision's.
		{`printf "[%*'5.*d]" 3 2 42`, "[   42]"},
	} {
		out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
		if out != tc.want || st != 0 {
			t.Errorf("%s: got %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// A flag written past a field restarts the scan, the way the `'` already
// does — and the precision is the rule that disagrees with the width's
// (#2910).
func TestPrintfFlagAfterTheFieldIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flag   Answer
		src    string
		want   string
		status int
	}{
		{"a flag past the width is a flag", Yes, `printf "[%5-d]" 42`, "[42   ]", 0},
		{"and refused where the scan ends at it", No, `printf "[%5-d]" 42`, "sh: printf: %5-: invalid directive\n[", 1},
		{"a run after it replaces the width", Yes, `printf "[%5-3d]" 42`, "[42 ]", 0},
		{"the other three flags restart too", Yes, `printf "[%5+d][%5 d][%5#d]" 42 42 42`, "[  +42][   42][   42]", 0},
		{"the flags accumulate across restarts", Yes, `printf "[%5-+d]" 42`, "[+42  ]", 0},
		{"and every run replaces the last", Yes, `printf "[%5-3-4d]" 42`, "[42  ]", 0},
		{"a zero after a width is still a digit", Yes, `printf "[%50d]" 42`, "[" + strings.Repeat(" ", 48) + "42]", 0},
		{"a star the restart replaced still took its operand", Yes, `printf "[%*-5d]" 4 42`, "[42   ]", 0},
		{"and a star after the restart takes one", Yes, `printf "[%5-*d]" 4 42`, "[42  ]", 0},

		{"a minus past a precision throws the precision away", Yes, `printf "[%.5-d]" 42`, "[42]", 0},
		{"and is not itself a flag: this is right-justified", Yes, `printf "[%.3-5d]" 42`, "[   42]", 0},
		{"the width already read survives it", Yes, `printf "[%5.3-d]" 42`, "[   42]", 0},
		{"what follows is read as a width again", Yes, `printf "[%5.3-2d]" 42`, "[42]", 0},
		{"and a second minus there is an ordinary flag", Yes, `printf "[%.3--5d]" 42`, "[42   ]", 0},
		{"a new point opens a new precision", Yes, `printf "[%.3-.4d]" 42`, "[0042]", 0},
		{"a plus past a precision leaves it standing", Yes, `printf "[%.5+d]" 42`, "[+00042]", 0},
		{"and the run after it replaces the precision", Yes, `printf "[%.3+5d]" 42`, "[+00042]", 0},
		{"a blank past a precision reads the same way", Yes, `printf "[%.3 5d]" 42`, "[ 00042]", 0},
		{"the two rules meet", Yes, `printf "[%.3+-5d][%.3-+5d]" 42 42`, "[  +42][  +42]", 0},
		{"a losing precision star is still read", Yes, `printf "[%.*-5d]" 4 42`, "[   42]", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfFlagAfterTheField = tc.flag
			sem.PrintfStarBesideTheFieldDigits = Yes
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != tc.status {
				t.Errorf("got %q status %d, want %q and %d", out, st, tc.want, tc.status)
			}
		})
	}
}

// And it is asked where a flag actually stands past a field and nowhere else.
func TestPrintfFlagAfterTheFieldIsAskedOnlyAtItsOwnDisagreement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		src     string
		refused bool
	}{
		{"a flag past a width", `printf "[%5-d]" 42`, true},
		{"a flag past a precision", `printf "[%.5-d]" 42`, true},
		{"a flag past a star", `printf "[%*-d]" 4 42`, true},
		{"a flag in front settles nothing", `printf "[%-5d]" 42`, false},
		{"nor one in front of a precision", `printf "[%-5.3d]" 42`, false},
		{"nor a width with no flag at all", `printf "[%5d]" 42`, false},
		{"nor a bare conversion", `printf "[%d]" 42`, false},
		{"and a hash past a precision is left alone", `printf "[%.3#d]" 42`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfFlagAfterTheField = Unspecified
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if got := strings.Contains(out, "past a field"); got != tc.refused {
				t.Errorf("got %q, want the axis consulted = %v", out, tc.refused)
			}
		})
	}
}

// The other reading of the same fill: in front of the `0x` rather than
// between it and the digits, at the same total width. Both are measured; see
// Semantics.PrintfHexFloatZeroFillPrecedesThePrefix.
func TestPrintfHexFloatFillMayPrecedeThePrefix(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the fill goes in front", `printf '[%014a]' 1.5`, "[0000000x1.8p+0]"},
		{"and behind the sign, which is still counted", `printf '[%014a]' -1.5`, "[-000000x1.8p+0]"},
		{"the capital is the same question", `printf '[%014A]' 1.5`, "[0000000X1.8P+0]"},
		{"a space fill is unanimous", `printf '[%14a]' 1.5`, "[      0x1.8p+0]"},
		{"and the - flag voids a zero fill", `printf '[%-014a]' 1.5`, "[0x1.8p+0      ]"},
		{"a width narrower than the value asks nothing", `printf '[%5a]' 1.5`, "[0x1.8p+0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfC99FloatConversions = Yes
			sem.PrintfHexFloatDefaultIsTwelveDigits = No
			sem.PrintfHexFloatZeroFillPrecedesThePrefix = Yes
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// Whether the `0` flag survives a precision on an integer conversion, which
// is asked only where there is a fill to place: a width the rendered field
// already fills leaves the two readings identical, and neither is consulted.
func TestPrintfZeroFlagAgainstAPrecision(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		survives  Answer
		want      string
	}{
		{"the flag is ignored, which is C", `printf '[%08.3d]' 7`, No, "[     007]"},
		{"and kept, which is the other reading", `printf '[%08.3d]' 7`, Yes, "[00000007]"},
		{"a sign keeps its place in front of the fill", `printf '[%+05.0d]' 7`, Yes, "[+0007]"},
		{"and in front of the blanks", `printf '[%+05.0d]' 7`, No, "[   +7]"},
		{"an octal's alternate form is inside the fill", `printf '[%#05.0o]' 7`, Yes, "[00007]"},
		{"a precision that produced no digits still has a field", `printf '[%08.0d]' 0`, Yes, "[00000000]"},
		// The arrangements neither reading is asked about, so an unanswered
		// axis is no refusal: no width, no `0` flag, a `-`, and a field the
		// value already fills.
		{"no width", `printf '[%.3d]' 7`, Unspecified, "[007]"},
		{"no zero flag", `printf '[%8.3d]' 7`, Unspecified, "[     007]"},
		{"a - flag", `printf '[%-08.3d]' 7`, Unspecified, "[007     ]"},
		{"a precision wider than the width", `printf '[%08.10d]' 7`, Unspecified, "[0000000007]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfZeroFlagSurvivesAPrecision = tc.survives
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}
