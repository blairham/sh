// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// A here-document body is read from the whole input here, so the `)` written
// onto the delimiter's line goes into the body and nothing closes the
// substitution.
//
//	v=$(cat <<EOF
//	a
//	EOF)
//
// Measured against /bin/dash: `Syntax error: end of file unexpected
// (expecting ")")`, at the line the input ran out on. Which is, word for
// word, what it says about `v=$(echo hi` — this shell needs no new sentence
// for it, because the state is the one it already has a sentence for (#963).
func TestADelimiterCarryingTheClosingParenLeavesTheSubstitutionOpen(t *testing.T) {
	const want = `Syntax error: end of file unexpected (expecting ")")`
	for _, c := range []struct {
		name, src string
		line      int
	}{
		{"the delimiter carries the paren", "v=$(cat <<EOF\na\nEOF)\necho x\n", 5},
		{"a second here-document carries it", "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\necho x\n", 8},
		{"two constructs close on the one line", "v=$(echo $(cat <<E\nz\nE))\necho x\n", 5},
		{"the control: nothing closed the substitution at all", "v=$(echo hi\necho x\n", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := syntax.Parse(c.src, dash.Dialect())
			if err == nil {
				t.Fatalf("%q parsed; this shell refuses it", c.src)
			}
			d := dash.Diagnostics()
			if got := d.ParseFailure(err); got != want {
				t.Errorf("said %q, want %q", got, want)
			}
			// The whole of what this shell prints is that sentence and this
			// number, and the number is the line the input ran out on —
			// measured against /bin/dash on each of these. Located at the
			// `)` instead it is three lines early on the first of them.
			if got := d.ParseFailureLine(err); got != c.line {
				t.Errorf("blamed line %d, want %d", got, c.line)
			}
		})
	}
}

// And what it goes on reading, so the refusal is about the shape rather than
// about here-documents inside parentheses.
func TestWhatThisShellStillReadsBesideThatShape(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"the delimiter is alone on its line", "v=$(cat <<EOF\na\nEOF\n)\necho \"[$v]\"\n"},
		{"backquotes cannot be taken into a body", "v=`cat <<E\nq\nE`\necho \"[$v]\"\n"},
		{"no here-document at all", "v=$(echo hi)\necho \"[$v]\"\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := syntax.Parse(c.src, dash.Dialect()); err != nil {
				t.Errorf("%q: %v", c.src, err)
			}
		})
	}
}
