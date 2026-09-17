// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The unit a string conversion's field is counted in —
// Semantics.PrintfFieldCountsCharacters and
// Semantics.PrintfLongModifierCountsCharacters. Tests name the axes and never
// a shell; the measurements are on the fields and in the dialect suites.
//
// Every operand here is `αβγ`: three characters and six bytes, so a field
// counted in either unit writes something the other cannot.

func printfUnitRun(t *testing.T, src string, plain, long Answer) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.MultibyteEncodingIsHonored = Yes
	sem.PrintfLengthModifiers = PrintfLengthModifiersC99
	sem.PrintfFieldCountsCharacters = plain
	sem.PrintfLongModifierCountsCharacters = long
	sem.PrintfQuote = PrintfQuoteAnsiCWord
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

const utf8Locale = "LC_ALL=en_US.UTF-8; "

func TestAStringFieldIsCountedInTheUnitTheAxisNames(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		chars, byts string
	}{
		{"a precision", `printf '[%.2s]' αβγ`, "[αβ]", "[α]"},
		{"a width", `printf '[%7s]' αβγ`, "[    αβγ]", "[ αβγ]"},
		{"a width on the left", `printf '[%-7s]' αβγ`, "[αβγ    ]", "[αβγ ]"},
		{"both", `printf '[%5.2s]' αβγ`, "[   αβ]", "[   α]"},
		{"a precision cutting inside a character", `printf '[%.3s]' αβγ`, "[αβγ]", "[α\xce]"},
		{"through %b", `printf '[%.2b]' αβγ`, "[αβ]", "[α]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.chars}, {No, tc.byts}} {
				out, errs, st := printfUnitRun(t, utf8Locale+tc.src, side.answer, No)
				if out != side.want || errs != "" || st != 0 {
					t.Errorf("%v: %s = %q (stderr %q, status %d), want %q",
						side.answer, tc.src, out, errs, st, side.want)
				}
			}
		})
	}
}

// Where the locale has no characters, both answers count bytes — so the
// dialect's answer is not what decides the C column.
func TestAStringFieldIsBytesWhereTheLocaleHasNoCharacters(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		src := `LC_ALL=C; printf '[%.2s|%7s|%.2ls|%lc]' αβγ αβγ αβγ αβγ`
		out, errs, st := printfUnitRun(t, src, answer, answer)
		if want := "[α| αβγ|α|\xce]"; out != want || errs != "" || st != 0 {
			t.Errorf("%v: %s = %q (stderr %q, status %d), want %q", answer, src, out, errs, st, want)
		}
	}
}

// The quoting conversion is bytes under both answers.
func TestAQuotedFieldIsAlwaysBytes(t *testing.T) {
	src := utf8Locale + `printf '[%.2q]' αβγ`
	out, errs, st := printfUnitRun(t, src, Yes, Yes)
	if want := "[α]"; out != want || errs != "" || st != 0 {
		t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, want)
	}
}

func TestTheLongModifierCountsInTheUnitItsAxisNames(t *testing.T) {
	for _, tc := range []struct {
		name, src   string
		chars, byts string
	}{
		{"a precision", `printf '[%.2ls]' αβγ`, "[αβ]", "[α]"},
		{"a width", `printf '[%7ls]' αβγ`, "[    αβγ]", "[ αβγ]"},
		{"any l in the run", `printf '[%.2lls]' αβγ`, "[αβ]", "[α]"},
		{"a character", `printf '[%lc]' αβγ`, "[α]", "[\xce]"},
		{"a character in a width", `printf '[%3lc]' αβγ`, "[  α]", "[  \xce]"},
		{"a character on the left", `printf '[%-3lc]' αβγ`, "[α  ]", "[\xce  ]"},
		{"a character's precision", `printf '[%.0lc]' abc`, "[]", "[a]"},
		{"a precision and a width", `printf '[%5.0lc]' ''`, "[     ]", "[    \x00]"},
		{"no character at all", `printf '[%-3lc]' ''`, "[\x00  ]", "[\x00  ]"},
		// Not the letter's to decide: `%lb` is the plain conversion's.
		{"%lb", `printf '[%.2lb]' αβγ`, "[α]", "[α]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, side := range []struct {
				answer Answer
				want   string
			}{{Yes, tc.chars}, {No, tc.byts}} {
				out, errs, st := printfUnitRun(t, utf8Locale+tc.src, No, side.answer)
				if out != side.want || errs != "" || st != 0 {
					t.Errorf("%v: %s = %q (stderr %q, status %d), want %q",
						side.answer, tc.src, out, errs, st, side.want)
				}
			}
		})
	}
}

// A column that ignores the letter falls back to the plain conversion's
// reading, so `%ls` there is `%s`.
func TestTheLongModifierFallsBackToThePlainReading(t *testing.T) {
	src := utf8Locale + `printf '[%.2ls|%lc]' αβγ αβγ`
	out, errs, st := printfUnitRun(t, src, Yes, No)
	if want := "[αβ|\xce]"; out != want || errs != "" || st != 0 {
		t.Errorf("%s = %q (stderr %q, status %d), want %q", src, out, errs, st, want)
	}
}

// Neither axis is asked of an operand that is all ASCII or a field with no
// width and no precision, so a dialect that answers neither still prints.
func TestTheFieldUnitIsNotAskedWhereItCannotMatter(t *testing.T) {
	for _, src := range []string{
		`printf '[%.2s|%7ls|%lc]' abc abc abc`,
		`printf '[%s|%ls|%b]' αβγ αβγ αβγ`,
	} {
		out, errs, st := printfUnitRun(t, utf8Locale+src, Unspecified, Unspecified)
		if errs != "" || st != 0 || out == "" {
			t.Errorf("%s = %q (stderr %q, status %d), want no question asked", src, out, errs, st)
		}
	}
}
