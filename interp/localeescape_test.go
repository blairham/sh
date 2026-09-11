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
	return func(r *Runner) {
		sem := PosixSemantics()
		sem.EchoInterpretsEscapes = Yes
		sem.EchoExpandsUnicodeEscapes = Yes
		sem.UnicodeEscapeOutsideTheLocale = p
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
		// Nothing names a locale: left alone, deliberately, because the panel
		// disagrees about that case and the disagreement is not this axis's.
		// See Runner.localeRefusesCodePoint.
		{"nothing named is left alone", "", `echo 'a\u00e9Z'`, "a\u00e9Z\n"},
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
