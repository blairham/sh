// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A completion widget called by name from another widget completes, the way a
// key bound to it does (#6142).
//
// The shape is zsh-autosuggestions': it wraps every widget, the completion
// widgets included, and its wrapper calls the original by name. Measured
// 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal, with the
// rc below and `x al` typed: the wrapper's key makes the line `x alp`, a
// second press lists `alpha   alpine`, and `$WIDGET` inside the completion
// function is the wrapper's name. Here the function ran outside any
// completion and its `compadd` refused with `can only be called from
// completion function`, every time.
func TestACompletionWidgetCalledByNameCompletes(t *testing.T) {
	control, screen := widgetSession(t, `f() { compadd alpha alpine; seen=$WIDGET }
zle -C mycomp .complete-word f
w() { zle mycomp }; zle -N w; bindkey '^T' w
show() { BUFFER="GOT[$BUFFER|$seen]" }; zle -N show; bindkey '^Xs' show
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	steps := []struct{ keys, want string }{
		{"x al\x14", "x alp"},
		{"\x14", "alpha   alpine"},
		{"\x18s", "GOT[x alp|w]"},
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
	if text := screen.Text(); strings.Contains(text, "can only be called") {
		t.Errorf("the completion function refused:\n%q", smoke.LastLines(text, 8))
	}
}
