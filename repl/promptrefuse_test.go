// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A construct the parser has refused is refused at the prompt, rather than
// asked about again.
//
// `if; then` is unfinished and wrong at once: the `if` is open, so more input
// is possible, and the `;` after the keyword is a token the grammar will never
// take, so no amount of it would help. A prompt that asks anyway reads the
// *next* command as part of the construct it has already refused, so the
// command a person typed disappears — the third line here is `echo three`, and
// it has to run (#1893).
//
// Measured 2026-09-11 with the same three lines into each shell under `-i`:
// bash 5.3.15, ksh93u+ and dash all refuse without drawing a continuation
// prompt and then run `echo three`; zsh 5.9.2 draws PS2 and waits.
func TestAConstructThatCanNeverFinishIsRefusedAtOnce(t *testing.T) {
	const typed = "echo one\nif; then\necho three\n"
	for _, c := range []struct {
		name      string
		ask       bool
		out, errs string
		status    int
	}{
		{
			name:   "refused where it stands",
			out:    "one\nthree\n",
			errs:   "P1> P1> testsh: refused\nP1> P1> ",
			status: 0,
		},
		{
			// The other answer, which one shell gives: a second continuation
			// prompt, the third line swallowed, and the refusal only at the
			// end of the input.
			name:   "or asked about again",
			ask:    true,
			out:    "one\n",
			errs:   "P1> P1> P2> testsh: refused\nP1> ",
			status: 7,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, st := refusalSession(t, typed, c.ask)
			if out != c.out {
				t.Errorf("output = %q, want %q", out, c.out)
			}
			if errs != c.errs {
				t.Errorf("prompts and complaint = %q, want %q", errs, c.errs)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// Which failures those are: a token the grammar cannot take, and never input
// that has merely not finished.
//
// The second list is the half that makes the first safe. Every line in it is
// incomplete too, and every shell in the panel prompts for all of them —
// including `for do` and `case in`, which are *wrong* and are still refused
// only once the next line arrives, because what is missing is a word rather
// than a word being in the way.
func TestOnlyARefusedTokenSkipsTheContinuationPrompt(t *testing.T) {
	for _, src := range []string{"if; then\n", "while; do\n"} {
		if _, _, _, ready := refusalAccept(t, src, false); !ready {
			t.Errorf("%q asked for another line, where three of the panel refuse it", src)
		}
	}
	for _, src := range []string{
		"if true; then\n",
		"for do\n",
		"case in\n",
		"echo one |\n",
		"echo one &&\n",
		"x='never closed\n",
		"cat <<EOT\n",
		"{\n",
		"echo one \\\n",
	} {
		if _, _, _, ready := refusalAccept(t, src, false); ready {
			t.Errorf("%q was refused, where the whole panel asks for another line", src)
		}
	}
	// And with the other answer even the first list waits.
	for _, src := range []string{"if; then\n", "while; do\n"} {
		if _, _, _, ready := refusalAccept(t, src, true); ready {
			t.Errorf("%q was refused, where the shell that continues waits", src)
		}
	}
}

// A line that is whole and wrong is unaffected: it was already refused where
// it stood, under either answer, because nothing about it is unfinished.
func TestALineThatIsWholeAndWrongIsRefusedUnderBothAnswers(t *testing.T) {
	for _, ask := range []bool{false, true} {
		stmts, _, err, ready := refusalAccept(t, "echo )\n", ask)
		if !ready || err == nil || len(stmts) != 0 {
			t.Errorf("ask=%v: ready=%v err=%v stmts=%d, want a refusal at once", ask, ready, err, len(stmts))
		}
	}
}

// refusalAccept runs one line through accept with the dialect's answer set.
func refusalAccept(t *testing.T, line string, ask bool) ([]*syntax.File, string, error, bool) {
	t.Helper()
	var pending strings.Builder
	s := Shell{AskAgainAfterARefusedToken: ask}
	return s.accept(&pending, nil, strings.TrimSuffix(line, "\n"))
}

// refusalSession is endOfInputSession with the dialect's answer set and the
// prompts left visible, since where a continuation prompt is drawn is half of
// what this file is about.
func refusalSession(t *testing.T, text string, ask bool) (out, errs string, status int) {
	t.Helper()
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "P1> ", "PS2": "P2> ", "PATH": "/bin:/usr/bin"})
	r.Stdout = &ran
	s := Shell{
		Runner:                     r,
		In:                         strings.NewReader(text),
		Out:                        &ran,
		Err:                        &said,
		Report:                     func(error) string { return "testsh: refused\n" },
		ParseFailureStatus:         func(error) int { return 7 },
		AskAgainAfterARefusedToken: ask,
	}
	st, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("run %q: %v", text, err)
	}
	return ran.String(), said.String(), st
}
