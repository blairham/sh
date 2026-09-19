// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A command asking for the editor, on a real terminal, in a real session.
//
// lineread_test.go grades what the editor does with the request. This grades
// the half a reader-driven test structurally cannot reach: **the terminal is
// in its own line discipline while a command runs**, and a read from inside
// one has to take raw mode again and hand it straight back. A stub editor and
// a reader would pass with none of that written.
//
// **Two rows of prompt, deliberately**, for completelistpty_test.go's reason:
// components can each be right while the nesting is broken, and a one-row
// `PS1` cannot see it (#2467, #3222).
func TestACommandCanAskForTheEditor(t *testing.T) {
	var got string
	var end interp.LineEditEnd
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		sh.Runner.Register("edit", func(r *interp.Runner, _ context.Context, args []string) int {
			got, end = r.EditLine(interp.LineEdit{Prompt: "P ", Initial: "world"})
			_, _ = fmt.Fprintf(r.Stdout, "got=[%s]\n", got)
			return 0
		})
	})

	s.typeLine("edit\n")
	waitFor(t, s.screen, "P world", "the value drawn in the line")
	// Row 0 is still the upper prompt row: the read drew where the line was
	// and not over what was above it.
	if row := s.row(0); row != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", row)
	}

	// **Typing without a newline is what proves raw mode**, and `^A` alone
	// does not: in the terminal's own line discipline the kernel buffers the
	// whole line and hands it over at the newline, so an editor reading it
	// then would still act on the `^A` and still produce the same line. What
	// only raw mode can do is *draw* between keystrokes. So the assertion is
	// that `X` inserted at the front of the text reaches the screen with no
	// newline typed at all — in the command's own line discipline nothing has
	// been delivered yet and the screen would still read `P world`.
	s.typeKeys("\x01X")
	waitFor(t, s.screen, "Xworld", "the insertion drawn before any newline")
	s.typeKeys("\n")
	waitFor(t, s.ran, "got=[Xworld]", "the line the command was given back")
	if end != interp.LineEditAccepted {
		t.Errorf("the read ended as %d, want accepted", end)
	}

	// That the terminal is handed *back* cannot be seen from here — a
	// command's output in this fixture does not go to the terminal at all —
	// so it is asked of the terminal directly in linereadmode_test.go.

	// And the session carries on: the terminal went back to the command's own
	// discipline, the command finished, and the next prompt takes a line.
	s.typeLine("echo after-$((6 * 7))-ok\n")
	waitFor(t, s.ran, "after-42-ok", "the command after the edit")
	s.end()
}

// An abandoned read is reported as an interrupt and the command is told, with
// the session still usable afterwards.
func TestACommandIsToldTheEditWasAbandoned(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Register("edit", func(r *interp.Runner, _ context.Context, args []string) int {
			line, end := r.EditLine(interp.LineEdit{Initial: "keepme"})
			_, _ = fmt.Fprintf(r.Stdout, "end=%d line=[%s]\n", end, line)
			return 0
		})
	})
	s.typeLine("edit\n")
	waitFor(t, s.screen, "keepme", "the value drawn in the line")
	s.typeKeys("\x03")
	waitFor(t, s.ran, fmt.Sprintf("end=%d line=[]", interp.LineEditInterrupted),
		"the interrupt reported to the command")
	// The session survived the interrupt, which is the half that would break
	// if the terminal were left in raw mode or left restored.
	s.typeLine("echo still-$((6 * 7))-here\n")
	waitFor(t, s.ran, "still-42-here", "the command after the interrupt")
	s.end()
}

// The listing a completion draws inside such a read reaches the terminal too,
// which is the case the refusal this replaced was argued from: a read that may
// stop to ask about a long listing was said to be unreachable from a command.
//
// It is reachable, and the assertion is the drawn row rather than the line —
// a completion that inserted without drawing would leave the same line behind.
func TestACompletionInsideSuchAReadDraws(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		sh.Completers = []Completer{CompleterFunc(func(c Completion) []Candidate {
			var got []string
			for _, m := range []string{"uniq_alpha", "uniq_beta"} {
				if strings.HasPrefix(m, c.Word) {
					got = append(got, m)
				}
			}
			return Words(got...)
		})}
		sh.Runner.Register("edit", func(r *interp.Runner, _ context.Context, args []string) int {
			line, _ := r.EditLine(interp.LineEdit{Initial: ": uniq"})
			_, _ = fmt.Fprintf(r.Stdout, "got=[%s]\n", line)
			return 0
		})
	})
	s.typeLine("edit\n")
	waitFor(t, s.screen, ": uniq", "the value drawn in the line")
	// Two Tabs: the first fills in what the two matches agree on and the
	// second lists them.
	s.typeKeys("\t\t")
	waitFor(t, s.screen, "uniq_beta", "the listing")
	s.typeKeys("\n")
	waitFor(t, s.ran, "got=[: uniq_]", "the completed line")
	s.end()
}
