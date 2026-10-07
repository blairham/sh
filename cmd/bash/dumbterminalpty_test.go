// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/smoke"
	"github.com/blairham/sh/internal/terminfofixture"
)

// describedTerminal is the environment of a session on a terminal the
// terminfo database describes and that is not one of the names bash treats
// as unable to take a paste's markers. The database is written by the test
// rather than read from the machine, which may not have one.
func describedTerminal(t *testing.T) []string {
	t.Helper()
	db := terminfofixture.Database(t, terminfofixture.Description{Name: "fixtureterm"})
	return []string{"TERM=fixtureterm", "TERMINFO=" + db}
}

// On a dumb terminal bash asks for no paste markers, and ^D at a continuation
// prompt leaves the row as it is, so what is said next is written after the
// prompt. On a terminal the database describes it asks for them.
//
// Measured 2026-10-07 through a pseudo-terminal on bash 5.3.20, `--norc -i`:
// under `TERM=dumb`, `cat <<E1` / `a` / ^D draws `> bash: warning: …` on one
// row with no `\e[?2004h` anywhere; under xterm both markers are written and
// the sequence taking them back ends the row. Here the markers were asked for
// whatever the terminal was (#6310). See
// repl.EditorStyle.BracketedPasteOffForTerminals and
// EndOfInputWithNoWordStaysOnTheRow.
func TestADumbTerminalGetsNoPasteMarkers(t *testing.T) {
	t.Run("dumb", func(t *testing.T) {
		control, screen := interruptSession(t, "")
		if _, err := control.WriteString("cat <<E1\n"); err != nil {
			t.Fatal(err)
		}
		if err := screen.Await(interruptPS2, interruptBudget); err != nil {
			t.Fatal(err)
		}
		if _, err := control.WriteString("\x04"); err != nil {
			t.Fatal(err)
		}
		if err := screen.Await("(wanted `E1')", interruptBudget); err != nil {
			t.Fatal(err)
		}
		awaitInterruptPrompt(t, screen, "the ^D")
		drawn := screen.Text()
		if strings.Contains(drawn, "\x1b[?2004h") {
			t.Errorf("a dumb terminal was asked for paste markers:\n%s", smoke.Readable(drawn))
		}
		if !strings.Contains(drawn, interruptPS2+"bash: warning: here-document") {
			t.Errorf("^D at the continuation prompt ended the row:\n%s", smoke.Readable(drawn))
		}
	})
	t.Run("described", func(t *testing.T) {
		control, screen := interruptSessionWith(t, "", describedTerminal(t)...)
		from := len(screen.Text())
		interruptAnswer(t, control, screen, "true", nil)
		if !strings.Contains(screen.Text()[from:], "\x1b[?2004h") {
			t.Errorf("a described terminal was not asked for paste markers:\n%s", smoke.Readable(screen.Text()[from:]))
		}
	})
}

// Inside Emacs the session starts with no line editor, decided from the
// environment before the startup files on both routes an interactive shell
// has. Measured 2026-10-07 on bash 5.3.20: `TERM=emacs` lists `emacs off` at a
// pseudo-terminal and from `-i -c` with no terminal, and `set -o vi` in the
// startup file still selects vi. See
// interp.Semantics.InsideEmacsTurnsEditingOff (#6310).
func TestInsideEmacsTheSessionHasNoLineEditor(t *testing.T) {
	t.Run("at a terminal", func(t *testing.T) {
		control, screen := interruptSessionWith(t, "", "TERM=emacs")
		got := interruptAnswer(t, control, screen, `[[ -o emacs ]] && echo "st-1-" || echo "st-0-"`, interruptStatusLine)
		if got != "0-" {
			t.Errorf("emacs mode at a TERM=emacs prompt read %q, want off (0-)", got)
		}
	})
	for _, tc := range []struct {
		name, rc string
		env      []string
		want     string
	}{
		{"from -i -c", "", []string{"TERM=emacs"}, "emacs=off vi=off"},
		{"a dumb terminal outside Emacs", "", []string{"TERM=dumb"}, "emacs=on vi=off"},
		{"EMACS=t on a dumb terminal", "", []string{"TERM=dumb", "EMACS=t"}, "emacs=off vi=off"},
		{"the startup file comes after", "set -o vi\n", []string{"TERM=emacs"}, "emacs=off vi=on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(tc.rc), 0o600); err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &out, io.Discard
			sh.Stdin = strings.NewReader("")
			sh.Env = append([]string{"HOME=" + home, "PATH=/usr/bin:/bin"}, tc.env...)
			driver.MainArgs(sh, []string{
				"bash", "-i", "-c",
				`for o in emacs vi; do [[ -o $o ]] && s=on || s=off; printf '%s=%s ' $o $s; done`,
			})
			if got := strings.TrimSpace(out.String()); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
