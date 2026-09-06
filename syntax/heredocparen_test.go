// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"errors"
	"testing"
)

// withParenEnd is the grammar of a shell that reads a here-document's body
// from between the parentheses it sits in.
func withParenEnd() Dialect {
	d := Core()
	d.HeredocEndsAtClosingParen = true
	return d
}

// Where a here-document's body ends, when the construct it sits in is closed
// on the delimiter's own line.
//
//	v=$(cat <<EOF
//	a
//	EOF)
//
// Two shells find the parentheses first and read the body from what is between
// them, so `EOF)` is the last line the body could have had and the document is
// delimited; two read the body from the whole input, so it takes the `)` with
// it and nothing ever closes the construct. Nothing here used to ask: the
// paren-counting that finds the `)` counts the same in every dialect, so all
// four accepted what two of the four refuse.
func TestAHereDocumentBodyEndingAtAClosingParenIsADialectQuestion(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		why  string
	}{
		{
			name: "the delimiter carries the closing paren",
			src:  "v=$(cat <<EOF\na\nEOF)\necho \"[$v]\"\n",
			why:  "the shape itself",
		},
		{
			name: "a second here-document carries it",
			src:  "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\necho \"[$v]\"\n",
			why:  "the first document is delimited normally and the second is not, so the question is per document rather than about the first one in the construct",
		},
		{
			name: "two constructs close on the one line",
			src:  "v=$(echo $(cat <<E\nz\nE))\necho \"[$v]\"\n",
			why:  "`E))` closes both, and the body would take both parens with it",
		},
		{
			name: "parentheses that hold a program without a dollar sign",
			src:  "cat <(cat <<EOF\na\nEOF)\necho done\n",
			why:  "it is a question about parentheses holding a program, not about `$( )` — the construct set holdsCommands already names",
		},
		{
			name: "the tab-stripping operator",
			src:  "v=$(cat <<-EOF\n\ta\n\tEOF)\necho \"[$v]\"\n",
			why:  "`<<-` finds its delimiter by a different rule and reaches the same question",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			mustParse(t, c.src, withParenEnd(), c.why)
			mustFail(t, c.src, Core(), c.why)
		})
	}
}

// And where it is refused, the complaint is one the dialect already had.
//
// Both refusing shells answer this with the message they answer a plainly
// unterminated `(` with, word for word — verified against the control
// `v=$(echo hi`. So the state really is "the construct was never closed" and
// not a second thing that resembles it, and there is no wording to add: no
// Diagnostics field, and no dialect has to learn a new sentence.
//
// The whole rendered failure is compared, position included. Each pair is
// written so the construct opens in the same column of the same line, so an
// equal position is a fact about where the failure is attributed and not a
// coincidence — and a fix that reported this somewhere else would be caught.
func TestABodyThatTookTheParenLeavesTheOrdinaryUnterminatedConstruct(t *testing.T) {
	for _, c := range []struct {
		name, src, control string
	}{
		{
			name:    "a command substitution",
			src:     "v=$(cat <<EOF\na\nEOF)\n",
			control: "v=$(echo hi\n",
		},
		{
			name:    "parentheses without a dollar sign",
			src:     "cat <(cat <<EOF\na\nEOF)\n",
			control: "cat <(echo hi\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, want := refusal(t, c.src), refusal(t, c.control)
			if got != want {
				t.Errorf("the complaint differs from the one for a plainly unterminated construct\n  got:  %q\n  want: %q", got, want)
			}
			// And where the parser carries the state a dialect words from,
			// that has to match too: a shell naming the construct or what
			// would have closed it must name the same things.
			g, w := parseErr(t, c.src), parseErr(t, c.control)
			if g == nil || w == nil {
				return
			}
			if g.Kind != w.Kind || g.Construct != w.Construct ||
				g.Innermost != w.Innermost || g.Expected != w.Expected {
				t.Errorf("the state behind the message differs\n  got:  kind=%d construct=%q innermost=%q expected=%q\n  want: kind=%d construct=%q innermost=%q expected=%q",
					g.Kind, g.Construct, g.Innermost, g.Expected,
					w.Kind, w.Construct, w.Innermost, w.Expected)
			}
		})
	}
}

// A prompt has to know it may ask for another line rather than run with what
// it has: the body is still open, which is exactly what the input running out
// inside a construct means.
func TestABodyThatTookTheParenLeavesTheInputIncomplete(t *testing.T) {
	p := NewParser("v=$(cat <<EOF\na\nEOF)\n", Core())
	p.Parse()
	if p.Err() == nil {
		t.Fatal("parsed, want a refusal")
	}
	if !p.Incomplete() {
		t.Error("not reported incomplete; a prompt would run with a half-read construct rather than asking for the rest")
	}
}

// The controls: two shapes that look like this one and are not it.
func TestWhatDoesNotReachTheQuestion(t *testing.T) {
	for _, c := range []struct{ name, src, why string }{
		{
			name: "the delimiter is alone on its line",
			src:  "v=$(cat <<EOF\na\nEOF\n)\necho \"[$v]\"\n",
			why:  "the document is delimited before the `)` is reached, so there is nothing to decide and all six shells take it",
		},
		{
			name: "backquotes rather than parentheses",
			src:  "v=`cat <<E\nq\nE`\necho \"[$v]\"\n",
			why:  "a body cannot hold the mark that closes a backquoted substitution, so the body never takes the closing delimiter and the whole panel agrees",
		},
		{
			name: "an arithmetic substitution",
			src:  "echo $(( a << b ))\n",
			why:  "the parentheses hold an expression rather than a program, so `<<` is a shift and there is no here-document to end anywhere",
		},
		{
			name: "no here-document at all",
			src:  "v=$(echo hi)\necho \"[$v]\"\n",
			why:  "the ordinary case, which the paren-counting fallback must go on reading",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			mustParse(t, c.src, Core(), c.why)
			mustParse(t, c.src, withParenEnd(), c.why)
		})
	}
}

// refusal is the whole of what the parser said, position included.
func refusal(t *testing.T, src string) string {
	t.Helper()
	_, err := Parse(src, Core())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	return err.Error()
}

// parseErr is the *Error behind a refusal, or nil where the failure carries
// no dialect state — which some do not, and which the two sides of a
// comparison then share.
func parseErr(t *testing.T, src string) *Error {
	t.Helper()
	_, err := Parse(src, Core())
	if err == nil {
		t.Fatalf("%q parsed, want a refusal", src)
	}
	var pe *Error
	if !errors.As(err, &pe) {
		return nil
	}
	return pe
}
