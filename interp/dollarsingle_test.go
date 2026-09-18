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
func dollarSingleSem(c DollarSingleControlPolicy, u DollarSingleUnknownPolicy, nul DollarSingleNulPolicy) Semantics {
	s := PosixSemantics()
	s.DollarSingleBackslashC = c
	s.DollarSingleUnknownEscape = u
	s.DollarSingleNul = nul
	// And the two the hexadecimal escape asks, answered the majority way so
	// that a row about the NUL is about the NUL: `$'a\x00b'` has three
	// digits after the `\x` and so reaches the first of them. The tests
	// that are *about* those two set them themselves and assert both sides.
	s.DollarSingleHexReadsEveryDigit = No
	s.DollarSingleDigitlessEscapeIsAZeroByte = No
	// And the one the octal escape asks, for the same reason: `$'a\400b'`
	// is a row about the NUL, so it takes the low byte here — three of the
	// four columns that have `$'…'` do. The test that is *about* this axis
	// sets it itself and asserts both sides.
	s.DollarSingleOctalPastAByteDropsTheLastDigit = No
	// And the three the escape table gave up in #3270, for the same
	// reason: a row about the NUL reaches `\u0000` on the way to it.
	s.DollarSingleEscEscape = Yes
	s.DollarSingleQuestionEscape = Yes
	s.DollarSingleUnicodeEscapes = Yes
	return s
}

// caretMetaSem is the fourth axis on its own, with the other three answered
// so that nothing else in a snippet asks a question.
func caretMetaSem(a DollarSingleCaretMetaPolicy) Semantics {
	s := dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownDropsBackslash, DollarSingleNulIsAByte)
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
			got, st := run(t, tc.src, withSem(caretMetaSem(DollarSingleCaretMetaMaskedWithAnOptionalDash)))
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
	sem := caretMetaSem(DollarSingleCaretMetaAbsent)
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

// The third reading, which is a different escape vocabulary under the same
// two letters rather than the pair above spelled loosely — see
// DollarSingleCaretMetaFoldedWithNoDash. Every row below is one the masked
// reading answers differently, which is the point of having a third value
// instead of a yes: `$'\C-A'` is two bytes here and one there.
//
// Measured 2026-09-13 on ksh93u+ 2012-08-01 through `od -c`.
func TestDollarSingleCaretAndMetaFoldedWithNoDash(t *testing.T) {
	sem := caretMetaSem(DollarSingleCaretMetaFoldedWithNoDash)
	sem.DollarSingleUnknownEscape = DollarSingleUnknownDropsBackslash
	for _, tc := range []struct{ name, src, want string }{
		// No dash in the spelling, so the dash is the argument.
		{"the dash is the argument", `printf '%s' $'\C-A'`, "mA"},
		{"and the letter after it is a letter", `printf '%s' $'\C-1'`, "m1"},
		{"the argument is the very next character", `printf '%s' $'\CA'`, "\x01"},
		// Folded up and then exclusive-ored, which is one rule with no
		// exceptions — the masked reading needs a special case for `?`.
		{"a lowercase letter folds up first", `printf '%s' $'\Ca'`, "\x01"},
		{"a digit is not masked", `printf '%s' $'\C1'`, "q"},
		{"a question mark falls out of the rule", `printf '%s' $'\C?'`, "\x7f"},
		{"a tilde too", `printf '%s' $'\C~'`, ">"},
		{"an at sign is the zero byte", `printf '%s' $'\C@x'`, "\x00x"},
		// The argument may be a further escape, including another one of
		// these.
		{"a hexadecimal escape", `printf '%s' $'\C\x41'`, "\x01"},
		{"an octal one", `printf '%s' $'\C\101'`, "\x01"},
		{"a named one", `printf '%s' $'\C\n'`, "J"},
		{"and a nested control", `printf '%s' $'\C\C-A'`, "\rA"},
		{"with nothing after it, nothing at all", `printf '%s' $'x\C'`, "x"},
		// `\M` is not an escape unless a dash follows, and `\M-` is the
		// escape byte taking no argument.
		{"a meta with no dash is not an escape", `printf '%s' $'\Mx'`, "Mx"},
		{"nor on its own", `printf '%s' $'\M'`, "M"},
		{"the two characters are the escape byte", `printf '%s' $'\M-'`, "\x1b"},
		{"and take nothing after them", `printf '%s' $'\M-x'`, "\x1bx"},
		{"a dash after it is a dash", `printf '%s' $'\M--'`, "\x1b-"},
		{"they compose in either order", `printf '%s' $'\M-\C-x'`, "\x1bmx"},
		{"and the other way", `printf '%s' $'\C-\M-x'`, "m\x1bx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, withSem(sem))
			if got != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", got, st, tc.want)
			}
		})
	}
}

// The two readings over the same spellings, side by side, which is the whole
// reason the axis is three values rather than a yes: every row here is one
// text and two different answers.
func TestTheTwoCaretReadingsPartOverTheSameSpelling(t *testing.T) {
	for _, tc := range []struct{ src, masked, folded string }{
		{`printf '%s' $'\C-A'`, "\x01", "mA"},
		{`printf '%s' $'\C-1'`, "\x11", "m1"},
		{`printf '%s' $'\M-x'`, "\xf8", "\x1bx"},
		{`printf '%s' $'\Mx'`, "\xf8", "Mx"},
		{`printf '%s' $'\C-\M-?'`, "\x9f", "m\x1b?"},
		{`printf '%s' $'\C-\x7f'`, "\x1f", "m\x7f"},
	} {
		got, _ := run(t, tc.src, withSem(caretMetaSem(DollarSingleCaretMetaMaskedWithAnOptionalDash)))
		if got != tc.masked {
			t.Errorf("masked %s: got %q, want %q", tc.src, got, tc.masked)
		}
		got, _ = run(t, tc.src, withSem(caretMetaSem(DollarSingleCaretMetaFoldedWithNoDash)))
		if got != tc.folded {
			t.Errorf("folded %s: got %q, want %q", tc.src, got, tc.folded)
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
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulEndsTheSpan)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.masked {
				t.Errorf("masked: got %q, want %q", got, tc.masked)
			}
			sem = dollarSingleSem(DollarSingleControlToggled, DollarSingleUnknownDropsBackslash, DollarSingleNulEndsTheSpan)
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
	sem := dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownDropsBackslash, DollarSingleNulIsAByte)
	if got, _ := run(t, `printf '%s' $'X\cAY'`, withSem(sem)); got != "XcAY" {
		t.Errorf("dropping: got %q, want %q", got, "XcAY")
	}
	sem = dollarSingleSem(DollarSingleControlAbsent, DollarSingleUnknownKeepsBackslash, DollarSingleNulIsAByte)
	if got, _ := run(t, `printf '%s' $'X\cAY'`, withSem(sem)); got != `X\cAY` {
		t.Errorf("keeping: got %q, want %q", got, `X\cAY`)
	}
}

// An escape with no meaning either keeps both characters or drops the
// backslash, and nothing agrees on which.
func TestDollarSingleUnknownEscape(t *testing.T) {
	sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulEndsTheSpan)
	if got, _ := run(t, `printf '%s' $'no\qescape'`, withSem(sem)); got != `no\qescape` {
		t.Errorf("keeping: got %q, want %q", got, `no\qescape`)
	}
	sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownDropsBackslash, DollarSingleNulEndsTheSpan)
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
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulEndsTheSpan)
			if got, _ := run(t, tc.src, withSem(sem)); got != tc.truncated {
				t.Errorf("truncating: got %q, want %q", got, tc.truncated)
			}
			sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulIsAByte)
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
		`printf '%s' $'\a\v\b\f'`,
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
		// The three #3270 took out of the shared table. They used to be in
		// the list above — `$'\e\a\v'` was one row of it — because a
		// comment over `simpleEscape` called the table unanimous across
		// bash, ksh93 and zsh, a panel written before BusyBox ash had a
		// column. It reads none of the three, so the core refuses them.
		{"the escape character", `printf '%s' $'\e'`},
		{"its capital spelling", `printf '%s' $'\E'`},
		{"the question-mark escape", `printf '%s' $'\?'`},
		{"a code point escape", `printf '%s' $'\u0041'`},
		{"its eight-digit spelling", `printf '%s' $'\U00000041'`},
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
	hexSem := func(every, digitless Answer, nul DollarSingleNulPolicy) Semantics {
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
			if got, _ := run(t, tc.src, withSem(hexSem(Yes, No, DollarSingleNulEndsTheSpan))); got != tc.every {
				t.Errorf("every digit: got %q, want %q", got, tc.every)
			}
			if got, _ := run(t, tc.src, withSem(hexSem(No, No, DollarSingleNulEndsTheSpan))); got != tc.stopAtTwo {
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
			sem := dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulIsAByte)
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
			sem = dollarSingleSem(DollarSingleControlMasked, DollarSingleUnknownKeepsBackslash, DollarSingleNulEndsTheSpan)
			sem.DollarSingleDigitlessEscapeIsAZeroByte = Yes
			if got, _ := run(t, tc.src, withSem(sem)); got != "[]" {
				t.Errorf("truncating: got %q, want %q", got, "[]")
			}
		})
	}
}
