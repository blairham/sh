// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// Input that runs out inside a quote or a substitution carries the whole
// state a dialect might name: the opener, the closer that never came, the
// text near the opener, and the line in both conventions.
func TestUnmatchedDelimitersCarryTheirState(t *testing.T) {
	for _, c := range []struct {
		src, opener, closer, near string
		openLine, eofLine         int
	}{
		{`echo "abc`, `"`, `"`, `"abc`, 1, 1},
		{`echo 'abc`, `'`, `'`, `'abc`, 1, 1},
		{"echo `echo", "`", "`", "`echo", 1, 1},
		{`echo $(echo`, `$(`, `)`, `$(echo`, 1, 1},
		// The two spellings that hold a program the other way. Before
		// #1023 these were not an *Error at all — a plain formatted
		// failure with none of this state on it, so no dialect could word
		// them and ParseFailureLine answered 0.
		{`cat <(echo`, `<(`, `)`, `<(echo`, 1, 1},
		{`cat >(echo`, `>(`, `)`, `>(echo`, 1, 1},
		{"cat <(cat <<E\na\nE)\n", `<(`, `)`, `<(cat <<E`, 1, 4},
		{`echo ${x`, `${`, `}`, `${x`, 1, 1},
		// A `${` opened inside a double quote blames the quote, which is
		// what three of the panel do — the fourth's wording names the
		// quote character too. The near text still starts at the `${`,
		// because that is the scan that ran out; nothing words it here.
		{`echo "${x"`, `"`, `"`, `${x"`, 1, 1},
		{"echo ok\necho \"abc\ndef", `"`, `"`, `"abc`, 2, 3},
	} {
		d := Core()
		// The two process-substitution rows need the construct to exist,
		// and the here-document one needs a body that does not stop at the
		// `)` — otherwise it closes and there is nothing unmatched (#963).
		d.ProcessSubstitution = true
		_, err := Parse(c.src, d)
		var se *Error
		if !errors.As(err, &se) || se.Kind != ErrUnmatched {
			t.Errorf("%q: got %v, want an ErrUnmatched", c.src, err)
			continue
		}
		if se.Token != c.opener || se.Expected != c.closer {
			t.Errorf("%q: opener %q closer %q, want %q %q", c.src, se.Token, se.Expected, c.opener, c.closer)
		}
		if se.LastToken != c.near {
			t.Errorf("%q: near %q, want %q", c.src, se.LastToken, c.near)
		}
		if se.Pos.Line != c.openLine || se.EofLine != c.eofLine {
			t.Errorf("%q: lines %d/%d, want %d/%d", c.src, se.Pos.Line, se.EofLine, c.openLine, c.eofLine)
		}
	}
}

// CloseQuotesAtEOF ends an unterminated quote at the end of input as if the
// closing mark were there; the substitutions still refuse.
func TestTheEndOfInputMayCloseAQuote(t *testing.T) {
	d := Core()
	d.CloseQuotesAtEOF = true
	for _, src := range []string{`echo "abc`, `echo 'abc`, "echo `echo"} {
		f, err := Parse(src, d)
		if err != nil {
			t.Errorf("%q: %v, want the quote closed and the command kept", src, err)
			continue
		}
		if len(f.Stmts) != 1 {
			t.Errorf("%q: %d statements, want one", src, len(f.Stmts))
		}
	}
	for _, src := range []string{`echo $(echo`, `echo ${x`} {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("%q parsed, want a refusal — a substitution is not a quote", src)
		}
	}
}
