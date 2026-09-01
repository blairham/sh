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
