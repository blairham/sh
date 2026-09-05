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
	return s
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
