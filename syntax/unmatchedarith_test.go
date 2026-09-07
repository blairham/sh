// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// An unterminated `$((` and `$[` carry what a dialect words from, the way
// every other unterminated construct has since #1023.
//
// Before this they took a plain formatted error: all four dialects said the
// lexer's own "unterminated arithmetic substitution", none said what its
// shell says, and ParseFailureLine answered 0 because there was no
// *syntax.Error to read a line from.
//
// The closer is the part worth asserting per spelling. It is `)` for `$((`
// and not `))` — measured, the dialect that echoes the closer back says
// "looking for matching `)'" for `echo $((1+2` — and `]` for `$[`. So the two
// spellings are one kind with two delimiter pairs, which is why the closer
// comes from the construct rather than from each dialect's sentence.
func TestAnUnterminatedArithmeticSubstitutionCarriesItsState(t *testing.T) {
	// `$[` is a dialect construct, so the rows that use it say so. Core has
	// the parenthesised spelling and not the bracketed one, which is exactly
	// the split the two shells that have `$[` make.
	bracket := Core()
	bracket.DollarBracketArith = true
	for _, c := range []struct {
		why, src, opener, closer, near string
		d                              Dialect
	}{
		{
			"the parenthesised spelling names one closing parenthesis",
			"echo $((1+2", "$((", ")", "$((1+2", Core(),
		},
		{
			"the bracketed spelling names its own bracket",
			"echo $[1+2", "$[", "]", "$[1+2", bracket,
		},
		{
			// The word rather than the construct, which is #1022's rule and
			// reaches this construct now that it goes through the same helper.
			"the quoted text is the word, assignment prefix included",
			"v=$((1+2", "$((", ")", "v=$((1+2", Core(),
		},
		{
			"and the word of the bracketed spelling",
			"v=$[1+2", "$[", "]", "v=$[1+2", bracket,
		},
	} {
		_, err := Parse(c.src, c.d)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%s: %q: got %v, want an ErrUnmatched", c.why, c.src, err)
			continue
		}
		if se.Token != c.opener || se.Expected != c.closer {
			t.Errorf("%s: %q: opener %q closer %q, want %q %q",
				c.why, c.src, se.Token, se.Expected, c.opener, c.closer)
		}
		if se.LastToken != c.near {
			t.Errorf("%s: %q: near %q, want %q", c.why, c.src, se.LastToken, c.near)
		}
	}
}

// The line the failure is *located* on, in both conventions the dialects read.
//
// This is what made #1086 a second measurement rather than a rider on #1023:
// a dialect that reports an unmatched `$(` at the line after the input's last
// reports `$((` at the opener's line. Both numbers have to be right in the
// error for either dialect to be, so both are pinned here — EofLine, which is
// the line the input ran out on, and Pos.Line, which is the opener's.
//
// A construct on line 1 of a one-line file makes the two numbers 1 and 2, so
// a mutant that returned either where the other was wanted would show.
func TestAnUnterminatedArithmeticSubstitutionCarriesBothLines(t *testing.T) {
	bracket := Core()
	bracket.DollarBracketArith = true
	for _, c := range []struct {
		why, src        string
		openerLine, eof int
		d               Dialect
	}{
		{"one line, so the opener is 1 and the input ran out on 2", "echo $((1+2\n", 1, 2, Core()},
		{"the bracketed spelling, the same way", "echo $[1+2\n", 1, 2, bracket},
		{"a construct further down carries its own opener", "echo pre\necho $((1+2\n", 2, 3, Core()},
		{"and the input running out several lines later", "echo $((1+2\n+3\n+4\n", 1, 4, Core()},
	} {
		_, err := Parse(c.src, c.d)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%s: %q: got %v, want an ErrUnmatched", c.why, c.src, err)
			continue
		}
		if se.Pos.Line != c.openerLine {
			t.Errorf("%s: %q: opener on line %d, want %d", c.why, c.src, se.Pos.Line, c.openerLine)
		}
		if se.EofLine != c.eof {
			t.Errorf("%s: %q: ran out on line %d, want %d", c.why, c.src, se.EofLine, c.eof)
		}
	}
}
