// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// dash, ksh93 and BusyBox ash end the last prompt's line with a newline when
// the input runs out, and not when `exit` ends the session. Measured
// 2026-10-07 on a pipe (#6330). See
// interp.Semantics.PromptEndOfInputEndsTheLine.
func TestTheEndOfInputEndsThePromptsLine(t *testing.T) {
	for _, c := range []struct {
		name string
		ends bool
		in   string
		errs string
	}{
		{"the end of the input ends the line", true, "echo A\n", "P> P> \n"},
		{"so does input with nothing in it", true, "", "P> \n"},
		{"exit does not", true, "exit\n", "P> "},
		{"nor an exit the input ends on", true, "exit", "P> "},
		// The end of the input finishing the command that exits: a
		// here-document's body. dash and ksh93 write no newline there either.
		{"nor an exit the end of the input ran", true, "exit <<E\nx\n", "P> > > "},
		{"and the other dialects write nothing", false, "echo A\n", "P> P> "},
	} {
		t.Run(c.name, func(t *testing.T) {
			var said strings.Builder
			r := newTestRunner(map[string]string{"PS1": "P> ", "PS2": "> "})
			r.Stdout = io.Discard
			s := Shell{Runner: r, In: strings.NewReader(c.in), Out: io.Discard, Err: &said, EndOfInputEndsTheLine: c.ends}
			if _, err := s.Run(t.Context()); err != nil {
				t.Fatal(err)
			}
			if said.String() != c.errs {
				t.Errorf("stderr %q, want %q", said.String(), c.errs)
			}
		})
	}
}
