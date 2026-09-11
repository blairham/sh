// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `\u` escape naming a code point the locale's encoding cannot hold.
//
// The escape's *reading* is settled and tested next door; this is the question
// after it, and it is two questions rather than one. Whether the locale has
// room is state read off the runner's own variables the way PATH and IFS are —
// `LC_ALL` over `LC_CTYPE` over `LANG`, the reader
// Semantics.MultibyteEncodingIsHonored's already uses. What happens when it
// has none is the dialect's, and the shells that have the escape split on it.
//
// Every case asserts the *bytes*, because the wrong answers are all
// plausible-looking text: the encoded character, the escape written back, and
// nothing at all are three different strings a Contains check would not tell
// apart.

// echoingUnicode is the core with the escape admitted and one answer given to
// what an out-of-range code point does.
func echoingUnicode(p OutsideLocaleEscapePolicy, locale string) func(*Runner) {
	return echoingUnicodeUnset(p, locale, No)
}

// echoingUnicodeUnset is the same with the unset-locale axis answered as well,
// for the rows that leave the locale unnamed. The default above is the answer
// every panel member but one gives, so a row that names a locale never reaches
// it either way.
func echoingUnicodeUnset(p OutsideLocaleEscapePolicy, locale string, unset Answer) func(*Runner) {
	return func(r *Runner) {
		sem := PosixSemantics()
		sem.EchoInterpretsEscapes = Yes
		sem.EchoExpandsUnicodeEscapes = Yes
		sem.UnicodeEscapeOutsideTheLocale = p
		sem.UnsetLocaleIsUnicodeAware = unset
		r.Semantics = &sem
		if locale != "" {
			r.Vars = map[string]string{"LC_ALL": locale}
		}
	}
}

// With the escape standing, the value is written back **normalized** rather
// than echoed: four digits or eight, upper-cased, and the letter chosen by the
// value rather than taken from the input.
func TestAnEscapeOutsideTheLocaleMayStandAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"padded to four digits", `echo 'a\ue9Z'`, `a\u00E9Z` + "\n"},
		{"upper-cased", `echo 'a\u00e9Z'`, `a\u00E9Z` + "\n"},
		{"the letter follows the value", `echo 'a\U000000e9Z'`, `a\u00E9Z` + "\n"},
		{"unchanged when it already has that shape", `echo 'a\u20ACZ'`, `a\u20ACZ` + "\n"},
		{"eight digits once four will not do", `echo 'a\U1F600Z'`, `a\U0001F600Z` + "\n"},
		// The rest of the word is written, which is the half that separates
		// this answer from the refusal below.
		{"two of them, and the text between", `echo 'a\u00e9b\u00e9c'`, `a\u00E9b\u00E9c` + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, echoingUnicode(OutsideLocaleEscapeWritten, "C"))
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q status 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// With the escape refused, what came *before* it is still written — with the
// closing newline `echo` would have added — and nothing after it is.
//
// The complaint is in every want because run() reads both streams into one
// buffer, and it is written *first*, which is the order the two come out in:
// the shell that refuses has read the word before `echo` writes anything.
// It appears once however many escapes the argument holds.
const refusal = "sh: character not in range\n"

func TestAnEscapeOutsideTheLocaleMayBeRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the text before it, and the newline", `echo 'a\u00e9Z'`, refusal + "a\n"},
		{"and no newline where none was asked for", `echo -n 'a\u00e9Z'`, refusal + "a"},
		{"nothing at all when it comes first", `echo '\u00e9Z'`, refusal + "\n"},
		// Two escapes draw one complaint and stop at the first, which is what
		// says the refusal ends the text rather than skipping a character.
		{"the first one ends it", `echo 'a\u00e9b\u00e9c'`, refusal + "a\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, echoingUnicode(OutsideLocaleEscapeRefused, "C"))
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// The refusal abandons the script, and the status it leaves is the one it
// already had rather than a failure.
func TestARefusedEscapeAbandonsTheScriptWithoutFailingIt(t *testing.T) {
	want := refusal + "a\n"
	out, st := run(t, `echo 'a\u00e9Z'; echo AFTER`,
		echoingUnicode(OutsideLocaleEscapeRefused, "C"))
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q status 0 — abandoned, not failed", out, st, want)
	}
}

// Neither answer is reached in a locale that has room for the character, which
// is what keeps the axis at the disagreement: a dialect that has not answered
// it still writes every escape a UTF-8 locale can hold.
func TestAnEscapeInsideTheLocaleNeverAsksTheAxis(t *testing.T) {
	for _, tc := range []struct{ name, locale, src, want string }{
		{"a UTF-8 locale holds it", "en_US.UTF-8", `echo 'a\u00e9Z'`, "a\u00e9Z\n"},
		{"and the codeset is read without case or hyphen", "en_US.utf8", `echo 'a\u00e9Z'`, "a\u00e9Z\n"},
		// ASCII is representable in every encoding, so the boundary is the
		// code point and not the escape: `\u007f` is the DEL byte in the C
		// locale and `\u0080` is the first one outside it.
		{"ASCII is in range even in C", "C", `echo 'a\u0041Z'`, "aAZ\n"},
		{"and so is the last ASCII byte", "C", `echo 'a\u007fZ'`, "a\x7fZ\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Unspecified on purpose: reaching the axis here would refuse the
			// script, so a passing row is evidence the axis was not asked.
			out, st := run(t, tc.src, echoingUnicode(OutsideLocaleEscapeUnspecified, tc.locale))
			if out != tc.want || st != 0 {
				t.Errorf("%s in %q = %q status %d, want %q status 0",
					tc.src, tc.locale, out, st, tc.want)
			}
		})
	}
}

// With nothing naming a locale, whether the character fits is the *other*
// axis's question — and it decides which of the two answers above is reached.
//
// Measured 2026-09-11 under `env -i`: bash 5.3.15 writes `61 c3 a9 5a` for
// `echo -e 'a\u00e9Z'` and zsh 5.9.2 refuses it, the same two answers they
// give to `s=héllo; echo ${#s}` and to uppercasing `café`. So one question
// decides all three (#2020), and this is the escape's half of it.
func TestAnUnsetLocaleDecidesWhetherTheEscapeFits(t *testing.T) {
	t.Run("Unicode-aware, so the character is written and no axis is asked",
		func(t *testing.T) {
			// UnicodeEscapeOutsideTheLocale left unspecified on purpose:
			// reaching it would refuse, so a written character is evidence
			// the locale had room.
			out, st := run(t, `echo 'a\u00e9Z'`,
				echoingUnicodeUnset(OutsideLocaleEscapeUnspecified, "", Yes))
			if out != "a\u00e9Z\n" || st != 0 {
				t.Errorf("got %q status %d, want %q status 0", out, st, "a\u00e9Z\n")
			}
		})
	t.Run("the C locale, so the escape's own axis decides", func(t *testing.T) {
		out, _ := run(t, `echo 'a\u00e9Z'`,
			echoingUnicodeUnset(OutsideLocaleEscapeRefused, "", No))
		if out != refusal+"a\n" {
			t.Errorf("got %q, want %q", out, refusal+"a\n")
		}
		out, st := run(t, `echo 'a\u00e9Z'`,
			echoingUnicodeUnset(OutsideLocaleEscapeWritten, "", No))
		if out != `a\u00E9Z`+"\n" || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, `a\u00E9Z`+"\n")
		}
	})
	t.Run("and an ASCII code point asks neither", func(t *testing.T) {
		out, st := run(t, `echo 'a\u0041Z'`,
			echoingUnicodeUnset(OutsideLocaleEscapeUnspecified, "", Unspecified))
		if out != "aAZ\n" || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, "aAZ\n")
		}
	})
}

// The third answer: the character is written whatever the locale says, which
// is a shell that never consults one.
//
// Measured 2026-09-11 under `LC_ALL=C`, bytes read with `od`: ksh93u+ writes
// `61 c3 a9 5a` for both of the sites it reads the escape at — a `printf`
// format and `$'…'` — where one shell leaves `a\u00e9Z` standing and the other
// refuses. It is the answer this shell gave everywhere before the axis existed
// (#2021).
func TestAnEscapeOutsideTheLocaleMayBeEncodedAnyway(t *testing.T) {
	out, st := run(t, `echo 'a\u00e9Z'`, echoingUnicode(OutsideLocaleEscapeEncoded, "C"))
	if want := "a\u00e9Z\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", out, st, want)
	}
}

// A `$'…'` refusal is raised while a **word** is expanded rather than while a
// builtin writes, and the status says so: 1 rather than the builtin sites' 0,
// with the command it was part of never run.
//
// Measured 2026-09-11 on zsh 5.9.2 under `LC_ALL=C`: `x=$'a\u00e9Z'; echo AFTER`
// writes the complaint alone and exits 1, and `(exit 3); x=$'a\u00e9Z'` exits 1
// as well — so it is one rather than whatever was already there, exactly as
// the builtin sites' zero is zero rather than what was there (#2021).
func TestADollarSingleEscapeOutsideTheLocaleIsRefusedAtTheWord(t *testing.T) {
	dollarSingle := func(p OutsideLocaleEscapePolicy) func(*Runner) {
		return func(r *Runner) {
			sem := PosixSemantics()
			sem.UnicodeEscapeOutsideTheLocale = p
			r.Semantics = &sem
			r.Vars = map[string]string{"LC_ALL": "C"}
		}
	}
	t.Run("refused", func(t *testing.T) {
		out, st := run(t, `x=$'a\u00e9Z'; echo AFTER`, dollarSingle(OutsideLocaleEscapeRefused))
		if out != refusal || st != 1 {
			t.Errorf("got %q status %d, want %q status 1 — the word never finished", out, st, refusal)
		}
	})
	t.Run("written back", func(t *testing.T) {
		out, st := run(t, `printf '%s' $'a\u00e9Z'`, dollarSingle(OutsideLocaleEscapeWritten))
		if want := `a\u00E9Z`; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, want)
		}
	})
	t.Run("encoded anyway", func(t *testing.T) {
		out, st := run(t, `printf '%s' $'a\u00e9Z'`, dollarSingle(OutsideLocaleEscapeEncoded))
		if want := "a\u00e9Z"; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, want)
		}
	})
	t.Run("and a UTF-8 locale asks nobody", func(t *testing.T) {
		// Unspecified on purpose: reaching the axis would refuse, so a
		// written character is evidence the locale had room for it.
		out, st := run(t, `printf '%s' $'a\u00e9Z'`, func(r *Runner) {
			sem := PosixSemantics()
			sem.UnicodeEscapeOutsideTheLocale = OutsideLocaleEscapeUnspecified
			r.Semantics = &sem
			r.Vars = map[string]string{"LC_ALL": "en_US.UTF-8"}
		})
		if want := "a\u00e9Z"; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, want)
		}
	})
	t.Run("and an ASCII code point asks nobody either", func(t *testing.T) {
		out, st := run(t, `printf '%s' $'a\u0041Z'`,
			func(r *Runner) {
				sem := PosixSemantics()
				r.Semantics = &sem
				r.Vars = map[string]string{"LC_ALL": "C"}
			})
		if out != "aAZ" || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, "aAZ")
		}
	})
}

// Both `printf` sites read the same escape through the same reader, and a
// refusal there ends the **builtin** rather than the pass over the format:
// the format is reused until the operands run out, and a shell that stopped
// only the pass would write the refused text again for every operand left.
//
// Measured 2026-09-11 on zsh 5.9.2 under `LC_ALL=C`: `printf 'a\u00e9Z\n'` writes
// `61` alone — without the newline the format ends with — and the next command
// does not run, with the script's status left at 0.
func TestAPrintfEscapeOutsideTheLocaleEndsTheBuiltin(t *testing.T) {
	printfLocale := func(p OutsideLocaleEscapePolicy) func(*Runner) {
		return func(r *Runner) {
			sem := PosixSemantics()
			sem.PrintfUnicodeEscape = PrintfUnicodeEscapeCodePoint
			sem.PrintfBUnicodeEscape = PrintfUnicodeEscapeCodePoint
			sem.UnicodeEscapeOutsideTheLocale = p
			r.Semantics = &sem
			r.Vars = map[string]string{"LC_ALL": "C"}
		}
	}
	for _, tc := range []struct{ name, src string }{
		{"in a format", `printf 'a\u00e9Z|' x y; echo AFTER`},
		{"in a %b argument", `printf '%b|' 'a\u00e9Z' 'p\u00e9Q'; echo AFTER`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, printfLocale(OutsideLocaleEscapeRefused))
			if want := refusal + "a"; out != want || st != 0 {
				t.Errorf("got %q status %d, want %q status 0 — one complaint, one `a`", out, st, want)
			}
		})
	}
	t.Run("and the escape may stand instead", func(t *testing.T) {
		out, st := run(t, `printf 'a\u00e9Z'`, printfLocale(OutsideLocaleEscapeWritten))
		if want := `a\u00E9Z`; out != want || st != 0 {
			t.Errorf("got %q status %d, want %q status 0", out, st, want)
		}
	})
}
