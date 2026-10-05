// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A completion widget whose function stops on an error completes nothing and
// draws the line again under the diagnostic, on a real terminal (#6062,
// #6068).
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal:
// `ech` and ^T print `cf: read-only variable: BUFFER`, a bell, and the
// prompt and `ech` again below them, and the line is still `ech` — where
// this editor's own completion, asked after the function, made it `echo`.
func TestACompletionThatStopsCompletesNothing(t *testing.T) {
	control, screen := widgetSession(t, `cf() { BUFFER=zz }
zle -C cw complete-word cf; bindkey '^T' cw
show() { BUFFER="GOT[$BUFFER]" }; zle -N show; bindkey '^Xs' show
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	steps := []struct{ keys, want string }{
		{"ech\x14", "cf: read-only variable: BUFFER\r\n\a"},
		// Drawn again: the prompt, then the line, under the diagnostic.
		{"", widgetMark + "ech"},
		{"\x18s", "GOT[ech]"},
		{"\x18cprint -r -- END$((1+1))\r", "END2"},
	}
	for _, step := range steps {
		if step.keys != "" {
			if _, err := control.WriteString(step.keys); err != nil {
				t.Fatalf("typing %q: %v", step.keys, err)
			}
		}
		if err := screen.Await(step.want, widgetBudget); err != nil {
			t.Fatalf("after %q, want\n%q\n%v\n%q", step.keys, step.want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
}
