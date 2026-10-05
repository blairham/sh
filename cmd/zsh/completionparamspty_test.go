// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A `zle -C` widget pressed on its key runs with the widget parameters, the
// line ones read-only, on a real terminal (#5999).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: `qzq` and ^T print `cf: read-only variable:
// BUFFER`, and ^Xs then draws `GOT[W=cw t=scalar-local-readonly-special|qzq|
// unset]` — the function saw its widget's name and a read-only line, stopped
// at the assignment, and the line was kept. Before the fix it drew `GOT[W=
// t=|qzq|widget]`.
//
// A word no command begins with, because a completion function that offers
// nothing hands the key to this editor's own completion: after `x` that is
// every command beginning with it, which on a Linux runner is enough for a
// list query to take the next keys.
func TestACompletionWidgetOnItsKeyHasItsParameters(t *testing.T) {
	control, screen := widgetSession(t, `cf() { seen="W=$WIDGET t=${(t)BUFFER}"; BUFFER=zz; ran=widget }
zle -C cw complete-word cf; bindkey '^T' cw
show() { BUFFER="GOT[$seen|$BUFFER|${ran-unset}]" }
zle -N show; bindkey '^Xs' show
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	steps := []struct{ keys, want string }{
		// With the carriage return: the function's output goes out the way a
		// widget's does, and raw mode alone writes a bare line feed.
		{"qzq\x14", "cf: read-only variable: BUFFER\r\n"},
		{"\x18s", "GOT[W=cw t=scalar-local-readonly-special|qzq|unset]"},
		{"\x18cprint -r -- END$((1+1))\r", "END2"},
	}
	for _, step := range steps {
		if _, err := control.WriteString(step.keys); err != nil {
			t.Fatalf("typing %q: %v", step.keys, err)
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%s\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
