// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Verifying an expansion, through a real terminal.
//
// It has to be a terminal and it cannot be anything else. The whole of this
// option is what happens to a line **between** two reads: the text is drawn
// back where it was typed, the cursor is at its end, and the person decides.
// A reader-driven test holds the editor's input and would grade a seeded field
// rather than a redrawn line, which is the failure mode this repository keeps
// finding — a rendered surface asserted through a fake underneath it.
//
// The rows are the measured ones. 2026-09-19 through a pseudo-terminal against
// bash 5.3.20 with `--norc --noprofile -i` and `shopt -s histverify`, and the
// same rows against zsh 5.9.2 under `setopt HIST_VERIFY`:
//
//	echo AAA                    AAA
//	!!                          a prompt reading `echo AAA`, nothing run
//	 BBB and Return             AAA BBB
//	the list                    `echo AAA`, `echo AAA BBB` — and no third
//	!! then ^C                  nothing run, and the list gains nothing
//
// The fourth and fifth rows are the correction this carries. The issue asking
// for the option said the expanded line still joins the history list the way
// `:p` puts it there. Measured, it does not: the list holds what was
// **accepted**, once, and an abandoned verification leaves it exactly as it
// was. So the option records nothing of its own, and the ordinary accept below
// it is the whole of the recording.
func TestAVerifiedExpansionIsDrawnBackOnTheLine(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.SetHistoryExpansion(true)
		sh.Runner.SetHistoryExpansionVerifies(true)
	})

	s.typeLine("echo ONE\n")
	waitFor(t, s.ran, "ONE", "the first command")

	// `!!` runs nothing. The expansion is drawn where it was typed, at the
	// same prompt — the command number has not moved, because no command has.
	s.typeLine("!!\n")
	waitForLine(t, s.screen, "[2]echo ONE", "the expansion drawn back on the line")

	// The cursor is at its end, which is what makes typing append rather than
	// insert. This is the assertion a seeded buffer with a cursor left at 0
	// would fail and a containment check would not.
	s.typeKeys(" TWO\n")
	waitFor(t, s.ran, "ONE TWO", "the verified line, once it was accepted")

	// A second expansion, abandoned this time, and deliberately a line no
	// earlier one has: `echo echo ONE TWO ZZZ` is not in the list and not
	// anything that ran, so Up below can tell whether it was recorded.
	s.typeLine("echo !! ZZZ\n")
	waitForLine(t, s.screen, "[3]echo echo ONE TWO ZZZ", "the second expansion drawn back")
	s.typeKeys("\x03")
	waitForLine(t, s.screen, "[3]", "an empty prompt after the interrupt")

	// The list gained nothing from it: Up recalls the last line that was
	// *accepted*. A shell that remembered the verification the way `:p`
	// remembers one would put `echo echo ONE TWO ZZZ` here.
	s.typeKeys("\x1b[A")
	waitForLine(t, s.screen, "[3]echo ONE TWO", "the newest entry in the list")
	s.typeKeys("\x03")

	// One more command, so that the session's last prompt is one this fixture
	// has not counted twice: the counter advances per line typed and the
	// command number advances per command *run*, and the four lines above ran
	// two commands between them.
	s.typeKeys("echo THREE\n")
	waitFor(t, s.ran, "THREE", "the session still taking commands")
	s.end()

	// And nothing ran twice. The intermediate expansion reaching a command
	// would print `ONE` a second time with no `TWO` after it, which the waits
	// above cannot see because each is satisfied by the text before it.
	if got := s.ran.String(); got != "ONE\nONE TWO\nTHREE\n" {
		t.Errorf("the commands printed %q, want the accepted lines only", got)
	}
}

// With the option off the same keystrokes run the expansion instead, which is
// the half that says the option is doing the work.
//
// The pair is the point. A test of the on state alone passes against a shell
// that never runs an expanded line at all, and one of the off state alone
// passes against a shell that has no option. Measured on bash 5.3.20 without
// `shopt -s histverify`: `echo ONE` then `!!` writes `echo ONE` to standard
// error and prints `ONE` a second time, with no prompt in between.
func TestWithoutTheOptionAnExpansionRunsAtOnce(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.SetHistoryExpansion(true)
	})
	s.typeLine("echo ONE\n")
	waitFor(t, s.ran, "ONE", "the first command")
	s.typeLine("!!\n")
	waitFor(t, s.ran, "ONE", "the expansion run without a second look")
	s.end()

	// The echo to standard error is the off state's own half: the person sees
	// what is about to run because there is no line to read it on. Written
	// with a carriage return because a session writes to a terminal — see
	// translating, which is the same reason a command's newline is one.
	if got := s.errs.String(); !strings.Contains(got, "echo ONE\r\n") {
		t.Errorf("the session said %q, want the expansion echoed", got)
	}
}

// A verification inside a half-typed construct keeps the construct.
//
// This is what separates the new outcome from the one beside it. A reference
// nothing matched abandons whatever was being typed — measured, bash draws a
// fresh prompt rather than a continuation one — and a verified line does the
// opposite: measured 2026-09-19 on bash 5.3.20, `for i in 1` then `do !!` is
// redrawn at `> ` with the whole physical line expanded, and `done` after it
// closes a loop that runs.
func TestAVerifiedLineKeepsTheConstructInHand(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.SetHistoryExpansion(true)
		sh.Runner.SetHistoryExpansionVerifies(true)
	})
	s.typeLine("echo BODY\n")
	waitFor(t, s.ran, "BODY", "the first command")

	s.typeLine("for i in 1\n")
	s.typeKeys("do !!\n")
	// The continuation prompt, not a fresh one, and the whole physical line
	// expanded on it — the `do` included, because the expander is handed the
	// line rather than the word.
	waitForLine(t, s.screen, "> do echo BODY", "the expansion at the continuation prompt")
	s.typeKeys("\n")
	s.typeKeys("done\n")
	waitFor(t, s.ran, "BODY", "the loop that was still being typed")
	s.end()
}
