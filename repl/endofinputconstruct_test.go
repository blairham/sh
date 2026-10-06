// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// constructAtEndSession runs a session on a pipe holding text, and answers what it
// wrote to its output and to its diagnostics.
func constructAtEndSession(t *testing.T, text string, ends bool) (ran, said string) {
	t.Helper()
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
	r.Stdout = &out
	s := Shell{
		Runner:                               r,
		In:                                   strings.NewReader(text),
		Out:                                  &out,
		Err:                                  &errs,
		Leaving:                              "exit",
		EndOfInputInAConstructEndsTheSession: ends,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	return out.String(), errs.String()
}

// Every shell in the panel but bash, when the end of input meets a construct
// still being typed, runs or refuses it and goes round for one more prompt;
// on a pipe that prompt meets the end again, and that is what ends the
// session. Measured 2026-10-06 under `-i` on a pipe with `PS1='P> '`: zsh
// 5.9.2, dash, ksh93u+ and busybox ash all write `P> ` once more after the
// output in each row below (#6263). See
// interp.Semantics.EndOfInputInAConstructEndsTheSession.
func TestTheEndOfInputInAConstructGoesRoundOnceMore(t *testing.T) {
	for _, c := range []struct{ name, text, ran, said string }{
		{"a here-document", "cat <<EOF\nhi\n", "hi\n", "$ > > $ exit\n"},
		{"a line continued", "echo one \\\n", "one\n", "$ > $ exit\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ran, said := constructAtEndSession(t, c.text, false)
			if ran != c.ran || said != c.said {
				t.Errorf("output %q and diagnostics %q, want %q and %q", ran, said, c.ran, c.said)
			}
		})
	}
	t.Run("a construct refused", func(t *testing.T) {
		_, said := constructAtEndSession(t, "if true; then\n", false)
		if !strings.HasPrefix(said, "$ > ") || !strings.HasSuffix(said, "\n$ exit\n") {
			t.Errorf("diagnostics %q, want the refusal and then one more prompt", said)
		}
	})
}

// bash ends the session instead — without its word when the construct ran,
// with it when it was refused — and goes round only after a here-document the
// input ran out inside. Measured 2026-10-06 on bash 5.3.20 under `--norc -i`
// on a pipe and through a pseudo-terminal alike (#6263).
func TestTheEndOfInputInAConstructEndsABashSession(t *testing.T) {
	for _, c := range []struct{ name, text, ran, said string }{
		{"a here-document goes round", "cat <<EOF\nhi\n", "hi\n", "$ > > $ exit\n"},
		{"a line continued leaves without the word", "echo one \\\n", "one\n", "$ > "},
		{"a here-document beside a line continued goes round", "cat <<EOF; echo x \\\n", "x\n", "$ > $ exit\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			ran, said := constructAtEndSession(t, c.text, true)
			if ran != c.ran || said != c.said {
				t.Errorf("output %q and diagnostics %q, want %q and %q", ran, said, c.ran, c.said)
			}
		})
	}
	t.Run("a construct refused leaves with the word", func(t *testing.T) {
		_, said := constructAtEndSession(t, "if true; then\n", true)
		if !strings.HasPrefix(said, "$ > ") || !strings.HasSuffix(said, "\nexit\n") || strings.Contains(said, "\n$ ") {
			t.Errorf("diagnostics %q, want the refusal and then the word, with no prompt between", said)
		}
	})
}
