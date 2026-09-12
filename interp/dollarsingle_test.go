// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// dollarSingleSem answers the three `$'…'` axes by name.
//
// PosixSemantics is the base rather than CoreSemantics because the snippets
// below use `printf`, and a test about a quoting rule should not be deciding
// anything about a builtin's options.
func dollarSingleSem(c DollarSingleControlPolicy, u DollarSingleUnknownPolicy, nul Answer) Semantics {
	s := PosixSemantics()
	s.DollarSingleBackslashC = c
	s.DollarSingleUnknownEscape = u
	s.DollarSingleNulTruncates = nul
	// And the two the hexadecimal escape asks, answered the majority way so
	// that a row about the NUL is about the NUL: `$'a\x00b'` has three
	// digits after the `\x` and so reaches the first of them. The tests
	// that are *about* those two set them themselves and assert both sides.
	s.DollarSingleHexReadsEveryDigit = No
	s.DollarSingleDigitlessEscapeIsAZeroByte = No
	return s
}

// caretMetaSem is the fourth axis on its own, with the other three answered
// so that nothing else in a snippet asks a question.
func caretMetaSem(a Answer) Semantics {
	s := dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownDropsBackslash, No)
	s.DollarSingleCaretMeta = a
	return s
}

// `\C-X` and `\M-X` inside `$'…'`, which one shell of the panel has.
//
// This is the reading side of what the `q+` expansion flag writes, so the
// rows are picked to cover what that flag can produce — every control byte,
// delete, and every byte above 0x7f — and then the corners a reading gets
// wrong. Measured on zsh 5.9.2, 2026-09-08, by `od`.
func TestDollarSingleCaretAndMeta(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The mask, not an uppercase-and-toggle: the two agree over the
		// letters and part over a digit, which is why `\C-1` is here.
		{"an uppercase letter", `printf '%s' $'\C-A'`, "\x01"},
		{"a lowercase letter", `printf '%s' $'\C-a'`, "\x01"},
		{"a digit", `printf '%s' $'\C-1'`, "\x11"},
		{"a bracket", `printf '%s' $'\C-['`, "\x1b"},
		{"an at sign is the zero byte", `printf '%s' $'\C-@x'`, "\x00x"},
		{"a space", `printf '%s' $'\C- '`, "\x00"},
		{"a minus, which is not the separator twice", `printf '%s' $'\C--'`, "\x0d"},
		// The separator is optional in both, which is what says it is a
		// separator rather than part of the escape's name.
		{"the dash may be left out", `printf '%s' $'\CA'`, "\x01"},
		{"for meta too", `printf '%s' $'\Mx'`, "\xf8"},
		// `?` is the one character the mask is not applied to, and it is the
		// byte exactly: a meta bit takes it out of the rule.
		{"a question mark is delete", `printf '%s' $'\C-?'`, "\x7f"},
		{"by way of an escape as well", `printf '%s' $'\C-\x3f'`, "\x7f"},
		{"where the delete byte itself is masked", `printf '%s' $'\C-\x7f'`, "\x1f"},
		{"and a meta question mark is not delete", `printf '%s' $'\C-\M-?'`, "\x9f"},
		// Meta sets the high bit over whatever the argument came to.
		{"a letter", `printf '%s' $'\M-x'`, "\xf8"},
		{"a named escape", `printf '%s' $'\M-\t'`, "\x89"},
		{"a control escape", `printf '%s' $'\M-\C-?'`, "\xff"},
		{"the zero byte with the bit set", `printf '%s' $'\M-\C-@x'`, "\x80x"},
		{"a space", `printf '%s' $'\M- '`, "\xa0"},
		{"a backslash", `printf '%s' $'\M-\\'`, "\xdc"},
		// The mask keeps the high bit it finds, so the two compose in
		// either order.
		{"control over meta", `printf '%s' $'\C-\M-x'`, "\x98"},
		{"meta over control", `printf '%s' $'\M-\C-x'`, "\x98"},
		// An escape with nothing to work on produces nothing at all, rather
		// than the characters it was written with.
		{"a control with no argument", `printf '%s' $'x\C'`, "x"},
		{"a meta with no argument", `printf '%s' $'x\M-'`, "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, withSem(caretMetaSem(Yes)))
			if got != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, tc.want)
			}
		})
	}
}

// Where the dialect has neither, the backslash is before a character nothing
// claims and the unknown-escape axis decides it — the same division of labor
// `\c` gets. Measured: bash keeps both characters, so `$'\C-A'` there is the
// four characters it was written as.
func TestDollarSingleCaretAndMetaAbsent(t *testing.T) {
	sem := caretMetaSem(No)
	sem.DollarSingleUnknownEscape = DollarSingleUnknownKeepsBackslash
	for _, tc := range []struct{ src, want string }{
		{`printf '%s' $'\C-A'`, `\C-A`},
		{`printf '%s' $'\M-x'`, `\M-x`},
	} {
		if got, st := run(t, tc.src, withSem(sem)); got != tc.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q", tc.src, got, st, tc.want)
		}
	}
}

// `\cX` decodes two different ways, and the difference is invisible over
// letters.
//
// Both answers uppercase the character first — which is why `\ca` and `\cA`
// are the same byte under either — and then either keep its low five bits or
// toggle bit 6. Those agree over `@` through `_`, so a test that asks only
// about `\cA` cannot tell them apart; `\c1` and `\c~` are where they part.
func TestDollarSingleControlCharacter(t *testing.T) {
	for _, tc := range []struct {
		name         string
		src          string
		masked, tggl string
	}{
		{"an uppercase letter", `printf '%s' $'\cA'`, "\x01", "\x01"},
		{"a lowercase letter", `printf '%s' $'\ca'`, "\x01", "\x01"},
		{"the last letter", `printf '%s' $'\cz'`, "\x1a", "\x1a"},
		{"a bracket", `printf '%s' $'\c['`, "\x1b", "\x1b"},
		{"an underscore", `printf '%s' $'\c_'`, "\x1f", "\x1f"},
		{"a backslash", `printf '%s' $'\c\\'`, "\x1c", "\x1c"},
		// The two arithmetics disagree here and nowhere a letter reaches.
		{"a digit", `printf '%s' $'\c1'`, "\x11", "q"},
		{"a tilde", `printf '%s' $'\c~'`, "\x1e", ">"},
		{"a space", `printf '%s' $'\c '`, "", "`"},
		// DEL by two different roads: a special case one way, and plain
		// arithmetic the other.
		{"a question mark", `printf '%s' $'\c?'`, "\x7f", "\x7f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, Yes)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.masked {
				t.Errorf("masked: got %q, want %q", got, tc.masked)
			}
			sem = dollarSingleSem(DollarSingleControlToggled, DollarSingleUnknownDropsBackslash, Yes)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.tggl {
				t.Errorf("toggled: got %q, want %q", got, tc.tggl)
			}
		})
	}
}

// Where the dialect has no `\c` at all, the backslash is before a character
// nothing claims and the unknown-escape axis decides it — which is the whole
// reason the two axes are separate.
func TestDollarSingleControlAbsentFallsToTheUnknownRule(t *testing.T) {
	sem := dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownDropsBackslash, No)
	if got, _ := run(t, `printf '%s' $'X\cAY'`, withSem(sem)); got != "XcAY" {
		t.Errorf("dropping: got %q, want %q", got, "XcAY")
	}
	sem = dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownKeepsBackslash, No)
	if got, _ := run(t, `printf '%s' $'X\cAY'`, withSem(sem)); got != `X\cAY` {
		t.Errorf("keeping: got %q, want %q", got, `X\cAY`)
	}
}

// An escape with no meaning either keeps both characters or drops the
// backslash, and nothing agrees on which.
func TestDollarSingleUnknownEscape(t *testing.T) {
	sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, Yes)
	if got, _ := run(t, `printf '%s' $'no\qescape'`, withSem(sem)); got != `no\qescape` {
		t.Errorf("keeping: got %q, want %q", got, `no\qescape`)
	}
	sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownDropsBackslash, Yes)
	if got, _ := run(t, `printf '%s' $'no\qescape'`, withSem(sem)); got != "noqescape" {
		t.Errorf("dropping: got %q, want %q", got, "noqescape")
	}
}

// A decoded NUL either ends the text or is a byte like any other, and what
// ends is the *span* rather than the word: the characters after the closing
// quote were never inside it.
func TestDollarSingleNulTruncation(t *testing.T) {
	for _, tc := range []struct {
		name             string
		src              string
		truncated, whole string
	}{
		{"an explicit zero", `printf '[%s]' $'a\0b'`, "[a]", "[a\x00b]"},
		{"the rest of the word survives", `printf '[%s]' $'a\0b'ccc`, "[accc]", "[a\x00bccc]"},
		{"later escapes are lost with it", `printf '[%s]' $'a\0b\tc'`, "[a]", "[a\x00b\tc]"},
		{"a hexadecimal zero", `printf '[%s]' $'a\x00b'`, "[a]", "[a\x00b]"},
		{"a code point of zero", `printf '[%s]' $'a\u0000b'`, "[a]", "[a\x00b]"},
		{"an octal value past a byte", `printf '[%s]' $'a\400b'`, "[a]", "[a\x00b]"},
		{"control-at", `printf '[%s]' $'a\c@b'`, "[a]", "[a\x00b]"},
		{"the length of what was assigned", `x=$'a\0b'; printf '[%s]' "${#x}"`, "[1]", "[3]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, Yes)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.truncated {
				t.Errorf("truncating: got %q, want %q", got, tc.truncated)
			}
			sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, No)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.whole {
				t.Errorf("keeping: got %q, want %q", got, tc.whole)
			}
		})
	}
}

// Every axis is asked at the disagreement and nowhere else. A `$'…'` made
// only of the escapes the shells agree about has to run under the core, which
// answers none of the three.
func TestDollarSingleAxesAreAskedOnlyWhereTheyDecide(t *testing.T) {
	for _, src := range []string{
		`printf '%s' $'a\tb'`,
		`printf '%s' $'\x41\101'`,
		`printf '%s' $'\e\a\v'`,
		`printf '%s' $'it\'s'`,
	} {
		t.Run(src, func(t *testing.T) {
			out, st := run(t, src, withSem(CoreSemantics()))
			if st != 0 || strings.Contains(out, "no dialect") {
				t.Errorf("got %q status %d, want no question asked", out, st)
			}
		})
	}
	for _, tc := range []struct{ name, src string }{
		{"a control character", `printf '%s' $'\cA'`},
		{"a caret escape", `printf '%s' $'\C-A'`},
		{"a meta escape", `printf '%s' $'\M-x'`},
		{"an escape with no meaning", `printf '%s' $'\q'`},
		{"a zero byte", `printf '%s' $'a\0b'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, st := run(t, tc.src, withSem(CoreSemantics()))
			if st == 0 {
				t.Errorf("status %d, want a refusal", st)
			}
		})
	}
}

// How far the digit run of a `\x` reaches, and what an empty one comes to
// (#554).
//
// Two axes and one escape. The digit count is asked only where the readings
// can differ — three digits or more — and the empty run only where there are
// none, so an ordinary `$'\x41'` puts neither question to the dialect.
func TestDollarSingleHexDigitRun(t *testing.T) {
	hexSem := func(every, digitless, nul Answer) Semantics {
		s := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, nul)
		s.DollarSingleHexReadsEveryDigit = every
		s.DollarSingleDigitlessEscapeIsAZeroByte = digitless
		return s
	}
	for _, tc := range []struct {
		name, src        string
		every, stopAtTwo string
	}{
		{
			// The sharpest of them: under the short reading the escape is a
			// NUL, which then ends the span in a shell that holds a word as
			// a C string.
			"a zero and a third digit",
			`printf '[%s]' $'\x00b'`,
			"[\x0b]", "[]",
		},
		{
			"a run of three is a code point",
			`printf '[%s]' $'\x414'`,
			"[Д]", "[A4]",
		},
		{
			// The digit count decides and not the value: the same 0xFF is a
			// byte written with two digits and a code point with four.
			"the same value at two widths",
			`printf '[%s]' $'\xFF' $'\x00FF'`,
			"[\xff][ÿ]", "[\xff][]",
		},
		{
			"two digits are a byte either way",
			`printf '[%s]' $'\x41'`,
			"[A]", "[A]",
		},
		{
			// Past the last code point there is, the extended form UTF-8 has
			// room for, and a run too long to hold keeps the low bits.
			"past the last code point",
			`printf '[%s]' $'\x41414141'`,
			"[\xfd\x81\x90\x94\x85\x81]", "[A414141]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := run(t, tc.src, withSem(hexSem(Yes, No, Yes))); got != tc.every {
				t.Errorf("every digit: got %q, want %q", got, tc.every)
			}
			if got, _ := run(t, tc.src, withSem(hexSem(No, No, Yes))); got != tc.stopAtTwo {
				t.Errorf("two digits: got %q, want %q", got, tc.stopAtTwo)
			}
		})
	}
}

// An escape with no hexadecimal digit at all, at the three places it can
// happen. The zero goes through the NUL rule, so the shell that ends a span
// there ends it here too — which is why the third column is the same answer
// as the second and looks nothing like it.
func TestDollarSingleEscapeWithNoDigits(t *testing.T) {
	for _, tc := range []struct{ name, src, zero, written string }{
		{"hex with letters after it", `printf '[%s]' $'\xzz'`, "[\x00zz]", `[\xzz]`},
		{"hex with nothing after it", `printf '[%s]' $'\x'`, "[\x00]", `[\x]`},
		{"a code point escape", `printf '[%s]' $'\uZ'`, "[\x00Z]", `[\uZ]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, No)
			sem.DollarSingleDigitlessEscapeIsAZeroByte = Yes
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.zero {
				t.Errorf("a zero byte: got %q, want %q", got, tc.zero)
			}
			sem.DollarSingleDigitlessEscapeIsAZeroByte = No
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.written {
				t.Errorf("as written: got %q, want %q", got, tc.written)
			}
			// And the zero is the ordinary road to a NUL, so a dialect that
			// ends the span at one ends it here.
			sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, Yes)
			sem.DollarSingleDigitlessEscapeIsAZeroByte = Yes
			if got, _ := run(t, tc.src, withSem(sem)); got != "[]" {
				t.Errorf("truncating: got %q, want %q", got, "[]")
			}
		})
	}
}
