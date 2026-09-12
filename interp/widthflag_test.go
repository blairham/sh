// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// measuringWidths is selectingWithFlags plus the quoting a control character
// has to be written with, so the rows that separate a column from a cell can
// be written at all.
func measuringWidths(d *syntax.Dialect) {
	selectingWithFlags(d)
	d.DollarSingleQuote = true
}

// runWidthFlag runs src with the flag group's grammar, a UTF-8 locale and the
// multibyte axis answered, which is the state `(m)` is a different answer in.
func runWidthFlag(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, measuringWidths, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = Yes
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), "LC_ALL=en_US.UTF-8")
	})
}

// `(m)` measures a character by how wide a terminal draws it.
//
// Every row is a measurement on zsh 5.9.2 under `LC_ALL=en_US.UTF-8`, the one
// panel member with the construct. `w` is two East Asian wide characters, `c`
// is an `e` and a combining acute, and `a` and `s` are the arrays the other
// length flags are measured on.
func TestTheWidthFlagMeasuresColumnsRatherThanCharacters(t *testing.T) {
	const setup = "w=日本; c=e\u0301; a=(日 de f); s='a b  c'; " +
		`b=$'\x01'; e=$'\x1b[31m'; `
	for _, tc := range []struct{ name, src, want string }{
		{"a wide character is two columns", `printf "[%s]" "${(m)#w}"`, "[4]"},
		{"where it is one character", `printf "[%s]" "${#w}"`, "[2]"},
		{"a combining mark is none", `printf "[%s]" "${(m)#c}"`, "[1]"},
		{"where it is a character of its own", `printf "[%s]" "${#c}"`, "[2]"},
		// The control row is what separates this from the line editor's
		// width, which counts a control character as no cell at all.
		{"a control character is one column", `printf "[%s]" "${(m)#b}"`, "[1]"},
		{"so an escape sequence is its characters", `printf "[%s]" "${(m)#e}"`, "[5]"},
		// All-ASCII is the same number under every reading, which is what
		// keeps the flag from reaching a locale question for the strings a
		// script usually holds.
		{"an ASCII value is unchanged", `printf "[%s]" "${(m)#s}"`, "[6]"},
		{"and so is an empty one", `printf "[%s]" "${(m)#nosuch}"`, "[0]"},

		// The flag says what a unit is; it does not say what to count.
		{"a list is still counted in elements", `printf "[%s]" "${(m)#a}"`, "[3]"},
		{"the characters flag is the control for the next row", `printf "[%s]" "${(c)#a}"`, "[6]"},
		{"and with the width flag it is columns", `printf "[%s]" "${(mc)#a}"`, "[7]"},
		{"the words flag has no width to measure", `printf "[%s]" "${(mw)#s}"`, "[3]"},
		{"nor does it lose its own answer", `printf "[%s]" "${(mW)#s}"`, "[4]"},
		// The separator `c` counts is measured in characters whatever the
		// flag says, which is the one probe that separates the two readings:
		// measuring it the same way as the words answers 9.
		{"a wide join separator is counted in characters", `printf "[%s]" "${(cj.日.)#a}"`, "[6]"},
		{"and the flag does not reach it", `printf "[%s]" "${(mcj.日.)#a}"`, "[7]"},
		{"an ASCII separator is the same either way", `printf "[%s]" "${(mcj.--.)#a}"`, "[9]"},

		// It is the length step and nothing else: the value is untouched.
		{"the value itself is unchanged", `printf "[%s]" "${(m)w}"`, "[日本]"},
		{"and a case flag beside it still measures", `printf "[%s]" "${(mU)#w}"`, "[4]"},
		// One element of a list is one string, measured as one.
		{"an element is measured as a string", `printf "[%s]" "${(m)#a[1]}"`, "[2]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runWidthFlag(t, setup+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// Where a length counts bytes, `(m)` counts bytes: there are no characters
// for one to be wider than another.
//
// Measured, and both halves matter — under `LC_ALL=C` and with `multibyte`
// switched off, `${(m)#…}` on two wide characters is 6 in that shell, which
// is what `${#…}` answers there. A flag that reached the width table anyway
// would answer 4 for a value the same shell calls six units long.
func TestTheWidthFlagCountsBytesWhereALengthDoes(t *testing.T) {
	const src = `w=日本; printf "[%s][%s]" "${#w}" "${(m)#w}"`
	for _, tc := range []struct {
		name   string
		answer Answer
		env    []string
		want   string
	}{
		{"a single-byte locale", Yes, []string{"LC_ALL=C"}, "[6][6]"},
		{"a shell with no decoder", No, []string{"LC_ALL=en_US.UTF-8"}, "[6][6]"},
		{"and the UTF-8 control", Yes, []string{"LC_ALL=en_US.UTF-8"}, "[2][4]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, src, selectingWithFlags, func(r *Runner) {
				sem := *r.Semantics
				sem.MultibyteEncodingIsHonored = tc.answer
				r.Semantics = &sem
				r.Env = append(append([]string(nil), r.Env...), tc.env...)
			})
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The padding half of the flag is refused **by name**, and the refusal is
// asserted rather than left to the general unbuilt-flag rule: the letter is
// now in the implemented set, so nothing else would notice if the composition
// started answering a field of the wrong width at status 0.
func TestTheWidthFlagIsRefusedBesideAPaddingFlag(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"on the left", `v=x; printf "[%s]" "${(ml:4::x:)v}"`, "sh: ${(ml:4::x:)v}: the (m) expansion flag is not implemented beside a padding flag\n"},
		{"on the right", `v=x; printf "[%s]" "${(mr:4::x:)v}"`, "sh: ${(mr:4::x:)v}: the (m) expansion flag is not implemented beside a padding flag\n"},
		{"and with the length operator beside it", `v=x; printf "[%s]" "${(ml:4:)#v}"`, "sh: ${(ml:4:)#v}: the (m) expansion flag is not implemented beside a padding flag\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runGrammar(t, tc.src, selectingWithFlags, nil)
			if out != tc.want {
				t.Errorf("output = %q, want %q", out, tc.want)
			}
			if st == 0 {
				t.Errorf("status 0, want the composition refused")
			}
		})
	}
}
