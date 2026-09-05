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
	return s
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
