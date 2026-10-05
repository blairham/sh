// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A `zle -F` handler runs while the question before a long listing waits for
// its answer, as it does while the line waits for a key (#6130).
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal,
// with LISTMAX=3, a descriptor that turns readable 1.5 s after the line is
// started and a handler that prints `H`: Tab asks `do you wish to see all …?`
// and `H` appears after the question while it goes on waiting. Here the
// handler was held until the question was answered.
func TestADescriptorHandlerRunsWhileTheListQuestionWaits(t *testing.T) {
	control, screen := widgetSession(t, `LISTMAX=3
f() { compadd alpha1 alpha2 alpha3 alpha4 alpha5 alpha6 }
zle -C cw complete-word f; bindkey '^T' cw
arm() { exec {fd}< <(sleep 2; print hi); zle -F $fd h }
h() { local l; read -u $1 l; zle -F $1; exec {1}<&-; print -n "HAND$((40+2))" }
zle -N arm; bindkey '^Xa' arm
`)
	steps := []struct{ keys, want string }{
		// The first press fills in `alpha` and the second lists, and six is
		// more than LISTMAX: the question.
		{"\x18aa\x14\x14", "possibilities"},
		// Nothing is typed: the handler has to fire on its own while the
		// question is still on the screen waiting.
		{"", "HAND42"},
		{"n", "alpha"},
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
	if _, err := control.WriteString("\x03print -r -- END$((1+1))\r"); err != nil {
		t.Fatal(err)
	}
	if err := screen.Await("END2", widgetBudget); err != nil {
		t.Fatalf("the line after: %v\n%q", err, smoke.LastLines(screen.Text(), 6))
	}
}
