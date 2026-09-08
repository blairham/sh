// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// Input that runs out inside a quote or a substitution carries the whole
// state a dialect might name: the opener, the closer that never came, the
// text near it, and the line in both conventions.
//
// The near text is the **word** the construct was written in, to the end of
// its line — not the construct. Every row here has the two coincide except
// the quoted `${`, because `echo ` precedes the opener in all of them; the
// rows that tell them apart live in the dialect that quotes it, where an
// assignment prefix and a preceding quoted span both belong to the word
// (#1022).
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
		// quote character too. The near text is the whole word either way:
		// it used to start at the `${`, because that was the scan that ran
		// out, and it starts at the word now. Nothing words this row, so it
		// is here as the rule being uniform rather than as a measurement.
		{`echo "${x"`, `"`, `"`, `"${x"`, 1, 1},
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
	d.CloseQuotesAtEOF = RouteFromCommandString
	d = d.On(RouteFromCommandString)
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

// The same dialect on a route it did not name refuses every one of those
// quotes, which is the whole of #1424: the leniency is one shell's answer for
// a command string and not its answer for a shell.
//
// Every route is asked by name rather than only the file one, because the
// defect was a flag applied where it had not been measured and a test that
// asks about one route reintroduces exactly that.
func TestAQuoteIsClosedOnlyOnTheRouteTheDialectNames(t *testing.T) {
	quotes := []string{`echo "abc`, `echo 'abc`, "echo `echo"}
	for _, route := range []ProgramRoutes{RouteOnNoRoute, RouteFromScriptFile, RouteOnStandardInput} {
		d := Core()
		d.CloseQuotesAtEOF = RouteFromCommandString
		d = d.On(route)
		for _, src := range quotes {
			if _, err := Parse(src, d); err == nil {
				t.Errorf("route %d: %q parsed, want a refusal", route, src)
			}
		}
	}
	// And a dialect that names no route at all refuses on the route the one
	// that does would have allowed — the flag is the dialect's answer, not
	// the route's.
	d := Core()
	d = d.On(RouteFromCommandString)
	for _, src := range quotes {
		if _, err := Parse(src, d); err == nil {
			t.Errorf("no dialect route: %q parsed, want a refusal", src)
		}
	}
}

// On leaves the language alone and answers only about this program, which is
// what lets one shell's dialect be shared by every route it is read on.
func TestOnChangesTheRouteAndNothingElse(t *testing.T) {
	d := Core()
	d.CloseQuotesAtEOF = RouteFromCommandString
	got := d.On(RouteFromScriptFile)
	if got.ProgramRoute != RouteFromScriptFile {
		t.Errorf("route %d, want %d", got.ProgramRoute, RouteFromScriptFile)
	}
	if got.CloseQuotesAtEOF != RouteFromCommandString {
		t.Errorf("On changed the language: CloseQuotesAtEOF %d", got.CloseQuotesAtEOF)
	}
	if d.ProgramRoute != RouteOnNoRoute {
		t.Errorf("On wrote through to the receiver: %d", d.ProgramRoute)
	}
}
