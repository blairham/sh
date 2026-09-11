// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// EchoExpandsUnicodeEscapes admits `\uHHHH` and `\UHHHHHHHH`, each read as a
// code point and written in UTF-8.
//
// Every case asserts the *bytes*, because the wrong answers here are all
// plausible-looking text: a code point written as one byte, a surrogate
// replaced by U+FFFD, and the escape left standing are three different
// strings that a Contains check on the letter would not tell apart.
func TestEchoExpandsUnicodeEscapes(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"four digits", `echo 'a\u0041Z'`, "aAZ\n"},
		{"eight digits", `echo 'a\U00000041Z'`, "aAZ\n"},
		{"fewer digits than the width", `echo 'a\u41Z'`, "aAZ\n"},
		{"the run stops at the width", `echo 'a\u004100'`, "aA00\n"},
		{"two bytes", `echo 'a\u00e9Z'`, "aéZ\n"},
		{"three bytes", `echo 'a\u20acZ'`, "a€Z\n"},
		{"four bytes", `echo 'a\U0001F600Z'`, "a\U0001F600Z\n"},
		{"two of them in one word", `echo 'a\u0041\u0042'`, "aAB\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.EchoInterpretsEscapes = Yes
			sem.EchoExpandsUnicodeEscapes = Yes
			// In a UTF-8 locale, where the reading is the whole of the
			// question: what a code point the encoding cannot hold does is
			// localeescape_test.go's, and what an unset locale is is the
			// dialect's (#2020).
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Vars = map[string]string{"LC_ALL": "en_US.UTF-8"}
			})
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// The same words with the axis answered No: the two characters stand, which is
// what the four shells without the escape write.
func TestEchoWithoutUnicodeEscapesLeavesThemAsWritten(t *testing.T) {
	sem := PosixSemantics()
	sem.EchoInterpretsEscapes = Yes
	sem.EchoExpandsUnicodeEscapes = No
	for _, src := range []string{`echo 'a\u0041Z'`, `echo 'a\U00000041Z'`} {
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		if !strings.Contains(out, `\u`) && !strings.Contains(out, `\U`) {
			t.Errorf("%s = %q, want the escape left standing", src, out)
		}
	}
}

// It is the *original* UTF-8 and not the range it was later narrowed to. A
// surrogate and a value past the last code point are encoded rather than
// replaced, and the five- and six-byte forms are reachable.
//
// These are the cases Go's rune type refuses: every one of them is U+FFFD
// under a naive encoder, so a test asserting only on `A` would pass for
// an implementation that gets all four of these wrong.
func TestEchoUnicodeEscapesUseTheWholeEncoding(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []byte
	}{
		{"a surrogate", `echo -n '\ud800'`, []byte{0xed, 0xa0, 0x80}},
		{"past the last code point", `echo -n '\U110000'`, []byte{0xf4, 0x90, 0x80, 0x80}},
		{"five bytes", `echo -n '\U200000'`, []byte{0xf8, 0x88, 0x80, 0x80, 0x80}},
		{"six bytes", `echo -n '\U4000000'`, []byte{0xfc, 0x84, 0x80, 0x80, 0x80, 0x80}},
		{"the last one six bytes hold", `echo -n '\U7fffffff'`, []byte{0xfd, 0xbf, 0xbf, 0xbf, 0xbf, 0xbf}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.EchoInterpretsEscapes = Yes
			sem.EchoExpandsUnicodeEscapes = Yes
			sem.EchoOptions = "n"
			out, _ := run(t, tc.src, func(r *Runner) {
				r.Semantics = &sem
				r.Vars = map[string]string{"LC_ALL": "en_US.UTF-8"}
			})
			if out != string(tc.want) {
				t.Errorf("%s = % x, want % x", tc.src, out, tc.want)
			}
		})
	}
}

// EchoEmptyHexDigitRunIsNul is one question for `\x`, `\u` and `\U` together,
// and it is asked only where such an escape actually runs out of digits.
func TestAHexadecimalEscapeWithNoDigits(t *testing.T) {
	const src = `echo -n 'a\xZ'; echo -n '|'; echo -n 'a\uZ'; echo -n '|'; echo -n 'a\U'`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"as written", No, `a\xZ|a\uZ|a\U`},
		{"a nul", Yes, "a\x00Z|a\x00Z|a\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.EchoInterpretsEscapes = Yes
			sem.EchoExpandsHexEscapes = Yes
			sem.EchoExpandsUnicodeEscapes = Yes
			sem.EchoEmptyHexDigitRunIsNul = tc.answer
			sem.EchoOptions = "n"
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The axis is not asked where the escape has its digits, so a word that cannot
// differ needs no answer to it — the same rule the hexadecimal and escape
// letters follow.
func TestTheEmptyRunAxisIsAskedOnlyWhereARunIsEmpty(t *testing.T) {
	sem := PosixSemantics()
	sem.EchoInterpretsEscapes = Yes
	sem.EchoExpandsHexEscapes = Yes
	sem.EchoExpandsUnicodeEscapes = Yes
	sem.EchoEmptyHexDigitRunIsNul = Unspecified
	out, st := run(t, `echo 'a\x41BZ'`, func(r *Runner) { r.Semantics = &sem })
	if out != "aABZ\n" || st != 0 {
		t.Errorf("got %q (status %d), want aABZ at 0 with nothing refused", out, st)
	}
	out, st = run(t, `echo 'a\xZ'`, func(r *Runner) { r.Semantics = &sem })
	if !strings.Contains(out, "no digits") || st != 2 {
		t.Errorf("got %q (status %d), want the unanswered axis named at 2", out, st)
	}
}
