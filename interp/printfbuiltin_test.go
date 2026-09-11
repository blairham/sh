// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// printfSem is a vector with every printf axis answered, so a test that is
// about one of them is not refused for the others.
func printfSem() Semantics {
	s := CoreSemantics()
	s.PrintfReportsBadNumber = No
	s.PrintfEmptyIsNotANumber = No
	s.PrintfBackslashC = PrintfBackslashCLiteral
	s.PrintfQuote = PrintfQuoteBackslash
	s.PrintfLengthModifiers = PrintfLengthModifiersAbsent
	s.PrintfUnfinishedConversionIsAPercent = No
	s.PrintfHexEscape = PrintfHexEscapeAbsent
	s.PrintfBHexEscape = PrintfHexEscapeAbsent
	s.PrintfUnicodeEscape = PrintfUnicodeEscapeAbsent
	s.PrintfBUnicodeEscape = PrintfUnicodeEscapeAbsent
	s.PrintfBEscEscape = No
	s.PrintfBCapitalEscEscape = No
	s.PrintfBOctalWithoutZero = No
	s.PrintfBStopIsPadded = Yes
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
		{"past the last code point there is, nothing", PrintfHexEscapeCodePoint, `printf '[\xffffffffffffffffffffff]'`, "[]"},
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
		{"backslash", PrintfQuoteBackslash, `a\ b`},
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

// An operand that is *missing* is never an error; one that is present and
// empty is, in one dialect. The two are easy to conflate and the corpus has a
// case for each.
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
