// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/smoke"
)

// An **asynchronous** suggestion, drawn at a real prompt — which is the route
// zsh-autosuggestions takes by default and the one a plugin manager configures
// (#4413).
//
// The plugin has two fetch paths and `_zsh_autosuggest_fetch` chooses between
// them on `${+ZSH_AUTOSUGGEST_USE_ASYNC}` alone. The synchronous one runs
// inside the keystroke's own widget and was right; the asynchronous one is
// this shape, and it drew nothing at all:
//
//	builtin exec {FD}< <( … print the suggestion … )
//	read pid <&$FD
//	zle -F $FD handler          # a plain handler, no -w
//	handler() { read …; zle a-widget }   # the widget sets POSTDISPLAY
//
// Note what the plugin actually uses, because the issue guessed otherwise
// from the older release: there is **no `zpty`** on this path. The worker is
// an ordinary process substitution whose descriptor `exec` parks in the
// shell's own table, and the asynchrony is `zle -F` over it.
//
// Three separate things had to be true for a character of it to appear, and
// none of them was:
//
//   - a widget invoked by `zle` from a plain handler must be **given the
//     line**, or its `POSTDISPLAY` lands on a variable nothing reads back —
//     dialect/zsh/zlewatchwidget_test.go;
//   - what the handler left must come back to the editor as something to
//     draw, `Postdisplay` included — repl/watchfdpostdisplay_test.go;
//   - and the wait the suggestion arrives on must survive a signal, or the
//     editor abandons the descriptor and waits for a keystroke instead —
//     internal/fdset/waiteintr_test.go.
//
// A fourth had to be true for the suggestion to be *right* rather than
// merely present: the `--` in `zle a-widget -- "$suggestion"` is the marker
// ending the widget's option list, so a shell that passes it through hands
// the widget `--` as `$1` and draws that. This session writes the call the
// plugin's own spelling, so that row is covered here too — see
// widgetCallArgs.
//
// This is the one test that fails if any of the three regresses, which is
// why it is written end to end rather than as a fourth unit test.
//
// Measured 2026-09-24 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) driven
// through a pseudo-terminal with the same startup file and the same
// keystrokes: after `echo hel` and the key that arms the fetch, the screen
// holds `echo hello-world` and the cursor is moved back over the eight cells
// of the suggestion. This shell now writes the same.

// asyncMark is the last row of the prompt this session synchronizes on, and
// it is not text anybody types — see widgetMark for why that matters.
const asyncMark = "as> "

// asyncBudget turns a hang into a failure.
const asyncBudget = 20 * time.Second

func TestAnAsynchronousSuggestionIsDrawnAtThePrompt(t *testing.T) {
	control, screen := asyncSession(t)

	// `echo hel` is typed a character at a time the way a person types it,
	// and then one key arms the fetch. The suggestion arrives 200ms later,
	// from a process this shell started, on a descriptor nothing is waiting
	// for except the editor.
	if _, err := control.WriteString("echo hel"); err != nil {
		t.Fatalf("typing the line: %v", err)
	}
	if err := screen.Await("echo hel", asyncBudget); err != nil {
		t.Fatalf("the line was not drawn: %v", err)
	}
	if _, err := control.WriteString("\a"); err != nil {
		t.Fatalf("pressing ^G: %v", err)
	}

	// The whole assertion: the suggestion is drawn after the line, without a
	// keystroke to prompt the redraw. Before the fix this waited out the
	// budget with `echo hel` on the screen.
	//
	// The wait is for the suggestion's own text rather than for the line and
	// the suggestion together. What the terminal *renders* is
	// `as> echo hello-world`, but the two halves reach it as separate writes
	// — `echo hel` as it was typed, `lo-world` when the fetch answered — so
	// the whole phrase never appears contiguously in the byte stream a screen
	// accumulates, and waiting for it waits for ever against a working shell.
	if err := screen.Await("lo-world", asyncBudget); err != nil {
		t.Fatalf("the suggestion was never drawn:\n%s",
			smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}

	// And it is a postdisplay rather than part of the line: accepting runs
	// what was typed. A shell that had appended the suggestion to the buffer
	// would draw the same screen and run the wrong command.
	//
	// The comparison is scoped to what arrives *after* the accept, because
	// the drawn line above legitimately contains the suggestion — that is
	// what the test just required — and a search of the whole screen would
	// find it there.
	before := len(screen.Text())
	if _, err := control.WriteString("\n"); err != nil {
		t.Fatalf("accepting the line: %v", err)
	}
	if err := screen.Await(asyncMark, asyncBudget); err != nil {
		t.Fatalf("no prompt after the line was accepted:\n%s",
			smoke.Readable(smoke.LastLines(screen.Text(), 8)))
	}
	// `echo` writes its argument in one piece, so a buffer that had swallowed
	// the suggestion prints `hello-world` contiguously here — which is
	// exactly what the two halves above could not do on the drawn line, and
	// is what makes this check discriminating rather than vacuous.
	ran := screen.Text()[before:]
	if !strings.Contains(ran, "hel\r\n") {
		t.Errorf("the accepted line did not run as `echo hel`:\n%s", smoke.Readable(ran))
	}
	// `echo` writes its argument in one piece, so a buffer that had swallowed
	// the suggestion prints `hello-world` here.
	if strings.Contains(ran, "hello-world") {
		t.Errorf("the suggestion was run as part of the line:\n%s", smoke.Readable(ran))
	}
}

// asyncSession puts an interactive zsh on a pseudo-terminal with the plugin's
// async shape in its startup file, a two-row prompt and a scratch home.
//
// **Two rows and not one**, for the reason widgetSession gives: a prompt whose
// upper row is rewritten on every keystroke draws a ladder a one-row `PS1`
// cannot see (#2467).
func asyncSession(t *testing.T) (*os.File, *smoke.Screen) {
	t.Helper()
	home := scratchHome(t)
	control, terminal, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	if err := pty.SetSize(terminal, 24, 100); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	// Written the way the plugin writes it, down to the plain handler and the
	// widget it calls: the handler has no line and the widget has to be given
	// one. The worker prints a pid first, as the plugin's does, so that the
	// handler's read is the *second* thing to arrive on the descriptor and
	// the wait is a real one.
	writeHomeFile(t, home, ".zshrc", "PS1=$'ASROW\\n"+asyncMark+"'\n"+
		"suggest() { POSTDISPLAY=$1 }\n"+
		"zle -N suggest\n"+
		"response() {\n"+
		"  IFS='' read -rd '' -u $1 SUGGESTION\n"+
		"  zle suggest -- \"$SUGGESTION\"\n"+
		"  builtin exec {1}<&-\n"+
		"  zle -F $1\n"+
		"}\n"+
		"fetch() {\n"+
		"  builtin exec {ASYNCFD}< <(echo 4242; sleep 0.2; echo -nE 'lo-world')\n"+
		"  read pid <&$ASYNCFD\n"+
		"  zle -F $ASYNCFD response\n"+
		"}\n"+
		"zle -N fetch\n"+
		"bindkey '^G' fetch\n")

	sh := scratchShell(t)
	sh.Stdin, sh.Stdout, sh.Stderr = terminal, terminal, terminal
	sh.Dir = home
	sh.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TERM=dumb",
		"HISTFILE=" + filepath.Join(home, "hist"),
	}
	screen := smoke.Watch(control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"zsh", "-i"}) }()
	t.Cleanup(func() {
		// The shell first, so nothing is left reading a terminal this test is
		// about to close, and then both ends of it.
		_, _ = control.WriteString("exit\n")
		select {
		case <-done:
		case <-time.After(asyncBudget):
			t.Error("the shell did not exit")
		}
		_ = terminal.Close()
		_ = control.Close()
	})
	if err := screen.Await(asyncMark, asyncBudget); err != nil {
		t.Fatalf("no first prompt: %v", err)
	}
	return control, screen
}
