// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
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
	return s
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
