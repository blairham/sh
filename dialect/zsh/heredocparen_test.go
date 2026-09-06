// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// A here-document body is read from the whole input here, so the `)` written
// onto the delimiter's line goes into the body and nothing closes the
// construct it was meant to close.
//
//	v=$(cat <<EOF
//	a
//	EOF)
//
// Measured against zsh 5.9.2, which refuses it — with the message it already
// has for an unterminated `(`, quoting the text the construct began with. So
// there is nothing to word for this shape, which is the claim: the whole
// rendered failure is pinned, and the control below shows the same wording
// reached from a plainly unterminated construct (#963).
//
// It is not a question about `$( )`. Real zsh refuses `cat <(cat <<EOF` …
// `EOF)` the same way, which is why both spellings are here.
//
// Two of these differ from what real zsh prints, and both differences are
// this shell's already and are shared with the control rather than caused by
// the shape: the quoted text starts at the `$(` where zsh starts at `v=$(`,
// and an unterminated `<(` gets a sentence of its own where zsh words it as
// the same parse error. Neither is what this case is about, and the corpus
// rows are what grade them.
func TestADelimiterCarryingTheClosingParenLeavesTheConstructOpen(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		line            int
		control         string
		controlWant     string
		controlLine     int
	}{
		{
			name: "the delimiter carries the paren",
			src:  "v=$(cat <<EOF\na\nEOF)\necho x\n",
			want: "parse error near `$(cat <<EOF'", line: 5,
			control:     "v=$(echo hi\necho x\n",
			controlWant: "parse error near `$(echo hi'", controlLine: 3,
		},
		{
			name: "a second here-document carries it",
			src:  "v=$(cat <<A\nx\nA\ncat <<B\ny\nB)\necho x\n",
			want: "parse error near `$(cat <<A'", line: 8,
			control:     "v=$(cat <<A\nx\nA\ncat <<B\ny\n",
			controlWant: "parse error near `$(cat <<A'", controlLine: 6,
		},
		{
			name: "parentheses that hold a program without a dollar sign",
			src:  "cat <(cat <<EOF\na\nEOF)\necho done\n",
			want: "unterminated process substitution", line: 0,
			control:     "cat <(echo hi\necho done\n",
			controlWant: "unterminated process substitution", controlLine: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := zsh.Diagnostics()
			err := refuses(t, c.src)
			if got := d.ParseFailure(err); got != c.want {
				t.Errorf("said %q, want %q", got, c.want)
			}
			// The line is the one the input ran out on, measured against
			// zsh 5.9.2 for each of these. Located at the `)` instead it is
			// three lines early on the first. The `<( )` row has no line at
			// all, which is this shell's own gap and is the control's too.
			if got := d.ParseFailureLine(err); got != c.line {
				t.Errorf("blamed line %d, want %d", got, c.line)
			}
			cerr := refuses(t, c.control)
			if got := d.ParseFailure(cerr); got != c.controlWant {
				t.Errorf("the control said %q, want %q", got, c.controlWant)
			}
			if got := d.ParseFailureLine(cerr); got != c.controlLine {
				t.Errorf("the control blamed line %d, want %d", got, c.controlLine)
			}
			// And it is the same failure underneath, which is what says no
			// new wording was needed: the kind and the token are what pick
			// the sentence, and both shapes pick the same one.
			if g, w := kindAndToken(t, err), kindAndToken(t, cerr); g != w {
				t.Errorf("the state behind the message is %v, where a plainly unterminated construct is %v", g, w)
			}
		})
	}
}

// The controls this shell still reads, so the refusal is about the shape and
// not about here-documents inside parentheses.
func TestWhatThisShellStillReadsBesideThatShape(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"the delimiter is alone on its line", "v=$(cat <<EOF\na\nEOF\n)\necho \"[$v]\"\n"},
		{"backquotes cannot be taken into a body", "v=`cat <<E\nq\nE`\necho \"[$v]\"\n"},
		{"a process substitution with no here-document", "cat <(echo hi)\n"},
		{"an arithmetic substitution, where `<<` is a shift", "echo $((\n4 << 2\n))\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := syntax.Parse(c.src, zsh.Dialect()); err != nil {
				t.Errorf("%q: %v", c.src, err)
			}
		})
	}
}

func refuses(t *testing.T, src string) error {
	t.Helper()
	_, err := syntax.Parse(src, zsh.Dialect())
	if err == nil {
		t.Fatalf("%q parsed; this shell refuses it", src)
	}
	return err
}

// kindAndToken is what the wording is chosen by, for the failures that carry
// it. A failure with no such state answers the same way on both sides.
func kindAndToken(t *testing.T, err error) [2]any {
	t.Helper()
	var se *syntax.Error
	if !errors.As(err, &se) {
		return [2]any{nil, nil}
	}
	return [2]any{se.Kind, se.Token}
}
