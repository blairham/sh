// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// The pre-redraw widget reads the key it follows as `$LASTWIDGET`, a widget
// a `zle -F` handler calls reads the key before the handler, and the
// handler's change to the line is drawn with no pre-redraw after it.
//
// #5875. Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a
// pseudo-terminal with this startup file, typing `e`, ^F, then ^Xf and
// waiting for the descriptor:
//
//	pre W=[zle-line-pre-redraw] B=[e] LW=[self-insert]
//	pre W=[zle-line-pre-redraw] B=[e] LW=[forward-char]
//	pre W=[zle-line-pre-redraw] B=[e] LW=[arm]
//	handler
//	inner LW=[arm]
//
// and nothing after `inner` until the next key. Before the fix this shell
// logged `LW=[]` for the first line, the key before for every other, `inner
// LW=[]`, and a `pre … B=[ei]` after the handler.
func TestPreRedrawAndHandlerWidgetsReadTheKeyBefore(t *testing.T) {
	control, screen, home := jobNoticeSessionRC(t, `pre() { print -r -- "pre W=[$WIDGET] B=[$BUFFER] LW=[$LASTWIDGET]" >> $HOME/out }
zle -N zle-line-pre-redraw pre
inner() { BUFFER+=i; print -r -- "inner LW=[$LASTWIDGET]" >> $HOME/out }
zle -N inner
h() { local l; read -r l <&$1; zle -F $1; print -r -- handler >> $HOME/out; zle inner }
arm() { exec {fd}< <(sleep 0.3; echo x); zle -F $fd h }
zle -N arm; bindkey '^Xf' arm
`, "zsh", "-i")
	log := func() string {
		data, _ := os.ReadFile(filepath.Join(home, "out"))
		return string(data)
	}
	await := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(jobNoticeBudget); !ok(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s; the log is\n%s\nthe screen:\n%s", what, log(), smoke.Readable(smoke.LastLines(screen.Text(), 4)))
			}
		}
	}

	// One key at a time, each sent once the redraw before it has been
	// logged, so that no key is read in the same chunk as another.
	if _, err := control.WriteString("e"); err != nil {
		t.Fatal(err)
	}
	await("typing e logged no pre-redraw", func() bool { return strings.Contains(log(), "B=[e]") })
	if _, err := control.WriteString("\x18f"); err != nil {
		t.Fatal(err)
	}
	await("the handler's widget did not run", func() bool { return strings.Contains(log(), "inner LW=") })
	// And drawn: the line the handler's widget left is on the screen. Read
	// off the screen rather than the bytes, which redraw the first column
	// with the second (#6325).
	await("the handler's change was not drawn", func() bool {
		g := grid(screen)
		p := promptRow(g)
		return p >= 0 && strings.TrimRight(g.Text(p), " ") == jobNoticeMark+"ei"
	})
	// A quiet moment, so that a pre-redraw the handler's redraw asked for has
	// had time to arrive: the bug wrote it within milliseconds.
	time.Sleep(300 * time.Millisecond)

	want := "pre W=[zle-line-pre-redraw] B=[e] LW=[self-insert]\n" +
		"pre W=[zle-line-pre-redraw] B=[e] LW=[arm]\n" +
		"handler\n" +
		"inner LW=[arm]\n"
	if got := log(); got != want {
		t.Errorf("the log is\n%s\nwant\n%s", got, want)
	}
}
