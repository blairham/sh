// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// What a widget prints returns the carriage at every newline, on a real
// terminal. Measured 2026-10-02 against zsh 5.9.2 under `zsh -f -i` on a
// pseudo-terminal: `w() { print one; print two; }` bound to a key writes
// `one\r\ntwo\r\n`. Before #5500 this shell wrote `one\ntwo\n` — raw mode has
// OPOST off — and each line started where the one before it ended.
//
// The rows are computed rather than typed, so the wait cannot be satisfied
// by the terminal echoing the line that defines the widget.
func TestWhatAWidgetPrintsReturnsTheCarriage(t *testing.T) {
	control, screen := widgetSession(t)
	widgetType(t, control, screen, `pw() { print -r -- A$((6*7)); print -r -- B$((6*7)); }`)
	widgetType(t, control, screen, "zle -N pw")
	widgetType(t, control, screen, `bindkey '^G' pw`)
	if _, err := control.WriteString("\a"); err != nil {
		t.Fatalf("pressing ^G: %v", err)
	}
	const want = "A42\r\nB42\r\n"
	if err := screen.Await(want, widgetBudget); err != nil {
		t.Fatalf("the widget's two rows are not %q: %v\n%q", want, err,
			smoke.LastLines(screen.Text(), 6))
	}
}
