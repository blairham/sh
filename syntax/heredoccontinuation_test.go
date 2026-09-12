// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document body line that ends in a backslash continues onto the line
// under it, and the delimiter is looked for on what that joins to.
//
// Every column of the panel does this and none of them did it here, which is
// what made #2430 worse than a wrong body: the document ended at the first
// line that merely *looked* like the delimiter, and everything under it — the
// rest of somebody's text — was handed to the parser as commands. Where the
// real delimiter never arrives the read runs to the end of the input, and
// under a terminal there is no end of the input.
//
// The bodies here are the text **as written**, continuations included. An
// unquoted body is raw at this stage and re-read when it expands, and
// HeredocSpans is what removes a continuation there — the same treatment the
// escapes beside it get. What this test pins is where the body *stops*.
func TestABodyLineJoinsAcrossABackslashNewline(t *testing.T) {
	for _, tc := range []struct {
		name, src, body string
		why             string
	}{
		{
			name: "the line under a continued one is not the delimiter",
			src:  "cat <<EOF\nA\\\nEOF\nB\nEOF\n",
			body: "A\\\nEOF\nB\n",
			why:  "the first EOF was asked for by the line above it, so the second one ends the document",
		},
		{
			name: "an ordinary body is untouched",
			src:  "cat <<EOF\nA\nEOF\n",
			body: "A\n",
			why:  "the control: nothing to join and the first delimiter ends it",
		},
		{
			name: "two backslashes end the line",
			src:  "cat <<EOF\nA\\\\\nEOF\nB\n",
			body: "A\\\\\n",
			why:  "a backslash escapes a backslash, so the newline is left with nothing before it and EOF is the delimiter",
		},
		{
			name: "three backslashes continue it",
			src:  "cat <<EOF\nA\\\\\\\nEOF\nB\nEOF\n",
			body: "A\\\\\\\nEOF\nB\n",
			why:  "the first two pair off and the third escapes the newline — parity, not the last character",
		},
		{
			name: "a continuation may join more than two lines",
			src:  "cat <<EOF\nA\\\nEOF\\\nEOF\nB\nEOF\n",
			body: "A\\\nEOF\\\nEOF\nB\n",
			why:  "each physical line is asked in turn, so a run of them is one line of the body",
		},
		{
			name: "the input may run out mid-continuation",
			src:  "cat <<EOF\nA\\\n",
			body: "A\\\n",
			why:  "there is no line to join to, which is unfinished input rather than a loop",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := onlyHeredocBody(t, tc.src, syntax.Core()); got != tc.body {
				t.Errorf("body = %q, want %q\n%s", got, tc.body, tc.why)
			}
		})
	}
}

// A quoted delimiter makes the body literal throughout, and that includes the
// backslash before a newline. It is the control for everything above: the two
// spellings agreed with the panel while the unquoted one did not.
func TestAQuotedDelimiterJoinsNothing(t *testing.T) {
	for _, src := range []string{
		"cat <<'EOF'\nA\\\nEOF\nB\n",
		"cat <<\"EOF\"\nA\\\nEOF\nB\n",
		"cat <<\\EOF\nA\\\nEOF\nB\n",
	} {
		t.Run(src, func(t *testing.T) {
			if got := onlyHeredocBody(t, src, syntax.Core()); got != "A\\\n" {
				t.Errorf("body = %q, want %q", got, "A\\\n")
			}
		})
	}
}

// `<<-` strips tabs from the start of the line *as written*, which is its
// first physical line — so a tab a continuation brings in survives. `→A\` over
// `→B` is `A→B` in every column of the panel.
func TestTabStrippingReachesTheLineAndNotTheJoin(t *testing.T) {
	for _, tc := range []struct{ name, src, body string }{
		{
			name: "the joined line keeps the tab it joined to",
			src:  "cat <<-EOF\n\tA\\\n\tB\n\tEOF\n",
			body: "A\\\n\tB\n",
		},
		{
			name: "the delimiter line is stripped like any other",
			src:  "cat <<-EOF\n\tA\\\n\tEOF\n\tB\n\tEOF\n",
			body: "A\\\n\tEOF\nB\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := onlyHeredocBody(t, tc.src, syntax.Core()); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
		})
	}
}

// Whether the joined text may *be* the delimiter is the axis, and all three
// values are reached here rather than only the two a dialect happens to hold.
//
// The rows are the two shapes that tell them apart: a continuation standing
// after text of the line, and one standing before any.
func TestHowFarADelimiterIsLookedForAcrossAContinuation(t *testing.T) {
	const afterText = "cat <<ABC\nA\\\nBC\nABC\nrest\nABC\n"
	const beforeAny = "cat <<ABC\n\\\nABC\nrest\nABC\n"
	for _, tc := range []struct {
		name                 string
		reach                syntax.ContinuedHeredocDelimiter
		wantAfter, wantStart string
	}{
		{
			name:      "never",
			reach:     syntax.NoHeredocDelimiterAcrossAContinuation,
			wantAfter: "A\\\nBC\n",
			wantStart: "\\\nABC\nrest\n",
		},
		{
			name:      "only a continuation before any text",
			reach:     syntax.HeredocDelimiterAfterALeadingContinuation,
			wantAfter: "A\\\nBC\n",
			wantStart: "",
		},
		{
			name:      "the whole joined line",
			reach:     syntax.HeredocDelimiterOnTheJoinedLine,
			wantAfter: "",
			wantStart: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := syntax.Core()
			d.HeredocDelimiterAcrossAContinuation = tc.reach
			if got := onlyHeredocBody(t, afterText, d); got != tc.wantAfter {
				t.Errorf("a continuation after text: body = %q, want %q", got, tc.wantAfter)
			}
			if got := onlyHeredocBody(t, beforeAny, d); got != tc.wantStart {
				t.Errorf("a continuation before any text: body = %q, want %q", got, tc.wantStart)
			}
		})
	}
}

// A body that continues past what looked like its delimiter is still one
// command, and that is the half of #2430 with teeth: the text under the
// mistaken delimiter used to arrive as statements of the program.
func TestTheBodyDoesNotBecomeStatementsOfTheProgram(t *testing.T) {
	const src = "cat <<EOF\nA\\\nEOF\nB\nEOF\necho after\n"
	p := syntax.NewParser(src, syntax.Core())
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(f.Stmts) != 2 {
		t.Fatalf("Stmts = %d, want 2 — the body's own lines were read as commands", len(f.Stmts))
	}
}

// onlyHeredocBody parses src and returns the body of its one here-document.
func onlyHeredocBody(t *testing.T, src string, d syntax.Dialect) string {
	t.Helper()
	p := syntax.NewParser(src, d)
	f := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if len(f.Stmts) == 0 {
		t.Fatalf("parse %q: no statements", src)
	}
	sc := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if len(sc.Redirs) != 1 || sc.Redirs[0].Heredoc == nil {
		t.Fatalf("no here-document in %q", src)
	}
	return sc.Redirs[0].Heredoc.Literal()
}
