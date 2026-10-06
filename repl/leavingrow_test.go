// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
	"time"
)

// Where the word a session leaves with lands, with a terminal on the other
// end of it.
//
// The piped loop has always been right — TestASessionWritesTheDialectsWordForLeaving
// grades that one, and its `$ $ exit\n` has the word on the last prompt's row
// because the loop writing the prompt is the loop writing the word. The
// editor's loop was not, and nothing here could see it: every file in the
// suite runs without a terminal, and the two reader-driven routes never reach
// [editor.stopped] at all. So this is the only instrument that can fail for
// #4011, and the assertion is on the **screen** rather than on the bytes —
// what was wrong was a row, and a row is not something a byte comparison
// names.
//
// Measured 2026-09-21 through a pseudo-terminal, `env -i`, `PS1='P> '`, ^D:
// bash 5.3.20 writes `P> exit\r\n`. Two prompts rather than one, because a
// prompt whose last row is its second is where a fix that merely moved the
// write up a line would come apart: the word belongs on the row the *cursor*
// is on, not on the row the prompt started on.
func TestTheWordForLeavingLandsOnTheRowTheLastPromptIsOn(t *testing.T) {
	for _, c := range []struct {
		name, ps1, leaving, want string
		paste                    bool
	}{
		{"a prompt on one row", "P> ", "exit", "P> exit", false},
		// A literal newline in the value, which is what a two-row prompt is
		// once the dialect's escapes have been read — there are none in this
		// session's Style, so the value is drawn as it stands.
		{"a prompt whose last row is its second", "top\nP> ", "exit", "top\nP> exit", false},
		// And four of the five dialects have no word. The row is still ended:
		// measured the same day, dash and ksh93 end it on ^D and so does
		// zsh, which has an editor and nothing to say.
		{"a dialect with no word still ends the row", "P> ", "", "P>", false},
		// With bracketed paste on, which is bash's default at a terminal,
		// the row is not the prompt's (#6243). Measured 2026-10-06 with
		// `TERM=xterm` and the two-row prompt: bash 5.3.20 writes
		// `\e[?2004l\r\r\nexit\r\n`, and `exit\r\n` once its rc turns the
		// bracketing off — which is the row above. See editor.takePasteOff.
		{"with bracketed paste on, the word is on a row of its own", "top\nP> ", "exit", "top\nP>\nexit", true},
		{"with bracketed paste on and no word, the row is still ended", "top\nP> ", "", "top\nP>", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			control, tty := openTerminal(t)
			// One buffer for both streams, because the composition is the
			// subject: the prompt is written to the output and the word to
			// the diagnostics, and a fixture that keeps them apart cannot
			// tell the two rows from one.
			screen := &syncBuffer{}
			r := newTestRunner(map[string]string{"PS1": c.ps1})
			r.Stdout, r.Stderr = screen, screen
			s := Shell{
				Runner: r, In: tty, Out: screen, Err: screen,
				Name: "sh", Leaving: c.leaving,
				Editor: EditorStyle{BracketedPaste: c.paste},
			}
			done := make(chan error, 1)
			go func() { _, err := s.Run(t.Context()); done <- err }()

			// The prompt has to be on the screen before the key is sent: the
			// shell takes the terminal into raw mode to read, and the kernel
			// drops what is queued but unread across that change (#635).
			waitFor(t, screen, "P> ", "the prompt")
			if _, err := control.WriteString("\x04"); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("the session did not end on ^D")
			}

			if c.paste && !strings.Contains(screen.String(), "\x1b[?2004l\r"+"\r\n") {
				// The bytes as well as the screen, for the case with no word:
				// the row it ends on looks the same either way round.
				t.Errorf("the bracketing is taken back after the row is "+
					"ended, where bash and zsh take it back first: %q",
					screen.String())
			}
			if got := shownBy(fixtureCols, screen.String()).text(); got != c.want {
				t.Errorf("the screen reads %q, want %q: the word a session "+
					"leaves with goes on the row the last prompt is on, not "+
					"on a row of its own below it (#4011). What was written "+
					"was %q", got, c.want, screen.String())
			}
		})
	}
}
