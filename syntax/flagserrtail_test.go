// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The source a flag-group error quotes is the rest of the **word**, from the
// expansion's `$` to the word's end, and not the expansion alone (#1647).
//
// Measured on zsh 5.9.2, 2026-09-12. The discriminating row is the third: a
// rule that took the rest of the *line* would carry ` tail` with it, and a
// rule that took the expansion alone would carry nothing in any of the rows
// below it.
func TestAFlagGroupErrorCarriesTheRestOfItsWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the expansion is the whole word", `echo ${(!)v}`, `${(!)v}`},
		{"a literal behind it", `echo ${(!)v}rest more`, `${(!)v}rest`},
		{"and the word ends at the blank", `echo ${(g:x:)v} tail`, `${(g:x:)v}`},
		{"the closing quote of the region it stands in", `print -r -- "[${(Z:x:)v}]"`, `${(Z:x:)v}]"`},
		{"an unterminated group takes it too", `echo "${(Ux}"`, `${(Ux}"`},
		{"across three quotings", `echo "${(!)v}"$'q'tail`, `${(!)v}"$'q'tail`},
		{"a newline inside the word", "echo \"a${(!)v}b\nsecond\"", "${(!)v}b\nsecond\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := Core()
			d.ParamExpansionFlags = true
			d.DollarSingleQuote = true
			e := firstParam(t, tc.src, d)
			if e.FlagsErrPos == 0 {
				t.Fatalf("no flags error in %q", tc.src)
			}
			if e.FlagsErrTail != tc.want {
				t.Errorf("tail = %q, want %q", e.FlagsErrTail, tc.want)
			}
		})
	}
}

// The guard the tail is taken behind. A word the grammar supplied rather than
// one a script wrote stands at a single position with nothing between its
// ends, and an alias body is lexed apart from the input these offsets count
// in — so a pair that does not describe a range of *this* source has to
// produce nothing, and the report falls back to the expansion alone.
func TestSourceBetweenAnswersOnlyForRealRanges(t *testing.T) {
	p := NewParser("echo hi", Core())
	at := func(off int32) Pos { return Pos{Offset: off, Line: 1, Col: off + 1} }
	for _, tc := range []struct {
		name     string
		from, to Pos
		want     string
	}{
		{"a range of the source", at(0), at(4), "echo"},
		{"one position, no range", at(2), at(2), ""},
		{"reversed", at(4), at(0), ""},
		{"past the end", at(0), at(99), ""},
		{"a negative start", Pos{Offset: -1, Line: 1}, at(4), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.sourceBetween(tc.from, tc.to); got != tc.want {
				t.Errorf("sourceBetween = %q, want %q", got, tc.want)
			}
		})
	}
}
