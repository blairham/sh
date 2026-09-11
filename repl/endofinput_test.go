// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Input that ends while a construct is still open is read as the whole of the
// input, because there is no other line coming. It is then either a command or
// a failure, and both halves are measured and unanimous across the panel.
//
// It was neither. The pending text was dropped without a word, so a person who
// typed a quote by accident and pressed ^D at the continuation prompt was told
// nothing at all about why their line had vanished — and `$?` said the session
// had succeeded (#1467).
func TestInputThatEndsWithAConstructStillOpen(t *testing.T) {
	for _, c := range []struct {
		name, text, out string
		refused         bool
		status          int
	}{
		{
			// The issue's own shape: a quote nobody closed.
			name:    "an unclosed quote is a failure",
			text:    "echo one\nx='never closed\n",
			out:     "one\n",
			refused: true,
			status:  7,
		},
		{
			name:    "and so is a construct nobody finished",
			text:    "if true; then\n",
			refused: true,
			status:  7,
		},
		{
			// The other half, and the reason this is not simply "report the
			// pending text": a trailing backslash at end of input joins the
			// line to nothing, and every shell in the panel runs it.
			name:   "a trailing backslash is a command",
			text:   "echo one \\\n",
			out:    "one\n",
			status: 0,
		},
		{
			// Same again for a here-document whose delimiter never arrived:
			// the body ends where the input does and the command runs.
			name:   "a here-document delimited by the end of input is one too",
			text:   "cat <<EOT\nbody\n",
			out:    "body\n",
			status: 0,
		},
		{
			// Nothing pending, which is the ordinary way a session ends and
			// must stay silent.
			name:   "input that ends between commands says nothing",
			text:   "echo one\n",
			out:    "one\n",
			status: 0,
		},
		{
			// Whitespace is not a construct: a blank continuation line left
			// over must not be reported as one.
			name:   "and neither is blank text",
			text:   "echo one\n   \n",
			out:    "one\n",
			status: 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := endOfInputSession(t, c.text)
			if out != c.out {
				t.Errorf("output = %q, want %q", out, c.out)
			}
			said := errs == "testsh: refused\n"
			if said != c.refused {
				t.Errorf("stderr = %q, want refused=%v", errs, c.refused)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// The text is parsed with the session's own grammar, so a construct half of
// which was typed finishes here exactly as it would have on the next line —
// rather than through whatever a fresh parser would have defaulted to.
func TestTheUnfinishedTextKeepsTheSessionsGrammar(t *testing.T) {
	const text = "[[ a = a ]] && echo yes \\\n"
	for _, c := range []struct {
		name    string
		enabled bool
		want    string
	}{
		{"the grammar has the construct", true, "yes\n"},
		{"and without it the same text is refused", false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var ran, said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
			r.Stdout = &ran
			s := Shell{
				Runner:             r,
				Dialect:            syntax.Dialect{DoubleBracket: c.enabled},
				In:                 strings.NewReader(text),
				Out:                &ran,
				Err:                &said,
				Report:             func(error) string { return "testsh: refused\n" },
				ParseFailureStatus: func(error) int { return 7 },
			}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if ran.String() != c.want {
				t.Errorf("output = %q, want %q", ran.String(), c.want)
			}
		})
	}
}

// endOfInputSession runs text through the session that has no editor, which is
// the loop `-i` on a pipe uses and the only one a test can drive.
//
// The status a refusal leaves is 7, which nothing else here produces, so a
// cell holding it can only have come from the refusal.
func endOfInputSession(t *testing.T, text string) (out, errs string, status int) {
	t.Helper()
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": "", "PATH": "/bin:/usr/bin"})
	r.Stdout = &ran
	s := Shell{
		Runner:             r,
		In:                 strings.NewReader(text),
		Out:                &ran,
		Err:                &said,
		Report:             func(error) string { return "testsh: refused\n" },
		ParseFailureStatus: func(error) int { return 7 },
	}
	st, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("run %q: %v", text, err)
	}
	return ran.String(), said.String(), st
}
