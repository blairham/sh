// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// The status a refused line leaves behind, through a real terminal.
//
// The two loops are separate code — an editor on one end and a reader on the
// other — and this is the one a person types at, so a refusal wired into only
// the reader's loop would be a fix nobody using the shell ever sees. That is
// the shape #1299 was found in: the prompt hooks are handed `$?`, and they are
// only ever handed it here.
//
// The prompt is waited on by *number*, and a refused line does not advance it:
// nothing ran, so there is no command to count. Waiting for the same number a
// second time is therefore waiting for the prompt drawn after the refusal,
// which is what the buffer's cursor is for.
//
// Two runs rather than one, with the command before the refusal leaving a
// different status each time and the dialect answering a different one: a
// single run cannot tell a status that was *set* from one that happened to
// match what was already there, and that is the confound this whole issue is.
func TestARefusedLineSetsTheStatusAtATerminal(t *testing.T) {
	for _, c := range []struct {
		name    string
		before  string
		answers int
		want    string
	}{
		{"after a command that failed", "false", 7, "M7E"},
		{"after a command that succeeded", "true", 9, "M9E"},
		{"after a status the answer is not", "(exit 7)", 9, "M9E"},
	} {
		t.Run(c.name, func(t *testing.T) {
			answers := c.answers
			se := newSessionWith(t, func(s *Shell) {
				s.Report = func(error) string { return "testsh: refused\n" }
				s.ParseFailureStatus = func(error) int { return answers }
			})

			waitFor(t, se.screen, "[1]", "the first prompt")
			if _, err := se.control.WriteString(c.before + "\r"); err != nil {
				t.Fatal(err)
			}

			waitFor(t, se.screen, "[2]", "the prompt after the first command")
			if _, err := se.control.WriteString("fi\r"); err != nil {
				t.Fatal(err)
			}
			// The diagnostic is the mark that the refusal has been reported,
			// and it is the whole rendered line rather than a piece of one —
			// carriage return included, because the terminal is in its own
			// line discipline when the complaint is written and that is what
			// a person sees.
			waitFor(t, se.errs, "testsh: refused\r\n", "the complaint about the refused line")

			// Still [2]: the refused line was not counted, which is what the
			// second wait for the same number relies on.
			waitFor(t, se.screen, "[2]", "the prompt after the refused line")
			if _, err := se.control.WriteString("printf 'M%sE\\n' \"$?\"\r"); err != nil {
				t.Fatal(err)
			}
			waitFor(t, se.ran, c.want, "the status the refusal left behind")

			se.prompt = 2
			se.end()
		})
	}
}
