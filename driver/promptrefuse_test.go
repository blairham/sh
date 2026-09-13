// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// In-package, for the reason parsestatus_test.go is: what is asserted is that
// an unexported builder reads a dialect's answer.
package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Whether a construct the parser refused still asks for another line is the
// dialect's answer, carried to the prompt rather than decided there.
//
// A dialect's answer dropped on the floor in this builder looks exactly like a
// dialect that did not answer, which is why it is asserted here and not only
// through a session: three of the four shells refuse at once and the fourth
// waits, so a wiring that always passed false would pass every test written
// against the majority (#1893).
func TestTheFrontEndCarriesWhetherAPromptAsksAgain(t *testing.T) {
	for _, answer := range []bool{false, true} {
		sh := Shell{
			Name:      "testsh",
			Semantics: interp.Semantics{PromptAsksAgainAfterARefusedToken: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).AskAgainAfterARefusedToken; got != answer {
			t.Errorf("the prompt was told %v, want %v", got, answer)
		}
	}
}

// And which option a `#` typed at the prompt waits on, carried the same way
// and for the same reason: three of the four shells name none, so a wiring
// that always passed the empty string would pass every test written against
// the majority and leave the fourth reading its own prompt the wrong way
// (#2537).
func TestTheFrontEndCarriesWhichOptionAHashWaitsOn(t *testing.T) {
	for _, answer := range []string{"", "interactivecomments"} {
		sh := Shell{
			Name:      "testsh",
			Semantics: interp.Semantics{PromptCommentsNeedTheOption: answer},
		}.withDefaults([]string{"testsh"})
		r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
		if got := sh.frontEnd(r, "testsh", sh.Diagnostics).CommentsNeedTheOption; got != answer {
			t.Errorf("the prompt was told %q, want %q", got, answer)
		}
	}
}

// The front end words a parse failure the way the dialect words one at a
// **prompt**, which is its own route: three of the four name no line there.
//
// Asserted through the builder rather than only through a session, for the
// reason above — a route dropped here looks exactly like a dialect that has
// no prompt answer, and the general answer is a plausible-looking sentence.
func TestTheFrontEndWordsAParseFailureTheWayAPromptDoes(t *testing.T) {
	sh := Shell{
		Name: "testsh",
		Diagnostics: interp.Diagnostics{
			Location:         interp.LocationLineWord,
			PromptLocation:   interp.LocationNameOnly,
			SyntaxUnexpected: "wrong: %[1]s",
		},
	}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	front := sh.frontEnd(r, "testsh", sh.Diagnostics)

	err := &syntax.Error{Kind: syntax.ErrUnexpected, Token: ";", Pos: syntax.Pos{Line: 2}}
	if got, want := front.Report(err), "testsh: wrong: ;\n"; got != want {
		t.Errorf("the prompt says %q, want %q", got, want)
	}
	if got := sh.Diagnostics.ParseDiagnostic("testsh", "", err, ""); got == front.Report(err) {
		t.Errorf("the prompt is using the general answer, %q", got)
	}
}

// And what it says about input it accepted anyway, worded the same way. One
// dialect warns about a here-document the input ran out inside; the prompt
// had no path to say it at all.
func TestTheFrontEndCarriesARemark(t *testing.T) {
	sh := Shell{
		Name: "testsh",
		Diagnostics: interp.Diagnostics{
			Location:          interp.LocationLineWord,
			PromptLocation:    interp.LocationNameOnly,
			HereDocumentAtEOF: "warning: here-document at line %[1]d (wanted `%[2]s')",
		},
	}.withDefaults([]string{"testsh"})
	r := sh.newRunner("testsh", nil, sh.Diagnostics, interp.RouteCommandString)
	front := sh.frontEnd(r, "testsh", sh.Diagnostics)
	if front.Remark == nil {
		t.Fatal("the prompt was given no way to say what the parser remarked")
	}
	rk := syntax.Remark{Kind: syntax.RemarkHeredocAtEOF, Token: "EOT", At: syntax.Pos{Line: 1}, Pos: syntax.Pos{Line: 2}}
	if got, want := front.Remark(rk), "testsh: warning: here-document at line 1 (wanted `EOT')\n"; got != want {
		t.Errorf("the prompt says %q, want %q", got, want)
	}
	// A dialect that remarks on nothing writes nothing, which is three of
	// the four.
	quiet := Shell{Name: "testsh"}.withDefaults([]string{"testsh"})
	qr := quiet.newRunner("testsh", nil, quiet.Diagnostics, interp.RouteCommandString)
	if got := quiet.frontEnd(qr, "testsh", quiet.Diagnostics).Remark(rk); got != "" {
		t.Errorf("a dialect with no remark said %q", got)
	}
}
